package main

import (
	"encoding/json"
	"flag"
	_ "image/jpeg"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

var listen = flag.String("l", ":8080", "")
var configfile = flag.String("c", "", "")
var o *oneManager
var conf *config

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
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	if *configfile == "" {
		log.Fatalln("Please specify the config file")
	}

	configbuf, err := ioutil.ReadFile(*configfile)
	if err != nil {
		log.Fatalln(err)
	}

	conf = &config{}
	if err := json.Unmarshal(configbuf, conf); err != nil {
		log.Fatalln(err)
	}

	if conf.Password == "" {
		log.Fatalln("Please specify a admin password")
	}

	conf.redir, err = url.Parse(conf.RedirURL)
	if err != nil {
		log.Fatalln(err)
	}

	if conf.Header != "" {
		buf, _ := ioutil.ReadFile(conf.Header)
		conf.Header = string(buf)
	}

	if conf.Footer != "" {
		buf, _ := ioutil.ReadFile(conf.Footer)
		conf.Footer = string(buf)
	}

	if conf.Ignore != "" {
		conf.ignoreRegex = regexp.MustCompile(conf.Ignore)
	}

	o = newOneManager(conf)
	http.HandleFunc("/authcallback", o.GetTokenCallback)
	http.HandleFunc("/", Main)

	log.Println("Hello", *listen)
	http.ListenAndServe(*listen, nil)
}
