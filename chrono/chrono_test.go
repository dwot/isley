package chrono

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

func zonesIn(tz string, system *time.Location) *Zones {
	return NewZones(func() string { return tz }, system)
}

// naive mimics what SQL drivers return for a naive TIMESTAMP: the stored
// digits tagged UTC.
func naive(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDate_ParseFormatAndJSON(t *testing.T) {
	t.Parallel()
	d, err := ParseDate("2026-11-01")
	require.NoError(t, err)
	assert.Equal(t, Date{2026, time.November, 1}, d)
	assert.Equal(t, "2026-11-01", d.String())

	b, err := json.Marshal(struct {
		D Date `json:"d"`
		Z Date `json:"z"`
	}{D: d})
	require.NoError(t, err)
	assert.JSONEq(t, `{"d":"2026-11-01","z":null}`, string(b))

	var back struct{ D, Z Date }
	require.NoError(t, json.Unmarshal([]byte(`{"D":"2026-02-28","Z":null}`), &back))
	assert.Equal(t, Date{2026, time.February, 28}, back.D)
	assert.True(t, back.Z.IsZero())

	for _, bad := range []string{"2026-11-1", "11/01/2026", "2026-02-30", "2026-11-01T00:00"} {
		_, err := ParseDate(bad)
		assert.Error(t, err, bad)
	}
	assert.Error(t, json.Unmarshal([]byte(`{"D":5}`), &back))
}

func TestDate_Scan(t *testing.T) {
	t.Parallel()
	want := Date{2026, time.November, 1}
	for _, src := range []interface{}{
		"2026-11-01",
		"2026-11-01T21:30",
		"2026-11-01 23:59:59",
		[]byte("2026-11-01"),
		time.Date(2026, 11, 1, 23, 0, 0, 0, time.UTC),
		// A driver tagging the stored digits with some offset: the digits
		// are the day, not the instant.
		time.Date(2026, 11, 1, 23, 0, 0, 0, time.FixedZone("x", -5*3600)),
	} {
		var d Date
		require.NoError(t, d.Scan(src), "%v", src)
		assert.Equal(t, want, d, "%v", src)
	}
	var d Date
	require.NoError(t, d.Scan(nil))
	assert.True(t, d.IsZero())
	assert.Error(t, d.Scan(42))
	assert.Error(t, d.Scan("nope"))

	v, err := want.Value()
	require.NoError(t, err)
	assert.Equal(t, "2026-11-01", v)
	v, err = Date{}.Value()
	require.NoError(t, err)
	assert.Nil(t, v)
}

func TestDate_Arithmetic(t *testing.T) {
	t.Parallel()
	// Across the US DST change: still whole days.
	a := Date{2026, time.March, 7}
	b := Date{2026, time.March, 9}
	assert.Equal(t, 2, b.DaysSince(a))
	assert.Equal(t, -2, a.DaysSince(b))
	assert.Equal(t, b, a.AddDays(2))
	assert.Equal(t, Date{2027, time.January, 1}, Date{2026, time.December, 31}.AddDays(1))
	assert.True(t, a.Before(b))
	assert.False(t, b.Before(a))
}

func TestPlantAge(t *testing.T) {
	t.Parallel()
	start := Date{2026, time.November, 1}
	cases := []struct {
		today     Date
		day, week int
	}{
		{start, 1, 1},
		{start.AddDays(6), 7, 1},
		{start.AddDays(7), 8, 2},
		{start.AddDays(13), 14, 2},
		{start.AddDays(14), 15, 3},
		{start.AddDays(-1), 0, 0},
		{start.AddDays(-9), -8, 0},
	}
	for _, c := range cases {
		day, week := PlantAge(start, c.today)
		assert.Equal(t, c.day, day, c.today.String())
		assert.Equal(t, c.week, week, c.today.String())
	}
}

func TestZones_Instance(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "America/New_York", zonesIn("America/New_York", nil).Instance().String())
	assert.Equal(t, time.Local, zonesIn("", nil).Instance())
	assert.Equal(t, time.Local, zonesIn("Not/AZone", nil).Instance())
	assert.Equal(t, time.UTC, NewZones(nil, nil).System())

	// The setting is read on every call.
	tz := "Asia/Kolkata"
	z := NewZones(func() string { return tz }, nil)
	assert.Equal(t, "Asia/Kolkata", z.Instance().String())
	tz = "Europe/Berlin"
	assert.Equal(t, "Europe/Berlin", z.Instance().String())
}

func TestZones_FromWall(t *testing.T) {
	t.Parallel()
	z := zonesIn("America/New_York", time.UTC)
	got := z.FromWall(naive("2026-11-10T21:30:00"))
	assert.Equal(t, "2026-11-10T21:30:00-05:00", got.Format(time.RFC3339))
	assert.True(t, z.FromWall(time.Time{}).IsZero())

	// Summer: same digits, daylight offset.
	assert.Equal(t, "2026-07-01T09:00:00-04:00", z.FromWall(naive("2026-07-01T09:00:00")).Format(time.RFC3339))
}

