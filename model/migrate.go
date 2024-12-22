package model

import (
	"database/sql"
	"embed"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"isley/logger"
	"os"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var db *sql.DB

func InitDB() {
	var err error
	db, err = sql.Open("sqlite", DbPath())
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database")
	}

	// Set connection pool limits
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
}

func GetDB() (*sql.DB, error) {
	return db, nil
}

func DbPath() string {
	return "data/isley.db?_journal_mode=WAL"
}

func MigrateDB() {
	// Ensure the data directory exists
	dataDir := "data"
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		logger.Log.Fatalf("Failed to create data directory: %v", err)
	}

	// Enforce WAL mode before running migrations
	enforceWalMode()

	logger.Log.Info("Starting database migration")

	// Open the database
	db, err := sql.Open("sqlite", DbPath())
	if err != nil {
		logger.Log.Fatalf("Error opening database: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Log.Errorf("Error closing database: %v", closeErr)
		}
	}()

	// Initialize the SQLite driver for golang-migrate
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		logger.Log.Fatalf("Failed to create SQLite driver: %v", err)
	}

	// Use iofs to load migrations from the embedded filesystem
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		logger.Log.Fatalf("Failed to load migrations from embedded filesystem: %v", err)
	}

	// Create the migrate instance
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", driver)
	if err != nil {
		logger.Log.Fatalf("Failed to initialize migration: %v", err)
	}

	// Run the migrations
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		logger.Log.Fatalf("Error applying migrations: %v", err)
	}

	if err == migrate.ErrNoChange {
		logger.Log.Info("No database migrations needed")
	} else {
		logger.Log.Info("Database migrated successfully")
	}
}

func enforceWalMode() {
	// Ensure the data directory exists
	dataDir := "data"
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		logger.Log.Fatalf("Failed to create data directory: %v", err)
	}

	// Open the database
	db, err := sql.Open("sqlite", DbPath())
	if err != nil {
		logger.Log.Errorf("Error opening database: %v", err)
		return
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Log.Errorf("Error closing database: %v", closeErr)
		}
	}()

	// Check WAL mode
	rows, err := db.Query("PRAGMA journal_mode")
	if err != nil {
		logger.Log.Errorf("Error checking WAL mode: %v", err)
		return
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logger.Log.Errorf("Error closing rows: %v", closeErr)
		}
	}()

	var mode string
	for rows.Next() {
		if err := rows.Scan(&mode); err != nil {
			logger.Log.Errorf("Error scanning WAL mode: %v", err)
			return
		}
	}

	logger.Log.Infof("Current WAL mode: %s", mode)

	if mode != "wal" {
		_, err := db.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			logger.Log.Errorf("Error setting WAL mode: %v", err)
			return
		}
		logger.Log.Info("WAL mode set successfully")
	}
}
