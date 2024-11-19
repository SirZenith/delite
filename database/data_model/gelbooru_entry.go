package data_model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GelbooruEntry struct {
	gorm.Model

	ThumbnailURL string `gorm:"unique"`
	ContentURL   string
	FileName     string

	Tag         string
	MarkDeleted bool
	Rating      int
}

func (entry *GelbooruEntry) Upsert(db *gorm.DB) {
	db.Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			UpdateAll: true,
		},
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "thumbnail_url"}},
			DoNothing: true,
		},
	).Create(entry)
}
