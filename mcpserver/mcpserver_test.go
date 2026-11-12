package mcpserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/tests/testutil"
)

// fixedNow is 08:00 on 11 Nov 2026 in New York.
var fixedNow = time.Date(2026, 11, 11, 13, 0, 0, 0, time.UTC)

type fixture struct {
	server *testutil.TestServer
	store  *config.Store
	db     *sql.DB
	plant  int
	strain int
	zone   int
	temp   int
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return newFixtureOn(t, testutil.NewTestDB(t))
}

func newFixtureOn(t *testing.T, db *sql.DB, opts ...testutil.ServerOption) fixture {
	t.Helper()
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	store.SetMCPEnabled(1)
	server := testutil.NewTestServer(t, db, append([]testutil.ServerOption{
		testutil.WithConfigStore(store),
		testutil.WithUploadDir(t.TempDir()),
		testutil.WithClock(chrono.Fixed(fixedNow))}, opts...)...)

	f := fixture{server: server, store: store, db: db}
	f.strain = testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "Mephisto"), "Sour Crack")
	f.zone = testutil.SeedZone(t, db, "Tent")
	f.plant = testutil.SeedPlant(t, db, "Lady", f.strain, f.zone)
	testutil.MustExec(t, db, "UPDATE plant SET start_dt = '2026-09-01' WHERE id = $1", f.plant)
	testutil.MustExec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-10-20 14:00:00')",
		f.plant, lookupID(t, db, "plant_status", "status", "Flower"))

	// A night in the tent: 20:00-08:00 New York, one reading an hour,
	// coolest at 04:00.
	f.temp = testutil.SeedSensor(t, db, "acinfinity", "ctrl", "temperature")
	testutil.MustExec(t, db, "UPDATE sensors SET name = 'Tent Temp', zone_id = $1, unit = '°F' WHERE id = $2", f.zone, f.temp)
	night := time.Date(2026, 11, 11, 1, 0, 0, 0, time.UTC) // 20:00 on the 10th in New York
	for h := 0; h <= 12; h++ {
		v := 70 + float64((h-8)*(h-8))/4 // 70 at 04:00, 86 at 20:00
		testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)",
			f.temp, v, night.Add(time.Duration(h)*time.Hour).Format("2006-01-02 15:04:05"))
	}
	return f
}

func lookupID(t *testing.T, db *sql.DB, table, column, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow("SELECT id FROM "+table+" WHERE "+column+" = $1", name).Scan(&id))
	return id
}

func (f fixture) key(t *testing.T, scopes string) string {
	t.Helper()
	return testutil.SeedAPIKeyWithScopes(t, f.db, "mcp-"+strings.ReplaceAll(scopes, ",", "-")+"-00000000000000000000", scopes)
}

type bearer struct {
	key  string
	base http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.key)
	return b.base.RoundTrip(r)
}

// connect opens an MCP session to the test server with key.
func (f fixture) connect(t *testing.T, key string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   f.server.URL + "/mcp",
		HTTPClient: &http.Client{Transport: bearer{key: key, base: http.DefaultTransport}},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// call runs a tool and splits its text into the summary line and the
// JSON data.
func call(t *testing.T, s *mcp.ClientSession, tool string, args map[string]any) (summary string, data json.RawMessage, isError bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	text := res.Content[0].(*mcp.TextContent).Text
	if res.IsError {
		return text, nil, true
	}
	summary, body, ok := strings.Cut(text, "\n")
	require.True(t, ok, "summary line then JSON: %s", text)
	require.True(t, json.Valid([]byte(body)), "not JSON: %s", body)
	return summary, json.RawMessage(body), false
}

func toolNames(t *testing.T, s *mcp.ClientSession) []string {
	t.Helper()
	res, err := s.ListTools(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "private", res.CacheScope, "the list depends on the key")
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if strings.HasPrefix(tool.Name, "get_") || strings.HasPrefix(tool.Name, "list_") || tool.Name == "find_strains" {
			assert.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
		} else {
			assert.False(t, tool.Annotations.ReadOnlyHint, tool.Name)
			require.NotNil(t, tool.Annotations.DestructiveHint, tool.Name)
			assert.False(t, *tool.Annotations.DestructiveHint, tool.Name)
		}
	}
	sort.Strings(names)
	return names
}

var readTools = []string{"find_strains", "get_overview", "get_plant", "get_sensor_history", "get_strain",
	"list_plants", "list_reference_data", "list_zones"}

var writeTools = []string{"adjust_seed_count", "create_plant", "link_plant_sensors", "log_activity",
	"record_measurement", "set_plant_status", "update_plant"}

