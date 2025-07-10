package data_model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaggedPostEntry struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`


	ThumbnailURL string `gorm:"primaryKey"`
	ContentURL   string
	FileName     string

	Tag         string
	MarkDeleted bool
	Rating      int

	DlFailed bool // Mark true when last download attempt for this entry failed
}

func (entry *TaggedPostEntry) Upsert(db *gorm.DB) {
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
