package main

import (
	"encoding/json"
	"flag"
	"fmt"
	_ "image/jpeg"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var listen = flag.String("l", ":8080", "Listening address")
var configfile = flag.String("c", "", "Config file to load")
var o *oneManager

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

func strlen(s string) int {
	ln := 0.0
	for _, r := range s {
		if r < 0x2e80 { // rough range
			ln++
		} else {
			ln += 1.66
		}
	}
	return int(ln)
}

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)

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

	if conf.Password == "" {
		log.Fatalln("Please specify a admin password")
	}
	if conf.ClientID == "" {
		log.Fatalln("Please specify a client ID")
	}
	if conf.ClientSecret == "" {
		log.Fatalln("Please specify a client secret")
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

	if conf.Prefetch != "" {
		conf.prefetchRegex = regexp.MustCompile(conf.Prefetch)
	}

	o = newOneManager(conf)

	os.Mkdir("cache", 0755)
	log.Println("Make cache dir: ./cache")

	if o.prefetch != nil {
		prefetched := int64(0)
		log.Println("Counting prefetched")
		filepath.Walk("cache", func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			name := info.Name()
			switch strings.ToLower(name[strings.LastIndex(name, "-")+1:]) {
			case "readme.md", "readme", "readme.txt", "readme.htm", "readme.html":
				os.Remove(path)
				return nil
			}

			prefetched += info.Size()
			o.prefetch.AddWeight(path, true, info.Size())
			return nil
		})
		log.Println("Prefetched:", prefetched, "bytes")
	}

	http.HandleFunc("/authcallback", o.GetTokenCallback)
	http.HandleFunc("/", Main)

	log.Println("Hello", *listen)

	if _, err := os.Stat(conf.ClientID + ".token"); err != nil {
		fmt.Println()
		fmt.Println("***********************************************************")
		fmt.Println("*     If this is your first time running gone server      *")
		fmt.Println("* Follow the belowed URL to sign in the Microsoft account *")
		fmt.Println("***********************************************************")
		fmt.Println("https://" + conf.redir.Hostname() + "/?auth=" + conf.Password)
		fmt.Println()
	}
	http.ListenAndServe(*listen, nil)
}
