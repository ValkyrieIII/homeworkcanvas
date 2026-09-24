package shared

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
)

func OpenDatabase() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (example: postgres://canvas:canvas@localhost:5432/homework_canvas?sslmode=disable)")
	}
	// GORM's default SQL error log can print bound values, including imported text.
	// Application errors are recorded with request/job identifiers instead.
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}
