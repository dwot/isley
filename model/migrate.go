package model

import (
	"database/sql"
	"embed"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	"isley/logger"
	_ "modernc.org/sqlite"
	"os"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var db *sql.DB

func InitDB() {
	var err error
	driver := os.Getenv("DB_DRIVER")

	var dsn string
	switch driver {
	case "postgres":
		dsn = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
		)
	case "sqlite", "":
		dsn = DbPath()
		driver = "sqlite"
	default:
		logger.Log.Fatalf("Unsupported DB_DRIVER: %s", driver)
	}

	logger.Log.Infof("Driver is: %s", driver)
	db, err = sql.Open(driver, dsn)
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database")
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)

	if driver == "postgres" {
		isEmpty, err := IsPostgresEmpty(db)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to check if Postgres is empty")
		} else if isEmpty {
			logger.Log.Info("Postgres database is empty, checking for SQLite migration source")
			if _, err := os.Stat("data/isley.db"); err == nil {
				err := MigrateSqliteToPostgres("data/isley.db", db)
				if err != nil {
					logger.Log.WithError(err).Error("Failed to migrate from SQLite")
				} else {
					logger.Log.Info("Migration from SQLite to Postgres completed successfully")
				}
			}
		}
	}

}

func GetDB() (*sql.DB, error) {
	stats := db.Stats()
	logger.Log.Infof("Open connections: %d", stats.OpenConnections)
	logger.Log.Infof("In-use connections: %d", stats.InUse)
	logger.Log.Infof("Idle connections: %d", stats.Idle)
	return db, nil
}

func DbPath() string {
	return "data/isley.db?_journal_mode=WAL"
}

func MigrateDB() {
	driver := os.Getenv("DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}

	if driver == "sqlite" {
		_ = os.MkdirAll("data", os.ModePerm)
		enforceWalMode()
	}

	dsn := ""
	switch driver {
	case "postgres":
		dsn = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
		)
	case "sqlite":
		dsn = DbPath()
	default:
		logger.Log.Fatalf("Unsupported DB_DRIVER: %s", driver)
	}

	logger.Log.Infof("Running migrations for %s", driver)

	db, err := sql.Open(driver, dsn)
	if err != nil {
		logger.Log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	// Use concrete types and interfaces specific to driver packages
	var m *migrate.Migrate

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		logger.Log.Fatalf("Failed to load migrations: %v", err)
	}

	switch driver {
	case "sqlite":
		sqliteDriver, err := sqlite.WithInstance(db, &sqlite.Config{})
		if err != nil {
			logger.Log.Fatalf("Failed to create SQLite driver: %v", err)
		}
		m, err = migrate.NewWithInstance("iofs", sourceDriver, "sqlite", sqliteDriver)
	case "postgres":
		postgresDriver, err := postgres.WithInstance(db, &postgres.Config{})
		if err != nil {
			logger.Log.Fatalf("Failed to create Postgres driver: %v", err)
		}
		m, err = migrate.NewWithInstance("iofs", sourceDriver, "postgres", postgresDriver)
	}
	if err != nil {
		logger.Log.Fatalf("Failed to initialize migration: %v", err)
	}

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
