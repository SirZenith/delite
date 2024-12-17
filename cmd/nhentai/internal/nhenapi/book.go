package nhenapi

import (
	"encoding/json"

	"github.com/SirZenith/delite/cmd/nhentai/internal/nhenapi/apipath"
)

type BookPageType string

const (
	BookPageGif  BookPageType = "g"
	BookpageJpg               = "j"
	BookPagePng               = "p"
	BookPageWebp              = "w"
)

var BookPageExtMap = map[BookPageType]string{
	BookPageGif:  "gif",
	BookpageJpg:  "jpg",
	BookPagePng:  "png",
	BookPageWebp: "webp",
}

type BookTitle struct {
	English  string `json:"english"`
	Japanese string `json:"japanese"`
	Pretty   string `json:"pretty"`
}

type BookPage struct {
	Type   BookPageType `json:"t"`
	Width  int          `json:"w"`
	Height int          `json:"h"`
}

func (bp *BookPage) GetExt() string {
	return BookPageExtMap[bp.Type]
}

type BookImages struct {
	Pages     []*BookPage `json:"pages"`
	Cover     BookPage    `json:"cover"`
	Thumbnail BookPage    `json:"thumbnail"`
}

type BookTag struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Count int    `json:"count"`
}

type Book struct {
	MediaID      string     `json:"media_id"`
	Title        BookTitle  `json:"title"`
	Images       BookImages `json:"images"`
	Scanlator    string     `json:"scanlator"`
	UploadDate   int64      `json:"upload_date"`
	Tags         []BookTag  `json:"tags"`
	NumPages     int        `json:"num_pages"`
	NumFavorites int        `json:"num_favorites"`
}

func NewBookFromJSON(data []byte) (*Book, error) {
	var book Book
	err := json.Unmarshal(data, &book)
	return &book, err
}

func (b *Book) GetPage(pageNum int) *BookPage {
	if pageNum > b.NumPages {
		return nil
	}
	return b.Images.Pages[pageNum-1]
}

func (b *Book) PageURL(pageNum int) []string {
	page := b.GetPage(pageNum)
	if page == nil {
		return nil
	}

	ext := page.GetExt()

	return apipath.BookPage(b.MediaID, pageNum, ext)
}

func (b *Book) CoverURL() string {
	cover := b.Images.Cover
	ext := cover.GetExt()
	return apipath.BookCover(b.MediaID, ext)
}

func (b *Book) PageThumbURL(pageNum int) string {
	page := b.GetPage(pageNum)
	if page == nil {
		return ""
	}
	ext := page.GetExt()
	return apipath.BookPageThumb(b.MediaID, pageNum, ext)
}

func (b *Book) CoverThumbURL() string {
	thumb := b.Images.Thumbnail
	ext := thumb.GetExt()
	return apipath.BookCoverThumb(b.MediaID, ext)
}
