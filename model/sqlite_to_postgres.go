package model

import (
	"database/sql"
	"fmt"
	_ "os"
	"strings"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"isley/logger"
)

func MigrateSqliteToPostgres(sqlitePath string, pg *sql.DB) error {
	sqliteDB, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer sqliteDB.Close()

	tables, err := listSQLiteTables(sqliteDB)
	if err != nil {
		return fmt.Errorf("list sqlite tables: %w", err)
	}

	for _, table := range tables {
		logger.Log.Infof("Migrating table: %s", table)
		if err := copyTableData(sqliteDB, pg, table); err != nil {
			return fmt.Errorf("copy table %s: %w", table, err)
		}
	}

	return nil
}

func listSQLiteTables(db *sql.DB) ([]string, error) {
	tables := []string{}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, nil
}

func copyTableData(src *sql.DB, dest *sql.DB, table string) error {
	rows, err := src.Query(fmt.Sprintf("SELECT * FROM %s", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ","),
		strings.Join(placeholders, ","),
	)

	tx, err := dest.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		if _, err := stmt.Exec(values...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func IsPostgresEmpty(db *sql.DB) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}
