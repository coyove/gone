package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/common/lru"
	"github.com/russross/blackfriday"
)

func writeError(w http.ResponseWriter, msg string) {
	w.Write([]byte(fmt.Sprintf(`<html>
		<head><meta charset="UTF-8"><title>Error</title></head>
		<body bgcolor="white">
		%s
		<hr>
		If this is a temporary error, try refreshing this page later
		</body>
		</html>
		`, msg)))
}

func writeInfo(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    "admin",
		Value:   o.conf.Password,
		Expires: time.Now().AddDate(1, 0, 0),
	})

	w.Write([]byte(`<html>
		<head><meta charset="UTF-8"><title>Info</title></head>
		<body bgcolor="white">
		<pre>`))

	buf, _ := json.MarshalIndent(o.conf, "", "  ")
	w.Write(buf)

	w.Write([]byte("<hr>"))

	w.Write([]byte("Access:\n" + o.access + "<hr>Refresh:\n" + o.refresh + "<hr>"))

	o.cache.Info(func(k lru.Key, v interface{}, hits, weight int64) {
		w.Write([]byte(fmt.Sprintf("%6d %s\n", hits, k)))
	})

	w.Write([]byte("<hr>"))

	o.prefetch.Info(func(k lru.Key, v interface{}, hits, weight int64) {
		w.Write([]byte(fmt.Sprintf("p %6d %s\n", hits, k)))
	})
	w.Write([]byte("</pre></body></html>"))
}

func _orderAsc(b bool) bool  { return b }
func _orderDesc(b bool) bool { return !b }

type dummyWriter struct{ bytes.Buffer }

func (d *dummyWriter) Header() http.Header { return http.Header{} }

func (d *dummyWriter) WriteHeader(statusCode int) {}

func serveFile(w http.ResponseWriter, r *http.Request, fn string, values []*driveItem) bool {
	for _, item := range values {
		if item.Name == fn {
			hash := fmt.Sprintf("%x", sha1.Sum([]byte(fn)))
			cachepath := "cache/" + hash[:2] + "/" + hash[2:4]
			os.MkdirAll(cachepath, 0755)
			cachepath += "/" + hash[4:] + "-" + fn
			o.prefetch.Get(cachepath)

			if _, err := os.Stat(cachepath); err == nil {
				http.ServeFile(w, r, cachepath)
				return true
			}

			resp, err := o.httpClient.Get(item.DownloadURL)
			if err != nil {
				writeError(w, err.Error())
				return true
			}

			for k, vs := range resp.Header {
				if k == "Content-Disposition" {
					continue
				}
				h := w.Header()
				if h != nil {
					for _, v := range vs {
						h.Add(k, v)
					}
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

			n, err := io.Copy(writer, resp.Body)
			if err == nil {
				o.prefetch.AddWeight(cachepath, true, n)
			} else {
				log.Println(err)
			}
			resp.Body.Close()
			return true
		}
	}
	return false
}

func Main(w http.ResponseWriter, r *http.Request) {
	admincookie, _ := r.Cookie("admin")
	isAdmin := admincookie != nil && admincookie.Value == o.conf.Password

	if img := r.FormValue("image"); img != "" {
		w.Header().Add("Content-Type", "image/png")
		w.Header().Add("Cache-Control", "max-age=31536000")
		w.Write(o.icons[img])
		return
	}

	if r.FormValue("auth") == o.conf.Password {
		url0 := "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=%s&scope=files.readwrite.all+offline_access&response_type=code&redirect_uri=%s"
		url0 = fmt.Sprintf(url0, o.conf.ClientID, o.conf.RedirURL)
		http.Redirect(w, r, url0, http.StatusTemporaryRedirect)
		return
	}

	if r.FormValue("info") == o.conf.Password {
		writeInfo(w)
		return
	}

	if strings.HasPrefix(r.RequestURI, "/favicon.ico") && o.conf.Favicon != "" {
		http.ServeFile(w, r, o.conf.Favicon)
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
	if fn != "" && o.conf.prefetchRegex != nil && o.conf.prefetchRegex.MatchString(fn) {
		if serveFile(w, r, fn, x.Values) {
			return
		}
	}

	upath, _ := url.PathUnescape(path)
	favicon := `<link rel="shortcut icon" href="https://onedrive.live.com/favicon.ico">`
	if o.conf.Favicon != "" {
		favicon = ""
	}
	w.Write([]byte(fmt.Sprintf(`<html>
<head><meta charset="UTF-8">%s<title>Index of %s</title></head>
<body bgcolor="white">
<h1 id=indexof>Index of %s</h1>%s<pre>`, favicon, upath, upath, o.conf.Header)))

	maxNameLen, maxSizeLen := 6, 2
	for i := len(x.Values) - 1; i >= 0; i-- {
		item := x.Values[i]
		item.isHidden = o.conf.ignoreRegex != nil && o.conf.ignoreRegex.MatchString(item.Name)

		l := strlen(item.Name)
		if item.Folder != nil {
			l++
		}
		if item.isHidden {
			// we will prepend "* "
			l += 2
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

	var readme []byte
	for _, item := range x.Values {
		href := item.DownloadURL + "/" + item.Name
		name := item.Name

		if item.Folder != nil {
			href = path + name
			name += "/"
		} else if o.conf.prefetchRegex != nil && o.conf.prefetchRegex.MatchString(name) {
			href = "?file=" + name
		}

		if item.isHidden {
			if isAdmin {
				name = "* " + name
			} else {
				continue
			}
		}

		if !o.conf.DisableReadme {
			switch strings.ToLower(name) {
			case "readme.md":
				dw := &dummyWriter{}
				if serveFile(dw, r, name, x.Values) {
					readme = blackfriday.MarkdownCommon(dw.Bytes())
				}
			case "readme.txt", "readme":
				dw := &dummyWriter{}
				dw.WriteString("<pre>")
				if serveFile(dw, r, name, x.Values) {
					dw.WriteString("</pre>")
					readme = dw.Bytes()
				}
			case "readme.html", "readme.htm":
				dw := &dummyWriter{}
				if serveFile(dw, r, name, x.Values) {
					readme = dw.Bytes()
				}
			}
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

	w.Write([]byte(`</pre><hr>`))
	w.Write(readme)
	w.Write([]byte(o.conf.Footer))
	w.Write([]byte(fmt.Sprintf(`
<address><a href="https://github.com/coyove/gone" target=_blank>Gone</a> (%s) Server at %s,
Last token at %s
</address>`,
		runtime.GOOS,
		o.conf.redir.Hostname(),
		time.Unix(o.lastRefreshed, 0).UTC().Format("15:04"))))
	w.Write([]byte("</body></html>"))
}
