package protodef

import "path"

// ----------------------------------------------------------------------------

type ReqGalleryGetGallery struct {
	Info      struct{} `path:"/api/v2/galleries/${GalleryId}" method:"GET"`
	GalleryId int
}

type RespGalleryGetGallery struct {
	ID           int           `json:"id"`
	MediaID      string        `json:"media_id"`
	Title        BookTitle     `json:"title"`
	Cover        BookCover     `json:"cover"`
	Thumbnail    BookThumbnail `json:"thumbnail"`
	Scanlator    string        `json:"scanlator"`
	UploadDate   int64         `json:"upload_date"`
	Tags         []BookTag     `json:"tags"`
	NumPages     int           `json:"num_pages"`
	NumFavorites int           `json:"num_favorites"`
	Pages        []*BookPage   `json:"pages"`
}

type BookTitle struct {
	English  string `json:"english"`
	Japanese string `json:"japanese"`
	Pretty   string `json:"pretty"`
}

type BookCover struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type BookThumbnail struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type BookPage struct {
	Number          int    `json:"number"`
	Path            string `json:"path"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Thumbnail       string `json:"thumbnail"`
	ThumbnailWidth  int    `json:"thumbnail_width"`
	ThumbnailHeight int    `json:"thumbnail_height"`
}

func (bp *BookPage) GetExt() string {
	basename := path.Base(bp.Path)
	return path.Ext(basename)
}

type BookTag struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	URL   string `json:"url"`
	Count int    `json:"count"`
}
