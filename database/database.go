package database

import (
	"fmt"
	"log"
	"os"

	"github.com/SirZenith/delite/database/data_model"
	"github.com/glebarez/sqlite"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(filePath string) (*gorm.DB, error) {
	newLogger := logger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		logger.Config{
			// SlowThreshold:             2 * time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  true,
		},
	)

	dsn := fmt.Sprintf(
		"%s?_pragma=%s(%s)&_pragma=%s(%s)&_pragma=%s(%d)&_pragma=%s(%s)&_pragma=%s(%d)&_pragma=%s(%d)",
		filePath,
		"journal_mode", "WAL",
		"synchronous", "NORMAL",
		"busy_timeout", 10_000,
		"temp_store", "memory",
		"mmap_size", 1_000_000_000,
		"page_size", 32768,
	)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database %s: %s", filePath, err)
	}

	return db, nil
}

func LimitConnection(db *gorm.DB, num int) error {
	sqlDb, err := db.DB()
	if err != nil {
		return err
	}

	sqlDb.SetMaxOpenConns(1)

	return nil
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&data_model.FileEntry{},
		&data_model.TaggedPostEntry{},
	)
}

func GetModel(tableName string) data_model.DataModel {
	switch tableName {
	case "file_entries":
		return &data_model.FileEntry{}
	case "tagged_post_entries":
		return &data_model.TaggedPostEntry{}
	default:
		return nil
	}
}

func Close(db *gorm.DB) error {
	inner, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to close database, can't read inner data: %s", err)
	}

	err = inner.Close()
	if err != nil {
		return fmt.Errorf("failed to close inner database: %s", err)
	}

	return nil
}
