package handlers_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// GetSensors
// ---------------------------------------------------------------------------

func TestGetSensors_OrderedAndShaped(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Tent A')`)
	testutil.MustExec(t, db, `INSERT INTO sensors (id, name, zone_id, source, device, type, unit) VALUES (1, 'Temp Probe', 1, 'acinfinity', 'AAA', 'temp', 'C')`)
	testutil.MustExec(t, db, `INSERT INTO sensors (id, name, zone_id, source, device, type, unit) VALUES (2, 'Soil Probe', 1, 'ecowitt', 'BBB', 'humi', '%')`)

	got := handlers.GetSensors(db)
	require.Len(t, got, 2)

	// Ordering: source ASC, then device ASC. acinfinity < ecowitt.
	assert.Equal(t, "acinfinity", got[0]["source"])
	assert.Equal(t, "ecowitt", got[1]["source"])

	// Shape spot-checks.
	first := got[0]
	assert.Equal(t, "Temp Probe", first["name"])
	assert.Equal(t, "Tent A", first["zone"])
	assert.Equal(t, "AAA", first["device"])
	assert.Equal(t, "temp", first["type"])
	assert.Equal(t, "C", first["unit"])
}

func TestGetSensors_EmptyDB(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	got := handlers.GetSensors(db)
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// GetZones / CreateNewZone / GetZoneIDByName
// ---------------------------------------------------------------------------

func TestZoneHelpers_RoundTrip(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	// Empty initially.
	assert.Empty(t, handlers.GetZones(db))

	id, err := handlers.CreateNewZone(db, nil, "Tent 1")
	require.NoError(t, err)
	assert.NotZero(t, id)

	zones := handlers.GetZones(db)
	require.Len(t, zones, 1)
	assert.Equal(t, "Tent 1", zones[0].Name)

	got, err := handlers.GetZoneIDByName(db, "Tent 1")
	require.NoError(t, err)
	assert.Equal(t, id, got)

	// Missing name returns (0, nil) — sentinel value, not an error.
	miss, err := handlers.GetZoneIDByName(db, "Missing")
	require.NoError(t, err)
	assert.Zero(t, miss)
}

// ---------------------------------------------------------------------------
// DeleteSensorByID — must cascade sensor_data
// ---------------------------------------------------------------------------

func TestDeleteSensorByID_CascadesSensorData(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	testutil.MustExec(t, db, `INSERT INTO sensors (id, name, zone_id, source, device, type) VALUES (1, 'Doomed', 1, 'src', 'D', 'temp')`)
	testutil.MustExec(t, db, `INSERT INTO sensor_data (sensor_id, value) VALUES (1, 1.0)`)
	testutil.MustExec(t, db, `INSERT INTO sensor_data (sensor_id, value) VALUES (1, 2.0)`)

	require.NoError(t, handlers.DeleteSensorByID(db, "1"))

	var sensorCount, dataCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensors WHERE id = 1`).Scan(&sensorCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data WHERE sensor_id = 1`).Scan(&dataCount))
	assert.Zero(t, sensorCount)
	assert.Zero(t, dataCount, "DeleteSensorByID must purge sensor_data rows for the sensor")
}

// ---------------------------------------------------------------------------
// GetSensorName
// ---------------------------------------------------------------------------

func TestGetSensorName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	testutil.MustExec(t, db, `INSERT INTO sensors (id, name, zone_id, source, device, type) VALUES (1, 'Tent Temp', 1, 'src', 'D', 'temp')`)

	assert.Equal(t, "Tent Temp", handlers.GetSensorName(db, "1"))
	assert.Equal(t, "", handlers.GetSensorName(db, "9999"), "missing id returns empty string")
}

// ---------------------------------------------------------------------------
// ValidateServerAddress (pure)
// ---------------------------------------------------------------------------

// kept as a *_db_test.go file rather than its own *_test.go to avoid
// the per-package dance of remembering which file is in which package.
func _validateServerAddressNoteForFutureMaintainers() {} //nolint:unused

func TestValidateServerAddress_TableDriven(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		address string
		ok      bool
	}{
		{"loopback IPv4", "127.0.0.1", true},
		{"private 10.x", "10.0.0.5", true},
		{"private 192.168.x", "192.168.1.100", true},
		{"private 172.16.x", "172.16.5.10", true},
		{"public IPv4 rejected", "8.8.8.8", false},
		{"loopback IPv6", "::1", true},
		{"hostname", "localhost", true},
		{"hostname with subdomain", "tent.local", true},
		{"empty rejected", "", false},
		{"hostname with bad chars", "bad host", false},
		{"hostname with slash", "bad/host", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tc.ok, handlers.ValidateServerAddress(tc.address),
				"ValidateServerAddress(%q)", tc.address)
		})
	}
}

// Compile-time assertion that GetSensorName takes a *sql.DB so the
// unit test signature stays in lockstep with the production function.
var _ func(*sql.DB, string) string = handlers.GetSensorName