func TestZones_FromSystem(t *testing.T) {
	t.Parallel()
	// SQLite / default Postgres: CURRENT_TIMESTAMP is UTC.
	z := zonesIn("America/New_York", time.UTC)
	got := z.FromSystem(naive("2026-11-11T18:35:10"))
	assert.Equal(t, "2026-11-11T13:35:10-05:00", got.Format(time.RFC3339))

	// A Postgres server running in another zone.
	berlin := mustLoc(t, "Europe/Berlin")
	z = zonesIn("Asia/Kolkata", berlin)
	got = z.FromSystem(naive("2026-11-11T12:00:00"))
	assert.Equal(t, "2026-11-11T16:30:00+05:30", got.Format(time.RFC3339))
	assert.True(t, z.FromSystem(time.Time{}).IsZero())
}

func TestZones_ToWallAndToSystem(t *testing.T) {
	t.Parallel()
	instant := time.Date(2026, 11, 11, 2, 30, 0, 0, time.UTC)

	assert.Equal(t, "2026-11-10T21:30:00", zonesIn("America/New_York", time.UTC).ToWall(instant))
	assert.Equal(t, "2026-11-11T08:00:00", zonesIn("Asia/Kolkata", time.UTC).ToWall(instant))
	assert.Equal(t, "2026-11-11 02:30:00", zonesIn("Asia/Kolkata", time.UTC).ToSystem(instant))
	assert.Equal(t, "2026-11-11 03:30:00", zonesIn("UTC", mustLoc(t, "Europe/Berlin")).ToSystem(instant))
}

func TestZones_WallRoundTrip(t *testing.T) {
	t.Parallel()
	for _, tz := range []string{"America/New_York", "Asia/Kolkata", "UTC", "Australia/Lord_Howe"} {
		z := zonesIn(tz, time.UTC)
		stored := "2026-11-10T21:30:00"
		back := z.ToWall(z.FromWall(naive(stored)))
		assert.Equal(t, stored, back, tz)
	}
}

func TestZones_ParseInput(t *testing.T) {
	t.Parallel()
	z := zonesIn("America/New_York", time.UTC)
	cases := map[string]string{
		"2026-11-11T11:45:00-05:00":   "2026-11-11T16:45:00Z",
		"2026-11-11T16:45:00Z":        "2026-11-11T16:45:00Z",
		"2026-11-11T22:15:00+05:30":   "2026-11-11T16:45:00Z",
		"2026-11-11T11:45:00.5-05:00": "2026-11-11T16:45:00.5Z",
		"2026-11-11T11:45:00":         "2026-11-11T16:45:00Z",
		"2026-11-11T11:45":            "2026-11-11T16:45:00Z",
		"2026-11-11 11:45:00":         "2026-11-11T16:45:00Z",
		" 2026-11-11 11:45 ":          "2026-11-11T16:45:00Z",
	}
	for in, want := range cases {
		got, err := z.ParseInput(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got.UTC().Format(time.RFC3339Nano), in)
	}
	for _, bad := range []string{"", "2026-11-11", "11/11/2026 11:45", "yesterday"} {
		_, err := z.ParseInput(bad)
		assert.Error(t, err, bad)
	}
}

func TestZones_TodayAndNow(t *testing.T) {
	t.Parallel()
	// 03:00 UTC on the 11th is still the evening of the 10th in New York
	// and already morning of the 11th in Kolkata.
	clock := Fixed(time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC))
	assert.Equal(t, Date{2026, time.November, 10}, zonesIn("America/New_York", nil).Today(clock))
	assert.Equal(t, Date{2026, time.November, 11}, zonesIn("Asia/Kolkata", nil).Today(clock))
	assert.Equal(t, "2026-11-10T22:00:00-05:00", zonesIn("America/New_York", nil).Now(clock).Format(time.RFC3339))
	assert.Equal(t, Date{2026, time.November, 11}, zonesIn("Asia/Kolkata", nil).DateOf(clock.Now()))
}

func TestFixedClock(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := Fixed(start)
	assert.Equal(t, start, c.Now())
	c.Advance(90 * time.Minute)
	assert.Equal(t, start.Add(90*time.Minute), c.Now())
	c.Set(start)
	assert.Equal(t, start, c.Now())

	assert.WithinDuration(t, time.Now(), System().Now(), time.Second)
}

func TestZones_InstanceName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Asia/Kolkata", zonesIn("Asia/Kolkata", nil).InstanceName())
	// With no setting it is a real zone name, never "Local" on a normal
	// Linux host or container.
	name := zonesIn("", nil).InstanceName()
	assert.Equal(t, LocalZoneName(), name)
	assert.NotEmpty(t, name)
}

func TestStored_Scan(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 11, 11, 21, 30, 0, 0, time.UTC)
	for _, src := range []any{
		"2026-11-11 21:30:00",
		"2026-11-11T21:30:00",
		"2026-11-11T21:30",
		[]byte("2026-11-11 21:30"),
		"2026-11-11T21:30:00Z",
		"2026-11-11 21:30:00+00:00",
		time.Date(2026, 11, 11, 21, 30, 0, 0, time.FixedZone("x", -5*3600)),
	} {
		var s Stored
		require.NoError(t, s.Scan(src), "%v", src)
		assert.True(t, s.Valid)
		assert.Equal(t, want, s.Time, "%v", src)
	}

	var s Stored
	require.NoError(t, s.Scan("2026-11-11"))
	assert.Equal(t, time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), s.Time)
	require.NoError(t, s.Scan(nil))
	assert.False(t, s.Valid)
	assert.Error(t, s.Scan("yesterday"))
	assert.Error(t, s.Scan(42))
}
