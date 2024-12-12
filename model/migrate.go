package model

import (
	"database/sql"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"log"
)

func DbPath() string {
	return "data/isley.db?_journal_mode=WAL"
}

func MigrateDB() {
	// Init the db
	//Check to see if WAL mode is enabled
	enforceWalMode()

	fmt.Println("Migrating database")
	m, err := migrate.New(
		"file://migrations",
		"sqlite://"+DbPath())
	if err != nil {
		fmt.Print(err)
	}
	err = m.Up()
	if err != nil {
		fmt.Print(err)
	}

}

func enforceWalMode() {
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
