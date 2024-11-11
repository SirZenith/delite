package data_model

import "gorm.io/gorm"

type FileEntry struct {
	gorm.Model

	URL      string `gorm:"unique"`
	Book     string
	Volume   string
	FileName string
}
