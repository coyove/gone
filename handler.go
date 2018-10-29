package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/common/lru"
)

func writeError(w http.ResponseWriter, msg string) {
	w.Write([]byte(fmt.Sprintf(`<html>
		<head><meta charset="UTF-8"><title>Error</title></head>
		<body bgcolor="white">
		%s
		<hr>
		Try refreshing this page later
		</body>
		</html>
		`, msg)))
}

func writeInfo(w http.ResponseWriter) {
	w.Write([]byte(`<html>
		<head><meta charset="UTF-8"><title>Info</title></head>
		<body bgcolor="white">
		<pre style="white-space:wrap">`))

	w.Write([]byte("Access:\n" + o.access + "\nRefresh:\n" + o.refresh + "\n"))

	o.cache.Info(func(k lru.Key, v interface{}, hits, weight int64) {
		w.Write([]byte(fmt.Sprintf("%6d %s\n", hits, k)))
	})

	w.Write([]byte("</pre></body></html>"))
}

func _orderAsc(b bool) bool  { return b }
func _orderDesc(b bool) bool { return !b }

func Main(w http.ResponseWriter, r *http.Request) {
	if img := r.FormValue("image"); img != "" {
		w.Header().Add("Content-Type", "image/png")
		w.Header().Add("Cache-Control", "max-age=31536000")
		w.Write(o.icons[img])
		return
	}

	if r.FormValue("auth") == conf.Password {
		url0 := "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=%s&scope=files.readwrite.all+offline_access&response_type=code&redirect_uri=%s"
		url0 = fmt.Sprintf(url0, conf.ClientID, conf.RedirURL)
		http.Redirect(w, r, url0, http.StatusTemporaryRedirect)
		return
	}

	if r.FormValue("info") == conf.Password {
		writeInfo(w)
		return
	}

	if strings.HasPrefix(r.RequestURI, "/favicon.ico") && conf.Favicon != "" {
		http.ServeFile(w, r, conf.Favicon)
		return
	}

	path := r.URL.Path[1:]
	order, revorder, orderfunc := r.FormValue("o"), "d", _orderAsc
	if order == "d" {
		revorder = "a"
		orderfunc = _orderDesc
	}

	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	if path[len(path)-1] != '/' {
		path += "/"
		http.Redirect(w, r, path, http.StatusTemporaryRedirect)
		return
	}

	// we will have a path that always start with / and end with /
	x := o.List(path)

	if x.Error.Message != "" {
		writeError(w, x.Error.Message)
		return
	}

	fn := r.FormValue("file")
	if fn != "" && conf.prefetchRegex != nil && conf.prefetchRegex.MatchString(fn) {
		for _, item := range x.Values {
			if item.Name == fn {
				hash := fmt.Sprintf("%x", sha1.Sum([]byte(fn)))
				cachepath := "cache/" + hash[:2] + "/" + hash[2:4]
				os.MkdirAll(cachepath, 0755)
				cachepath += "/" + hash[4:] + filepath.Ext(fn)

				if _, err := os.Stat(cachepath); err == nil {
					http.ServeFile(w, r, cachepath)
					return
				}

				resp, err := o.httpClient.Get(item.DownloadURL)
				if err != nil {
					writeError(w, err.Error())
					return
				}

				for k, vs := range resp.Header {
					if k == "Content-Disposition" {
						continue
					}
					h := w.Header()
					for _, v := range vs {
						h.Add(k, v)
					}
				}

				cachefile, err := os.OpenFile(cachepath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
				var writer io.Writer
				if err == nil {
					writer = io.MultiWriter(w, cachefile)
					defer cachefile.Close()
				} else {
					writer = w
				}

				io.Copy(writer, resp.Body)
				resp.Body.Close()
				return
			}
		}
	}

	upath, _ := url.PathUnescape(path)
	w.Write([]byte(fmt.Sprintf(`<html>
<head><meta charset="UTF-8"><title>Index of %s</title></head>
<body bgcolor="white">
<h1 id=indexof>Index of %s</h1>%s<pre>`, upath, upath, conf.Header)))

	maxNameLen, maxSizeLen := 6, 2
	for i := len(x.Values) - 1; i >= 0; i-- {
		item := x.Values[i]
		if conf.ignoreRegex != nil && conf.ignoreRegex.MatchString(item.Name) {
			x.Values = append(x.Values[:i], x.Values[i+1:]...)
			continue
		}

		l := strlen(item.Name)
		if item.Folder != nil {
			l++
		}
		if l > maxNameLen {
			maxNameLen = l
		}
		if s := prettySize(item.Size); len(s) > maxSizeLen {
			maxSizeLen = len(s)
		}
	}

	switch r.FormValue("c") {
	case "n":
		sort.Slice(x.Values, func(i, j int) bool {
			if x.Values[i].Folder != nil && x.Values[j].Folder == nil {
				return true
			}
			if x.Values[i].Folder == nil && x.Values[j].Folder != nil {
				return false
			}
			return orderfunc(x.Values[i].Name < x.Values[j].Name)
		})
	case "t":
		sort.Slice(x.Values, func(i, j int) bool {
			if x.Values[i].Folder != nil && x.Values[j].Folder == nil {
				return true
			}
			if x.Values[i].Folder == nil && x.Values[j].Folder != nil {
				return false
			}
			return orderfunc(x.Values[i].LastModifiedDateTime < x.Values[j].LastModifiedDateTime)
		})
	case "s":
		sort.Slice(x.Values, func(i, j int) bool {
			if x.Values[i].Folder != nil && x.Values[j].Folder == nil {
				return true
			}
			if x.Values[i].Folder == nil && x.Values[j].Folder != nil {
				return false
			}
			if x.Values[i].Folder == nil && x.Values[j].Folder == nil {
				return orderfunc(x.Values[i].Size < x.Values[j].Size)
			}
			return orderfunc(x.Values[i].Folder.ChildCount < x.Values[j].Folder.ChildCount)
		})
	}

	w.Write([]byte(
		`<img src="?image=empty.png"> <a href="?c=n&o=` + revorder + `">Name</a>` + strings.Repeat(" ", maxNameLen+1-4) +
			`<a href="?c=t&o=` + revorder + `">Last Modified</a>   ` +
			strings.Repeat(" ", maxSizeLen+2-4) + `<a href="?c=s&o=` + revorder + `">Size</a>`,
	))

	w.Write([]byte(`<hr><img src="?image=back.png"> <a href="../">Parent Directory</a>` + strings.Repeat(" ", maxSizeLen+maxNameLen+16+3-16-1) + `-
`))

	for _, item := range x.Values {
		href := item.DownloadURL
		name := item.Name

		if item.Folder != nil {
			href = path + name
			name += "/"
		}

		w.Write([]byte(fmt.Sprintf("<img src='?image=%s'> ", nameIcon(name, item.Folder != nil))))
		w.Write([]byte(fmt.Sprintf("<a href='%s'>%s</a>", href, template.HTMLEscapeString(name))))
		w.Write(bytes.Repeat([]byte(" "), maxNameLen+1-strlen(name)))
		w.Write([]byte(item.LastModifiedDateTime[:10] + " " + item.LastModifiedDateTime[11:16]))

		size := prettySize(item.Size)
		if item.Folder != nil {
			size = "(" + strconv.Itoa(item.Folder.ChildCount) + ")"
		}

		w.Write(bytes.Repeat([]byte(" "), maxSizeLen+2-len(size)))
		w.Write([]byte(size))
		w.Write([]byte("\n"))
	}

	w.Write([]byte(fmt.Sprintf(`</pre><hr>
<address><a href="https://github.com/coyove/gone" target=_blank>Gone</a> (%s) Server at %s,
Last token at %s
</address>
%s
</body>
</html>`,
		runtime.GOOS,
		conf.redir.Hostname(),
		time.Unix(o.lastRefreshed, 0).UTC().Format("15:04"),
		conf.Footer)))

}
