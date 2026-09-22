package shared

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
)

func OpenDatabase() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (example: postgres://canvas:canvas@localhost:5432/homework_canvas?sslmode=disable)")
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}
