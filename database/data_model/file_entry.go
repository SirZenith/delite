package data_model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FileEntry struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	URL      string `gorm:"primaryKey"`
	Book     string
	Volume   string
	FileName string
}

func (entry *FileEntry) Upsert(db *gorm.DB) {
	db.Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			UpdateAll: true,
		},
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "url"}},
			DoNothing: true,
		},
	).Create(entry)
}
