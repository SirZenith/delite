package apipath

import (
	"fmt"
)

type HostType int

const (
	HostAPI HostType = iota
	HostImage
	HostThumb
)

type SearchSortMode string

const (
	SortNone    SearchSortMode = ""
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

const hostScheme = "https"

func wrap(hostType HostType, path string) string {
	return fmt.Sprintf("%s://%s%s", hostScheme, HostList[hostType], path)
}

func Search(query string, page int, sort SearchSortMode) string {
	var searchQuery string
	if sort == SortNone {
		searchQuery = ""
	} else {
		searchQuery = "&sort=" + string(sort)
	}

	path := fmt.Sprintf(
		"/api/galleries/search?query=%s&page=%d%s",
		query, page, searchQuery,
	)

	return wrap(HostAPI, path)
}

func SearchTagged(tagID string, page int) string {
	path := fmt.Sprintf(
		"/api/galleries/tagged?tag_id=%s&page=%d",
		tagID, page,
	)
	return wrap(HostAPI, path)
}

func SearchAlike(bookID int) string {
	path := fmt.Sprintf(
		"/api/gallery/%d/related", bookID,
	)
	return wrap(HostAPI, path)
}

func Book(bookID int) string {
	path := fmt.Sprintf(
		"/api/gallery/%d", bookID,
	)
	return wrap(HostAPI, path)
}

func BookCover(mediaID string, extension string) string {
	path := fmt.Sprintf(
		"/galleries/%s/cover.%s",
		mediaID, extension,
	)
	return wrap(HostThumb, path)
}

func BookPage(mediaID string, page int, extension string) string {
	path := fmt.Sprintf(
		"/galleries/%s/%d.%s",
		mediaID, page, extension,
	)
	return wrap(HostImage, path)
}

func BookCoverThumb(mediaID string, extension string) string {
	path := fmt.Sprintf(
		"/galleries/%s/thumb.%s",
		mediaID, extension,
	)
	return wrap(HostThumb, path)
}

func BookPageThumb(mediaID string, page int, extension string) string {
	path := fmt.Sprintf(
		"/galleries/%s/%dt.%s",
		mediaID, page, extension,
	)
	return wrap(HostThumb, path)
}

func RandomBookRedirect() string {
	path := "/random/"
	return wrap(HostAPI, path)
}