func TestMCP_OffByDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.store.SetMCPEnabled(0)
	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/mcp", strings.NewReader("{}"))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+f.key(t, "admin"))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	assert.Zero(t, config.NewStore().MCPEnabled(), "a new install has it off")
}

func TestMCP_Auth(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	post := func(key string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, f.server.URL+"/mcp", strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		testutil.DrainAndClose(resp)
		return resp
	}
	assert.Equal(t, http.StatusUnauthorized, post("").StatusCode)
	assert.Equal(t, http.StatusUnauthorized, post("not-a-key").StatusCode)
	assert.Equal(t, http.StatusForbidden, post(f.key(t, "ingest")).StatusCode, "an ingest-only key can't read")
}

// A read key can answer "how did the tent do overnight?" and sees no
// write tools.
func TestMCP_ReadKey(t *testing.T) {
	t.Parallel()
	checkReadKey(t, newFixture(t))
}

func checkReadKey(t *testing.T, f fixture) {
	s := f.connect(t, f.key(t, "read"))
	assert.Equal(t, readTools, toolNames(t, s))

	summary, data, isErr := call(t, s, "get_overview", nil)
	require.False(t, isErr, summary)
	assert.Contains(t, summary, "1 plant growing, 1 zone")
	assert.Contains(t, summary, "2026-11-11T08:00:00-05:00 (America/New_York)")
	var ov struct {
		Plants []struct {
			Name      string `json:"name"`
			Status    string `json:"status"`
			Day       int    `json:"day"`
			FlowerDay int    `json:"flower_day"`
			Zone      string `json:"zone"`
		} `json:"growing_plants"`
		Zones []struct {
			Name    string `json:"name"`
			Sensors []struct {
				ID     int `json:"id"`
				Latest struct {
					Value float64 `json:"value"`
					At    string  `json:"at"`
				} `json:"latest"`
			} `json:"sensors"`
		} `json:"zones"`
	}
	require.NoError(t, json.Unmarshal(data, &ov))
	require.Len(t, ov.Plants, 1)
	assert.Equal(t, "Flower", ov.Plants[0].Status)
	assert.Equal(t, 72, ov.Plants[0].Day, "1 Sep is day 1")
	assert.Equal(t, 23, ov.Plants[0].FlowerDay, "20 Oct is day 1 of flower")
	require.Len(t, ov.Zones, 1)
	require.Len(t, ov.Zones[0].Sensors, 1)
	assert.Equal(t, "2026-11-11T08:00:00-05:00", ov.Zones[0].Sensors[0].Latest.At)

	summary, data, isErr = call(t, s, "get_sensor_history", map[string]any{
		"zone_id": f.zone, "from": "2026-11-10T20:00", "to": "2026-11-11T08:00",
	})
	require.False(t, isErr, summary)
	assert.Contains(t, summary, "Tent Temp: min 70°F")
	assert.Contains(t, summary, "max 86°F")
	var hist struct {
		From    string `json:"from"`
		Bucket  string `json:"bucket"`
		Sensors []struct {
			Readings int     `json:"readings"`
			Min      float64 `json:"min"`
			Buckets  []struct {
				Start string  `json:"start"`
				Min   float64 `json:"min"`
			} `json:"buckets"`
		} `json:"sensors"`
	}
	require.NoError(t, json.Unmarshal(data, &hist))
	assert.Equal(t, "2026-11-10T20:00:00-05:00", hist.From)
	assert.Equal(t, "30m", hist.Bucket, "12 hours in about 24 buckets")
	require.Len(t, hist.Sensors, 1)
	assert.Equal(t, 12, hist.Sensors[0].Readings, "[from, to): 08:00 is out")
	assert.Equal(t, 70.0, hist.Sensors[0].Min)

	// Write tools aren't there for a read key.
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "log_activity",
		Arguments: map[string]any{"plant_ids": []int{f.plant}, "activity": "Water"}})
	assert.True(t, err != nil || res.IsError, "a read key can't call log_activity")
}

// A write key can create a plant and log a watering, by names.
func TestMCP_WriteKey(t *testing.T) {
	t.Parallel()
	checkWriteKey(t, newFixture(t))
}

