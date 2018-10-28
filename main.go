package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/draw"
	_ "image/jpeg"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/common/lru"
)

type node struct {
	x, y, w, h  int
	used        bool
	down, right *node
}

type packer struct {
	root   *node
	canvas *image.RGBA
	images []image.Image
	w, h   int
}

func newPacker(images []image.Image) *packer {
	w, h := 0, 0
	const tile = 2048

	for _, img := range images {
		if img.Bounds().Max.X > w {
			w = img.Bounds().Max.X
		}
		if img.Bounds().Max.Y > h {
			h = img.Bounds().Max.Y
		}
	}

	if h < tile {
		h = tile
	}
	if w < tile {
		w = tile
	}

	sort.Slice(images, func(i, j int) bool {
		bi := images[i].Bounds()
		bj := images[j].Bounds()
		return bi.Dx()*bi.Dy() > bj.Dx()*bj.Dy()
	})

	return &packer{
		root:   &node{w: w, h: h},
		canvas: image.NewRGBA(image.Rect(0, 0, w, h)),
		images: images,
	}
}

func (p *packer) fit() []image.Image {
	tmp := make([]image.Image, 0)
	for _, img := range p.images {
		bound := img.Bounds()
		node := p.findNode(p.root, bound.Dx(), bound.Dy())
		if node != nil {
			p.splitNodeAndDraw(node, bound.Dx(), bound.Dy(), img)
		} else {
			tmp = append(tmp, img)
		}
	}

	ret := []image.Image{p.shrink()}
	if len(tmp) > 0 {
		p2 := newPacker(tmp)
		ret = append(ret, p2.fit()...)
	}

	return ret
}

func (p *packer) findNode(root *node, w, h int) *node {
	if root.used {
		n := p.findNode(root.right, w, h)
		if n != nil {
			return n
		}
		return p.findNode(root.down, w, h)
	} else if (w <= root.w) && (h <= root.h) {
		return root
	} else {
		return nil
	}
}

func (p *packer) splitNodeAndDraw(n *node, w, h int, img image.Image) {
	n.used = true
	n.down = &node{x: n.x, y: n.y + h, w: n.w, h: n.h - h}
	n.right = &node{x: n.x + w, y: n.y, w: n.w - w, h: h}
	draw.Draw(p.canvas, image.Rect(n.x, n.y, n.x+w, n.y+h), img, image.ZP, draw.Src)
	if n.x+w > p.w {
		p.w = n.x + w
	}
	if n.y+h > p.h {
		p.h = n.y + h
	}
}

func (p *packer) shrink() image.Image {
	return p.canvas.SubImage(image.Rect(0, 0, p.w, p.h))
}

const (
	refreshLimit = 3550
)

type _state int

const (
	stateOK = iota + 1
	stateNotYet
	stateTimeout
	stateRefreshFailed
)

type _stream struct {
	ts       int64
	callback chan _state
}

type oneManager struct {
	client struct {
		id, secret string
		redir      string
	}
	access, refresh string
	exit            chan bool
	stream          chan _stream
	lastRefreshed   int64
	httpClient      *http.Client
	dirTemplate     *template.Template
	cache           *lru.Cache
	icons           map[string][]byte
	cacheTTL        int64
}

func newOneManager(conf *config) *oneManager {
	o := &oneManager{}
	o.client.id = conf.ClientID
	o.client.secret = conf.ClientSecret
	o.client.redir = conf.RedirURL
	o.httpClient = &http.Client{
		Timeout: time.Second * 2,
	}

	if conf.CacheSize < 32 {
		conf.CacheSize = 32
	}
	if conf.CacheTTL < 10 {
		conf.CacheTTL = 10
	}

	o.cache = lru.NewCache(int64(conf.CacheSize))
	o.cacheTTL = 60
	o.icons = DefaultIcons

	buf, _ := ioutil.ReadFile(o.client.id + ".token")
	parts := strings.Split(string(buf), "\n")
	if len(parts) == 3 {
		o.lastRefreshed, _ = strconv.ParseInt(parts[0], 10, 64)
		o.access = parts[1]
		o.refresh = parts[2]
		o.Restart()
		log.Println("Preloaded:", o.lastRefreshed)
	}
	return o
}

func (o *oneManager) saveTokens() {
	ioutil.WriteFile(o.client.id+".token", []byte(
		strconv.Itoa(int(time.Now().Unix()))+"\n"+
			o.access+"\n"+
			o.refresh,
	), 0755)
}

