package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/SirZenith/delite/database/data_model"
	"github.com/glebarez/sqlite"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(filePath string) (*gorm.DB, error) {
	newLogger := logger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             2 * time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  true,
		},
	)

	db, err := gorm.Open(sqlite.Open(filePath), &gorm.Config{

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
	)
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