func checkWriteKey(t *testing.T, f fixture) {
	s := f.connect(t, f.key(t, "write"))
	assert.Equal(t, mergeSorted(readTools, writeTools), toolNames(t, s))

	testutil.MustExec(t, f.db, "UPDATE strain SET seed_count = 5 WHERE id = $1", f.strain)
	summary, data, isErr := call(t, s, "create_plant", map[string]any{
		"name": "Lady 2", "strain_id": f.strain, "status": "seedling", "zone_id": f.zone,
		"decrement_seed_count": true,
	})
	require.False(t, isErr, summary)
	var created struct {
		ID        int    `json:"id"`
		StartDate string `json:"start_date"`
		Status    string `json:"status"`
		Day       int    `json:"day"`
	}
	require.NoError(t, json.Unmarshal(data, &created))
	assert.Equal(t, "2026-11-11", created.StartDate, "today in New York")
	assert.Equal(t, "Seedling", created.Status)
	assert.Equal(t, 1, created.Day)
	assert.Contains(t, summary, "Created plant")
	var seeds int
	require.NoError(t, f.db.QueryRow("SELECT seed_count FROM strain WHERE id = $1", f.strain).Scan(&seeds))
	assert.Equal(t, 4, seeds)

	summary, data, isErr = call(t, s, "log_activity", map[string]any{
		"plant_ids": []int{f.plant, created.ID}, "activity": "water", "at": "2026-11-11T07:30",
		"note": "pH 6.3",
	})
	require.False(t, isErr, summary)
	assert.Equal(t, "Logged Water for Lady, Lady 2 at 2026-11-11T07:30:00-05:00.", summary)
	var stored chrono.Stored
	require.NoError(t, f.db.QueryRow("SELECT date FROM plant_activity WHERE plant_id = $1", created.ID).Scan(&stored))
	assert.Equal(t, "2026-11-11 12:30:00", stored.Time.Format("2006-01-02 15:04:05"), "stored in UTC")

	_, data, isErr = call(t, s, "get_plant", map[string]any{"plant_id": created.ID})
	require.False(t, isErr)
	assert.Contains(t, string(data), `"days_since_watered":0`)

	summary, _, isErr = call(t, s, "record_measurement", map[string]any{"plant_id": f.plant, "metric": "height", "value": 31.5})
	require.False(t, isErr, summary)
	assert.Equal(t, "Recorded Height 31.5 in for plant "+strconv.Itoa(f.plant)+" at 2026-11-11T08:00:00-05:00.", summary)

	summary, _, isErr = call(t, s, "set_plant_status", map[string]any{"plant_id": f.plant, "status": "Drying",
		"harvest_weight_g": 112.5})
	require.False(t, isErr, summary)
	var weight float64
	require.NoError(t, f.db.QueryRow("SELECT harvest_weight FROM plant WHERE id = $1", f.plant).Scan(&weight))
	assert.Equal(t, 112.5, weight)
	summary, _, isErr = call(t, s, "set_plant_status", map[string]any{"plant_id": f.plant, "status": "Drying"})
	require.False(t, isErr, summary)
	assert.Contains(t, summary, "Already Drying; nothing changed.")

	summary, _, isErr = call(t, s, "update_plant", map[string]any{"plant_id": created.ID, "name": "Lady II"})
	require.False(t, isErr, summary)
	assert.Contains(t, summary, "Updated: Lady II")

	summary, _, isErr = call(t, s, "adjust_seed_count", map[string]any{"strain_id": f.strain, "delta": 10, "reason": "new pack"})
	require.False(t, isErr, summary)
	assert.Equal(t, "Sour Crack now has 14 seeds.", summary)

	summary, _, isErr = call(t, s, "link_plant_sensors", map[string]any{"plant_id": created.ID, "sensor_ids": []int{f.temp}})
	require.False(t, isErr, summary)
	assert.Equal(t, "Lady II now has 1 sensor linked.", summary)
}

func TestMCP_ErrorsSayWhatToFix(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := f.connect(t, f.key(t, "write"))

	text, _, isErr := call(t, s, "set_plant_status", map[string]any{"plant_id": f.plant, "status": "Bloom"})
	require.True(t, isErr)
	assert.Contains(t, text, `"Bloom" is not one of`)
	assert.Contains(t, text, "Flower")

	text, _, isErr = call(t, s, "get_plant", map[string]any{"plant_id": 9999})
	require.True(t, isErr)
	assert.Contains(t, text, "plant_not_found")

	text, _, isErr = call(t, s, "log_activity", map[string]any{"plant_ids": []int{f.plant}, "activity": "Water", "at": "yesterday"})
	require.True(t, isErr)
	assert.Contains(t, text, "at: must be a date-time")

	text, _, isErr = call(t, s, "get_sensor_history", map[string]any{})
	require.True(t, isErr)
	assert.Contains(t, text, "give sensor_ids or zone_id")
}

func mergeSorted(a, b []string) []string {
	out := append(append([]string{}, a...), b...)
	sort.Strings(out)
	return out
}