func (o *oneManager) WaitState() _state {
	if o.lastRefreshed == 0 {
		return stateNotYet
	}

	now := time.Now().Unix()
	if now-o.lastRefreshed < refreshLimit-10 {
		return stateOK
	}

	cb := make(chan _state, 1)
	o.stream <- _stream{
		ts:       now,
		callback: cb,
	}

	select {
	case resp := <-cb:
		return resp
	case <-time.After(time.Second):
		return stateTimeout
	}
}

func (o *oneManager) Restart() {
	if o.exit != nil {
		o.exit <- true
	}
	o.exit = make(chan bool, 1)
	o.stream = make(chan _stream, 1024)

	go func() {
		exit2 := make(chan bool, 1)
		go func() {
			for {
				select {
				case <-exit2:
					return
				default:
				}

				select {
				case <-exit2:
					return
				case o.stream <- _stream{
					ts:       time.Now().Unix(),
					callback: make(chan _state, 1),
				}:
					// Ping every second
				}
				time.Sleep(time.Second)
			}
		}()

		for {
			// exit signal always comes first
			select {
			case <-o.exit:
				return
			default:
			}

			select {
			case s := <-o.stream:
				if s.ts-o.lastRefreshed < refreshLimit {
					s.callback <- stateOK
					continue
				}

				err := o.RefreshToken()
				if err != nil {
					log.Println("Refresh token:", err)
					s.callback <- stateRefreshFailed
					continue
				}

				o.lastRefreshed = time.Now().Unix()
				log.Println("New token is OK at", o.lastRefreshed)
				s.callback <- stateOK
			case <-o.exit:
				log.Println("Old dying")
				exit2 <- true
				return
			}
		}
	}()
}

func (o *oneManager) MakeForm() url.Values {
	form := url.Values{}
	form.Add("client_id", o.client.id)
	form.Add("redirect_uri", o.client.redir)
	form.Add("client_secret", o.client.secret)
	return form
}

func (o *oneManager) MakeRequest(endpoint string) *http.Request {
	req, _ := http.NewRequest("GET", "https://graph.microsoft.com/v1.0"+endpoint, nil)
	req.Header.Add("Authorization", "bearer "+o.access)
	return req
}

