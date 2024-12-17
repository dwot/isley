package model

import (
	"database/sql"
	"embed"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"log"
	"os"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func DbPath() string {
	return "data/isley.db?_journal_mode=WAL"
}

func MigrateDB() {
	// Ensure the data directory exists
	dataDir := "data"
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Enforce WAL mode before running migrations
	enforceWalMode()

	fmt.Println("Migrating database")

	// Open the database
	db, err := sql.Open("sqlite", DbPath())
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	// Initialize the SQLite driver for golang-migrate
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		log.Fatalf("Failed to create SQLite driver: %v", err)
	}

	// Use iofs to load migrations from the embedded filesystem
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		log.Fatalf("Failed to load migrations from embedded filesystem: %v", err)
	}

	// Create the migrate instance
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", driver)
	if err != nil {
		log.Fatalf("Failed to initialize migration: %v", err)
	}

	// Run the migrations
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Error applying migrations: %v", err)
	}

	fmt.Println("Database migrated successfully")
}

func enforceWalMode() {
	// Ensure the data directory exists
	dataDir := "data"
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Open the database
	db, err := sql.Open("sqlite", DbPath())
	if err != nil {
		log.Println("Error opening database:", err)
		return
	}
	defer db.Close()

	// Check WAL mode
	rows, err := db.Query("PRAGMA journal_mode")
	if err != nil {
		log.Println("Error checking WAL mode:", err)
		return
	}
	defer rows.Close()

	var mode string
	for rows.Next() {
		err = rows.Scan(&mode)
		if err != nil {
			log.Println("Error scanning WAL mode:", err)
			return
		}
	}

	fmt.Println("Current WAL mode:", mode)

	if mode != "wal" {
		_, err = db.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			log.Println("Error setting WAL mode:", err)
			return
		}
		fmt.Println("WAL mode set")
	}
}
