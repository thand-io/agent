package common

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func QueryWithParams(query string, args ...any) (string, error) {
	// Create GORM connection with in-memory SQLite
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: logger.Default.LogMode(logger.Info), // This will log SQL queries
	})
	if err != nil {
		return "", fmt.Errorf("failed to open in-memory database: %w", err)
	}

	// Get the underlying SQL DB for cleanup
	sqlDB, err := db.DB()
	if err != nil {
		return "", fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}
	defer sqlDB.Close()

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Raw(query, args...)
	})

	logrus.WithFields(logrus.Fields{
		"raw_sql": sql,
		"args":    args,
	}).Info("Generated SQL query")

	return sql, nil
}
