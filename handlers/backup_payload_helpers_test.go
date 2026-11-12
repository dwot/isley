package handlers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
)

// Payloads and checks shared by the SQLite and Postgres restore tests.

// jsonNum is a small helper for building row-maps that mirror what
// ParseBackupArchive returns: numeric columns come back as json.Number
// because the parser uses json.Decoder with UseNumber.
func jsonNum(s string) json.Number { return json.Number(s) }

// samplePayload mirrors the seedSampleData fixture used in
// backup_archive_test.go but as a hand-built BackupPayload. Same shape:
// settings, zones, sensors, sensor_data, breeder, strain. Lets the
// Postgres apply path exercise multi-table FK ordering, sequence reset,
// and constraint deferral without depending on a SQLite source DB.
func samplePayload() handlers.BackupPayload {
	return handlers.BackupPayload{
		Settings: []map[string]interface{}{
			{"id": jsonNum("1"), "name": "canary", "value": "green"},
			{"id": jsonNum("2"), "name": "polling_interval", "value": "60"},
		},
		// The zone works its VPD out from its own sensor: zones and
		// sensors point at each other.
		Zones: []map[string]interface{}{
			{"id": jsonNum("1"), "name": "Tent A", "vpd_temp_sensor_id": jsonNum("1"), "vpd_humidity_sensor_id": jsonNum("1")},
		},
		Sensors: []map[string]interface{}{
			{
				"id":         jsonNum("1"),
				"name":       "Tent A Temp",
				"zone_id":    jsonNum("1"),
				"source":     "acinfinity",
				"device":     "DEV1",
				"type":       "ACI.tempC",
				"unit":       "C",
				"visibility": "zone_plant",
			},
		},
		SensorData: []map[string]interface{}{
			{"id": jsonNum("1"), "sensor_id": jsonNum("1"), "value": jsonNum("21.0")},
			{"id": jsonNum("2"), "sensor_id": jsonNum("1"), "value": jsonNum("22.5")},
		},
		Breeders: []map[string]interface{}{
			{"id": jsonNum("1"), "name": "Acme Genetics"},
		},
		Strains: []map[string]interface{}{
			{
				"id":          jsonNum("1"),
				"name":        "OG Test",
				"sativa":      jsonNum("50"),
				"indica":      jsonNum("50"),
				"autoflower":  jsonNum("0"),
				"description": "desc",
				"seed_count":  jsonNum("5"),
				"breeder_id":  jsonNum("1"),
			},
		},
	}
}

// rollingAvgPayload has two sensors with 20 readings a minute apart:
// sensor 1 reads 1..20, sensor 2 reads 101..120.
func rollingAvgPayload() handlers.BackupPayload {
	p := samplePayload()
	p.Sensors = append(p.Sensors, map[string]interface{}{
		"id": jsonNum("2"), "name": "Tent A Hum", "zone_id": jsonNum("1"), "source": "acinfinity",
		"device": "DEV1", "type": "ACI.humidity", "unit": "%", "visibility": "zone_plant",
	})
	p.SensorData = nil
	id := 0
	for s := 1; s <= 2; s++ {
		for i := 1; i <= 20; i++ {
			id++
			p.SensorData = append(p.SensorData, map[string]interface{}{
				"id":        jsonNum(strconv.Itoa(id)),
				"sensor_id": jsonNum(strconv.Itoa(s)),
				"value":     jsonNum(strconv.Itoa((s-1)*100 + i)),
				"create_dt": fmt.Sprintf("2026-11-10 00:%02d:00", i),
			})
		}
	}
	return p
}

// assertRollingAverages checks each sensor's rolling average is the mean of
// the 16 readings before its newest (what the trigger leaves), and that the
// trigger is back on: a new reading moves the average.
func assertRollingAverages(t *testing.T, db *sql.DB) {
	t.Helper()
	want := map[int]float64{1: 11.5, 2: 111.5} // mean of 4..19 and 104..119
	for sensor, avg := range want {
		var got float64
		var latest time.Time
		require.NoError(t, db.QueryRow(`SELECT avg_value, create_dt FROM rolling_averages WHERE sensor_id = $1`, sensor).Scan(&got, &latest))
		assert.InDeltaf(t, avg, got, 1e-4, "sensor %d rolling average", sensor)
		assert.Equalf(t, "00:20", latest.Format("15:04"), "sensor %d rolling average is stamped with its newest reading", sensor)
	}

	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'update_rolling_avg'`).Scan(&ddl); err == nil {
		assert.Contains(t, ddl, "LIMIT 16 OFFSET 1", "SQLite gets back the trigger it had")
	}
	_, err := db.Exec(`INSERT INTO sensor_data (id, sensor_id, value, create_dt) VALUES (1000, 1, 21, '2026-11-10 00:21:00')`)
	require.NoError(t, err)
	var got float64
	require.NoError(t, db.QueryRow(`SELECT avg_value FROM rolling_averages WHERE sensor_id = 1`).Scan(&got))
	assert.InDelta(t, 12.5, got, 1e-4, "the trigger is enabled again after the restore")
}
