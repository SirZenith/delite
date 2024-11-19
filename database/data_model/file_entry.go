package data_model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FileEntry struct {
	gorm.Model

	URL      string `gorm:"unique"`
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
