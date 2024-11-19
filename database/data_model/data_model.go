package data_model

import "gorm.io/gorm"

type DataModel interface {
	Upsert(db *gorm.DB)
}
