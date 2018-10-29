package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/common/lru"
)

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
	cacheTTL        int64
	prefetch        *lru.Cache
	icons           map[string][]byte
	conf            *config
}

func newOneManager(conf *config) *oneManager {
	o := &oneManager{}
	o.conf = conf
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
	o.cacheTTL = int64(conf.CacheTTL)
	o.prefetch = lru.NewCache(int64(conf.PrefetchSize) * 1024 * 1024)
	o.prefetch.OnEvicted = func(k lru.Key, v interface{}) {
		go func() {
			time.Sleep(time.Second)
			os.Remove(k.(string))
		}()
	}
	o.icons = DefaultIcons

	buf, _ := ioutil.ReadFile(o.client.id + ".token")
	parts := strings.Split(string(buf), "\n")
	if len(parts) == 3 {
		o.lastRefreshed, _ = strconv.ParseInt(parts[0], 10, 64)
		o.access = parts[1]
		o.refresh = parts[2]
		o.Restart()
		log.Println("Preloaded from ", o.client.id, ".token:", time.Unix(o.lastRefreshed, 0))
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

				start := time.Now()
				err := o.RefreshToken()
				log.Println("Refresh token in", time.Now().Sub(start).Seconds(), "s")

				if err != nil {
					log.Println("Refresh token:", err)
					s.callback <- stateRefreshFailed
					continue
				}

				o.lastRefreshed = time.Now().Unix()
				log.Println("New token is OK at", time.Now())
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

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
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
