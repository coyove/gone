package main

import (
	"bytes"
	"net/http"
	"sort"
	"strings"

	"github.com/russross/blackfriday"
)

type dummyWriter struct{ bytes.Buffer }

func (d *dummyWriter) Header() http.Header { return http.Header{} }

func (d *dummyWriter) WriteHeader(statusCode int) {}

func renderReadme(name string, values []*driveItem, r *http.Request) []byte {
	switch strings.ToLower(name) {
	case "readme.md":
		dw := &dummyWriter{}
		if serveFile(dw, r, name, values) {
			return blackfriday.MarkdownCommon(dw.Bytes())
		}
	case "readme.txt", "readme":
		dw := &dummyWriter{}
		dw.WriteString("<pre>")
		if serveFile(dw, r, name, values) {
			dw.WriteString("</pre>")
			return dw.Bytes()
		}
	case "readme.html", "readme.htm":
		dw := &dummyWriter{}
		if serveFile(dw, r, name, values) {
			return dw.Bytes()
		}
	}
	return nil
}

func _orderAsc(b bool) bool { return b }

func _orderDesc(b bool) bool { return !b }

func sortValues(cat string, values []*driveItem, orderfunc func(bool) bool) {
	// folders always come first, then files
	switch cat {
	case "n":
		sort.Slice(values, func(i, j int) bool {
			if values[i].Folder != nil && values[j].Folder == nil {
				return true
			}
			if values[i].Folder == nil && values[j].Folder != nil {
				return false
			}
			return orderfunc(values[i].Name < values[j].Name)
		})
	case "t":
		sort.Slice(values, func(i, j int) bool {
			if values[i].Folder != nil && values[j].Folder == nil {
				return true
			}
			if values[i].Folder == nil && values[j].Folder != nil {
				return false
			}
			return orderfunc(values[i].LastModifiedDateTime < values[j].LastModifiedDateTime)
		})
	case "s":
		sort.Slice(values, func(i, j int) bool {
			if values[i].Folder != nil && values[j].Folder == nil {
				return true
			}
			if values[i].Folder == nil && values[j].Folder != nil {
				return false
			}
			if values[i].Folder == nil && values[j].Folder == nil {
				return orderfunc(values[i].Size < values[j].Size)
			}
			return orderfunc(values[i].Folder.ChildCount < values[j].Folder.ChildCount)
		})
	}
}

func spaces(num int) string {
	if num < 32 {
		return "                                "[:num]
	}
	return strings.Repeat(" ", num)
}
