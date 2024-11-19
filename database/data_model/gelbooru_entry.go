package data_model

import "gorm.io/gorm"

type GelbooruEntry struct {
	gorm.Model

	ThumbnailURL string `gorm:"unique"`
	ContentURL   string
	FileName     string

	Tag         string
	MarkDeleted bool
	Rating      int
}
