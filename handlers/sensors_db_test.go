package handlers_test

import (
	"context"
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
	// Ingest without a zone creates sensors like this one; they used to be
	// dropped from the list.
	testutil.MustExec(t, db, `INSERT INTO sensors (id, name, zone_id, source, device, type, unit) VALUES (3, 'Loose', NULL, 'custom', 'CCC', 'temp', 'F')`)

	got := handlers.GetSensors(context.Background(), testutil.NewServices(t, db))
	require.Len(t, got, 3)

	// Ordering: source, then device.
	assert.Equal(t, "acinfinity", got[0]["source"])
	assert.Equal(t, "custom", got[1]["source"])
	assert.Equal(t, "ecowitt", got[2]["source"])

	first := got[0]
	assert.Equal(t, "Temp Probe", first["name"])
	assert.Equal(t, "Tent A", first["zone"])
	assert.Equal(t, 1, first["zone_id"])
	assert.Equal(t, "AAA", first["device"])
	assert.Equal(t, "temp", first["type"])
	assert.Equal(t, "C", first["unit"])
	assert.Equal(t, "zone_plant", first["visibility"])

	assert.Equal(t, "", got[1]["zone"])
	assert.Equal(t, 0, got[1]["zone_id"])
}

func TestGetSensors_EmptyDB(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	got := handlers.GetSensors(context.Background(), testutil.NewServices(t, db))
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// ValidateServerAddress (pure)
// ---------------------------------------------------------------------------

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
