package nhenapi

import (
	"path"
)

type HostType int

const (
	HostAPI HostType = iota
	HostImage
	HostThumb
)

type SearchSortMode string

const (
	SortDate    SearchSortMode = "date"
	SortPopular SearchSortMode = "popular"
	SortWeek    SearchSortMode = "popular-week"
	SortToday   SearchSortMode = "popular-today"
	SortMonth   SearchSortMode = "popular-month"
)

var HostList = []string{
	HostAPI:   "nhentai.net",
	HostImage: "i.nhentai.net",
	HostThumb: "t.nhentai.net",
}

var ImageHosts = []string{
	"i.nhentai.net",
	"i1.nhentai.net",
	"i2.nhentai.net",
	"i3.nhentai.net",
	"i4.nhentai.net",
	"i5.nhentai.net",
	"i6.nhentai.net",
	"i7.nhentai.net",
}

var ThumbnailHosts = []string{
	"t.nhentai.net",
	"t1.nhentai.net",
	"t2.nhentai.net",
	"t3.nhentai.net",
	"t4.nhentai.net",
	"t6.nhentai.net",
	"t7.nhentai.net",
}

const hostScheme = "https"

func Wrap(hostType HostType, targetPath string) string {
	return hostScheme + "://" + path.Join(HostList[hostType], targetPath)
}

func BatchWrap(hostList []string, targetPath string) []string {
	result := make([]string, 0, len(hostList))

	for _, host := range hostList {
		url := hostScheme + "://" + path.Join(host, targetPath)
		result = append(result, url)
	}

	return result
}

