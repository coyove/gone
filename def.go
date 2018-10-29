package main

import (
	"net/url"
	"regexp"
)

type _folder struct {
	ChildCount int `json:"childCount"`
}

type driveItem struct {
	isHidden             bool
	DownloadURL          string `json:"@microsoft.graph.downloadUrl"`
	CreatedDateTime      string `json:"createdDateTime"`
	ID                   string `json:"id"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	Name                 string `json:"name"`
	Size                 int    `json:"size"`
	WebURL               string `json:"webUrl"`
	CreatedBy            struct {
		User struct {
			DisplayName string `json:"displayName"`
			ID          string `json:"id"`
		} `json:"user"`
	} `json:"createdBy"`
	LastModifiedBy struct {
		User struct {
			DisplayName string `json:"displayName"`
			ID          string `json:"id"`
		} `json:"user"`
	} `json:"lastModifiedBy"`
	ParentReference struct {
		DriveID   string `json:"driveId"`
		DriveType string `json:"driveType"`
		ID        string `json:"id"`
		Path      string `json:"path"`
	} `json:"parentReference"`
	FileSystemInfo struct {
		CreatedDateTime      string `json:"createdDateTime"`
		LastModifiedDateTime string `json:"lastModifiedDateTime"`
	} `json:"fileSystemInfo"`
	Folder *_folder `json:"folder"`
}

type driveItems struct {
	ts int64

	Values []*driveItem `json:"value"`
	Error  struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type config struct {
	ClientID      string
	ClientSecret  string
	RedirURL      string
	redir         *url.URL
	Password      string
	Header        string
	Footer        string
	Ignore        string
	ignoreRegex   *regexp.Regexp
	Prefetch      string
	prefetchRegex *regexp.Regexp
	Favicon       string
	DisableReadme bool
	CacheSize     int
	CacheTTL      int
	PrefetchSize  int
}
