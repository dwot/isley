package model

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	"isley/logger"
	_ "modernc.org/sqlite"
)

var conflictKeys = map[string]string{
	"settings":           "id",
	"zones":              "id",
	"sensors":            "id",
	"strain":             "id",
	"plant_status":       "id",
	"plant":              "id",
	"plant_status_log":   "id",
	"metric":             "id",
	"plant_measurements": "id",
	"activity":           "id",
	"plant_activity":     "id",
	"plant_images":       "id",
	"breeder":            "id",
	"sensor_data":        "id",
	"streams":            "id",
}

var boolToIntFields = map[string][]string{
	"strain": {"autoflower"},
}

var orderedTables = []string{
	"settings",
	"zones",
	"breeder", // Must come before strain
	"strain",
	"sensors",
	"sensor_data",
	"plant_status",
	"plant",
	"plant_status_log",
	"metric",
	"plant_measurements",
	"activity",
	"plant_activity",
	"plant_images",
	"streams",
}

func MigrateSqliteToPostgres(sqlitePath string, pg *sql.DB) error {
	sqliteDB, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer sqliteDB.Close()

	tables := orderedTables

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
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'`)
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
	const batchSize = 5000

	// 🔻 Disable triggers (only works in Postgres)
	if IsPostgres() {
		_, err := dest.Exec(fmt.Sprintf("ALTER TABLE %s DISABLE TRIGGER ALL", table))
		if err != nil {
			return fmt.Errorf("disabling triggers on %s: %w", table, err)
		}
	}

	// Count total rows (for logging)
	var totalRows int
	err := src.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&totalRows)
	if err != nil {
		return fmt.Errorf("counting rows in %s: %w", table, err)
	}

	offset := 0
	for offset < totalRows {
		query := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", table, batchSize, offset)
		rows, err := src.Query(query)
		if err != nil {
			return err
		}

		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}

		colCount := len(cols)
		valuesList := [][]interface{}{}

		for rows.Next() {
			values := make([]interface{}, colCount)
			ptrs := make([]interface{}, colCount)
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}

			// Normalize booleans that are strings ("true"/"false") to 1/0
			for i, col := range cols {
				for _, bfield := range boolToIntFields[table] {
					if col == bfield {
						if s, ok := values[i].(string); ok {
							if strings.EqualFold(s, "true") {
								values[i] = 1
							} else if strings.EqualFold(s, "false") {
								values[i] = 0
							}
						}
					}
				}
			}

			valuesList = append(valuesList, values)
		}
		rows.Close()

		if len(valuesList) == 0 {
			break
		}

		// Build batch insert
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", table, strings.Join(cols, ","))
		args := []interface{}{}
		placeholders := []string{}

		argCounter := 1
		for _, vals := range valuesList {
			ph := []string{}
			for range vals {
				ph = append(ph, fmt.Sprintf("$%d", argCounter))
				argCounter++
			}
			placeholders = append(placeholders, fmt.Sprintf("(%s)", strings.Join(ph, ",")))
			args = append(args, vals...)
		}

		insertSQL += strings.Join(placeholders, ",")

		if conflictKey, ok := conflictKeys[table]; ok {
			insertSQL += fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", conflictKey)
		}

		_, err = dest.Exec(insertSQL, args...)
		if err != nil {
			return fmt.Errorf("executing batch insert into %s: %w", table, err)
		}

		offset += len(valuesList)
		logger.Log.Infof("Migrated %d rows from %s (%d/%d)", len(valuesList), table, offset, totalRows)
	}

	if IsPostgres() && hasSerialID(table) {
		_, err := dest.Exec(fmt.Sprintf(`
        SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1), true) FROM %s
    `, table, table))
		if err != nil {
			return fmt.Errorf("reset sequence for table %s: %w", table, err)
		}
	}

	// 🔺 Re-enable triggers
	if IsPostgres() {
		_, err := dest.Exec(fmt.Sprintf("ALTER TABLE %s ENABLE TRIGGER ALL", table))
		if err != nil {
			return fmt.Errorf("re-enabling triggers on %s: %w", table, err)
		}
	}

	return nil
}

func hasSerialID(table string) bool {
	// List of tables where 'id' is a SERIAL/identity column and needs sequence reset
	serialTables := map[string]bool{
		"settings":           true,
		"zones":              true,
		"sensors":            true,
		"sensor_data":        true,
		"strain":             true,
		"plant_status":       true,
		"plant":              true,
		"plant_status_log":   true,
		"metric":             true,
		"plant_measurements": true,
		"activity":           true,
		"plant_activity":     true,
		"plant_images":       true,
		"breeder":            true,
		"streams":            true,
	}

	return serialTables[table]
}

func IsPostgresEmpty(db *sql.DB) (bool, error) {
	var tablesToCheck = []string{"sensors", "sensor_data", "plant"}

	for _, table := range tablesToCheck {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		err := db.QueryRow(query).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("error checking table %s: %w", table, err)
		}
		if count > 0 {
			return false, nil // Data exists
		}
	}
	return true, nil // No rows found in any key table
}