func (o *oneManager) GetTokenCallback(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	form := o.MakeForm()
	form.Add("code", code)
	form.Add("grant_type", "authorization_code")
	req, _ := http.NewRequest("POST", "https://login.microsoftonline.com/common/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}

	defer resp.Body.Close()
	buf, _ := ioutil.ReadAll(resp.Body)

	o.access, o.refresh = parseToken(buf)
	if o.access == "" || o.refresh == "" {
		log.Println(resp.Status)
		log.Println(string(buf))
		return
	}

	log.Println("Init new token:", len(o.access), len(o.refresh))
	o.saveTokens()
	o.Restart()
}

func (o *oneManager) RefreshToken() error {
	form := o.MakeForm()
	form.Add("refresh_token", o.refresh)
	form.Add("grant_type", "refresh_token")
	req, _ := http.NewRequest("POST", "https://login.microsoftonline.com/common/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	buf, _ := ioutil.ReadAll(resp.Body)
	o.access, o.refresh = parseToken(buf)
	if o.access == "" || o.refresh == "" {
		return fmt.Errorf("failed to refresh token: %s", resp.Status)
	}

	o.saveTokens()
	return nil
}

func (o *oneManager) List(path string) (x *driveItems) {
	if i, ok := o.cache.Get(path); ok {
		x = i.(*driveItems)
		if time.Now().Unix()-x.ts < o.cacheTTL {
			return
		}
	}

	x = &driveItems{}

	state := o.WaitState()
	switch state {
	case stateNotYet:
		x.Error.Message = "Server is not available yet"
		return
	case stateTimeout:
	case stateRefreshFailed:
		x.Error.Message = "Please try again later"
		return
	}

	xpath := ""
	if path == "/" {
		xpath = "/me/drive/root/children"
	} else {
		xpath = "/me/drive/root:" + path + ":/children"
	}

	req := o.MakeRequest(xpath)
	resp, err := o.httpClient.Do(req)
	if err != nil {
		x.Error.Message = err.Error()
		return
	}

	defer resp.Body.Close()
	buf, _ := ioutil.ReadAll(resp.Body)

	json.Unmarshal(buf, x)

	x.ts = time.Now().Unix()
	o.cache.Add(path, x)
	return
}

var listen = flag.String("l", ":8080", "")
var configfile = flag.String("c", "", "")

func parseToken(buf []byte) (string, string) {
	m := map[string]interface{}{}
	json.Unmarshal(buf, &m)
	a, _ := m["access_token"].(string)
	r, _ := m["refresh_token"].(string)
	return a, r
}

func prettySize(size int) string {
	if size < 1024 {
		return strconv.Itoa(size)
	}
	if size < 1024*1024 {
		return strconv.FormatFloat(float64(size)/1024, 'f', 1, 64) + "K"
	}
	if size < 1024*1024*1024 {
		return strconv.FormatFloat(float64(size)/1024/1024, 'f', 1, 64) + "M"
	}
	return strconv.FormatFloat(float64(size)/1024/1024/1024, 'f', 1, 64) + "G"
}

func main() {
	flag.Parse()

	if *configfile == "" {
		log.Fatalln("Please specify the config file")
	}

	configbuf, err := ioutil.ReadFile(*configfile)
	if err != nil {
		log.Fatalln(err)
	}

	conf := &config{}
	if err := json.Unmarshal(configbuf, conf); err != nil {
		log.Fatalln(err)
	}

	o := newOneManager(conf)
	http.HandleFunc("/authcallback", o.GetTokenCallback)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if img := r.FormValue("image"); img != "" {
			w.Header().Add("Content-Type", "image/png")
			w.Write(o.icons[img])
			return
		}

		if r.FormValue("auth") == conf.Password {
			url0 := "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=%s&scope=files.readwrite.all+offline_access&response_type=code&redirect_uri=%s"
			url0 = fmt.Sprintf(url0, conf.ClientID, conf.RedirURL)
			http.Redirect(w, r, url0, http.StatusTemporaryRedirect)
			return
		}

		path := r.URL.Path[1:]
		order, revorder, orderfunc := r.FormValue("o"), "d", func(b bool) bool { return b }
		if order == "d" {
			revorder = "a"
			orderfunc = func(b bool) bool { return !b }
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
			w.Write([]byte(x.Error.Message))
			return
		}

		upath, _ := url.PathUnescape(path)
		w.Write([]byte(fmt.Sprintf(`<html>
<head><meta charset="UTF-8"><title>Index of %s</title></head>
<body bgcolor="white">
<h1>Index of %s</h1><pre>
`, upath, upath)))

		switch r.FormValue("c") {
		case "n":
			sort.Slice(x.Values, func(i, j int) bool { return orderfunc(x.Values[i].Name < x.Values[j].Name) })
		case "t":
			sort.Slice(x.Values, func(i, j int) bool {
				return orderfunc(x.Values[i].LastModifiedDateTime < x.Values[j].LastModifiedDateTime)
			})
		case "s":
			sort.Slice(x.Values, func(i, j int) bool { return orderfunc(x.Values[i].Size < x.Values[j].Size) })
		}

		maxNameLen, maxSizeLen := 4, 2
		for _, item := range x.Values {
			if len(item.Name) > maxNameLen {
				maxNameLen = len(item.Name)
			}
			if s := prettySize(item.Size); len(s) > maxSizeLen {
				maxSizeLen = len(s)
			}
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

			if item.Folder != nil {
				href = path + item.Name
				item.Name += "/"
			}

			w.Write([]byte(fmt.Sprintf("<img src='?image=%s'> ", nameIcon(item.Name, item.Folder != nil))))
			w.Write([]byte(fmt.Sprintf("<a href='%s'>%s</a>", href, template.HTMLEscapeString(item.Name))))
			w.Write(bytes.Repeat([]byte(" "), maxNameLen+1-len(item.Name)))
			w.Write([]byte(item.LastModifiedDateTime[:10] + " " + item.LastModifiedDateTime[11:16]))

			size := prettySize(item.Size)
			if item.Folder != nil {
				size = "(" + strconv.Itoa(item.Folder.ChildCount) + ")"
			}

			w.Write(bytes.Repeat([]byte(" "), maxSizeLen+2-len(size)))
			w.Write([]byte(size))
			w.Write([]byte("\n"))
		}

		w.Write([]byte(`</pre><hr></body>
</html>`))

	})

	log.Println(*listen)
	http.ListenAndServe(*listen, nil)
	// files, _ := ioutil.ReadDir("/Users/coyove/Downloads/hw")
	// images := []image.Image{}
	// for _, file := range files {
	// 	if file.IsDir() {
	// 		continue
	// 	}
	// 	ifs, _ := os.Open("/Users/coyove/Downloads/hw/" + file.Name())
	// 	img, _, err := image.Decode(ifs)
	// 	ifs.Close()
	// 	if err != nil {
	// 		continue
	// 	}
	// 	images = append(images, img)
	// }

	// p := newPacker(images)

	// x := p.fit()

	// for i, y := range x {
	// 	of, _ := os.Create(strconv.Itoa(i) + ".png")
	// 	png.Encode(of, y)
	// 	of.Close()
	// }
}
