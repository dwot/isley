package service

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"isley/chrono"
)

const (
	// MinBucket and MaxBuckets bound bucketed history: no finer than a
	// minute, no more than MaxBuckets buckets per call.
	MinBucket  = time.Minute
	MaxBuckets = 5000
	// MaxRawPoints caps raw readings per call.
	MaxRawPoints = 10000
)

// Point is one raw reading.
type Point struct {
	ID    int
	At    time.Time
	Value float64
}

// Bucket summarises the readings in [Start, Start+bucket).
type Bucket struct {
	Start time.Time
	Min   float64
	Avg   float64
	Max   float64
	Count int
	// OnFraction and Starts are set for on/off sensors (ports): the share
	// of readings that were on, and how often it went from off to on.
	OnFraction *float64
	Starts     *int
}

// IsOnOffSensor reports whether a sensor of this type is a port whose
// reading is a speed or level (0 = off), as AC Infinity ports are.
func IsOnOffSensor(sensorType string) bool {
	return strings.HasPrefix(sensorType, "ACIP.")
}

// ParseBucket reads a bucket size: Go durations ("5m", "1h30m") or whole
// days ("1d", "7d").
func ParseBucket(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid bucket %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid bucket %q", s)
	}
	return d, nil
}

// FormatBucket writes a bucket size the way ParseBucket reads it.
func FormatBucket(d time.Duration) string {
	if d >= 24*time.Hour && d%(24*time.Hour) == 0 {
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	}
	// Duration.String spells out zero parts: "30m0s", "1h0m0s".
	out := d.String()
	if strings.HasSuffix(out, "m0s") {
		out = strings.TrimSuffix(out, "0s")
	}
	if strings.HasSuffix(out, "h0m") {
		out = strings.TrimSuffix(out, "0m")
	}
	return out
}

// DefaultBucket picks the smallest of the usual sizes that keeps a range
// to about 300 buckets.
func DefaultBucket(from, to time.Time) time.Duration {
	span := to.Sub(from)
	for _, d := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour,
		6 * time.Hour, 24 * time.Hour} {
		if span/d <= 300 {
			return d
		}
	}
	return 7 * 24 * time.Hour
}

// Raw returns readings in [from, to), oldest first, after the reading
// after (0 to start), at most limit of them. more says whether there are
// further readings in the range.
func (s *SensorService) Raw(ctx context.Context, id int, from, to time.Time, after, limit int) ([]Point, bool, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, false, err
	}
	if err := checkRange(from, to); err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > MaxRawPoints {
		limit = MaxRawPoints
	}
	z := s.deps.Zones
	args := []any{id, z.ToSystem(from), z.ToSystem(to)}
	cond := ""
	if after > 0 {
		// Readings are ordered by time, then id; continue past the given one.
		args = append(args, after)
		cond = ` AND (create_dt > (SELECT create_dt FROM sensor_data WHERE id = $4)
			OR (create_dt = (SELECT create_dt FROM sensor_data WHERE id = $4) AND id > $4))`
	}
	rows, err := s.deps.DB.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, create_dt, value FROM sensor_data
		WHERE sensor_id = $1 AND create_dt >= $2 AND create_dt < $3%s
		ORDER BY create_dt, id LIMIT %d`, cond, limit+1), args...)
	if err != nil {
		return nil, false, fmt.Errorf("sensor %d readings: %w", id, err)
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var (
			p  Point
			at chrono.Stored
		)
		if err := rows.Scan(&p.ID, &at, &p.Value); err != nil {
			return nil, false, fmt.Errorf("scan reading: %w", err)
		}
		p.At = z.FromSystem(at.Time)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("sensor %d readings: %w", id, err)
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

func checkRange(from, to time.Time) error {
	if !from.Before(to) {
		return Invalid(map[string]string{"from": "must be before to"})
	}
	return nil
}

// Buckets summarises readings in [from, to) by bucket size, oldest first;
// buckets with no readings are left out. Buckets line up with midnight in
// the instance zone; whole-day buckets follow the calendar. Whole-hour
// buckets read the hourly rollup where it is complete and raw readings
// for the rest; on/off sensors always read raw readings.
//
// A rollup hour is taken as complete; the rollup job keeps re-summarising
// the last 25 hours, so only the newest hour can be partial.
func (s *SensorService) Buckets(ctx context.Context, id int, from, to time.Time, size time.Duration) ([]Bucket, error) {
	sn, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := checkRange(from, to); err != nil {
		return nil, err
	}
	if size < MinBucket {
		return nil, Invalid(map[string]string{"bucket": "must be at least 1m"})
	}
	if n := to.Sub(from) / size; n > MaxBuckets {
		return nil, Invalid(map[string]string{"bucket": fmt.Sprintf(
			"gives %d buckets over this range; at most %d (use a bigger bucket or a shorter range)", n, MaxBuckets)})
	}

	from, to = from.In(s.deps.Zones.Instance()), to.In(s.deps.Zones.Instance())
	g := newBucketer(from, size)
	onOff := IsOnOffSensor(sn.Type)

	if size%time.Hour != 0 || onOff {
		if _, err := s.addRaw(ctx, id, from, to, g, onOff); err != nil {
			return nil, err
		}
		return g.result(onOff), nil
	}
	// Rollup hours where they exist; raw readings for the gaps between
	// them (hours the rollup never saw, after downtime or a restore) and
	// from the newest rollup hour on, which may still be filling.
	hours, err := s.rollupHours(ctx, id, from, to)
	if err != nil {
		return nil, err
	}
	cursor := from
	var newest *rollupHour
	if len(hours) > 0 {
		newest = &hours[len(hours)-1]
		hours = hours[:len(hours)-1]
	}
	for _, h := range hours {
		if h.at.After(cursor) {
			if _, err := s.addRaw(ctx, id, cursor, h.at, g, false); err != nil {
				return nil, err
			}
		}
		g.addSummary(h.at, h.lo, h.hi, h.avg, h.count)
		cursor = h.at.Add(time.Hour)
	}
	tail := cursor
	if newest != nil && newest.at.After(cursor) {
		if _, err := s.addRaw(ctx, id, cursor, newest.at, g, false); err != nil {
			return nil, err
		}
		tail = newest.at
	}
	n, err := s.addRaw(ctx, id, tail, to, g, false)
	if err != nil {
		return nil, err
	}
	// The newest rollup hour stands in for readings that retention has
	// already pruned.
	if newest != nil && n == 0 {
		g.addSummary(newest.at, newest.lo, newest.hi, newest.avg, newest.count)
	}
	return g.result(onOff), nil
}

type rollupHour struct {
	at          time.Time
	lo, hi, avg float64
	count       int
}

// rollupHours returns the hourly rollup rows in [from, to), oldest first.
func (s *SensorService) rollupHours(ctx context.Context, id int, from, to time.Time) ([]rollupHour, error) {
	z := s.deps.Zones
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT bucket, min_val, max_val, avg_val, sample_count FROM sensor_data_hourly
		WHERE sensor_id = $1 AND bucket >= $2 AND bucket < $3
		ORDER BY bucket`, id, z.ToSystem(from), z.ToSystem(to))
	if err != nil {
		return nil, fmt.Errorf("sensor %d rollup: %w", id, err)
	}
	defer rows.Close()
	var hours []rollupHour
	for rows.Next() {
		var (
			h  rollupHour
			at chrono.Stored
		)
		if err := rows.Scan(&at, &h.lo, &h.hi, &h.avg, &h.count); err != nil {
			return nil, fmt.Errorf("scan rollup: %w", err)
		}
		h.at = z.FromSystem(at.Time)
		hours = append(hours, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sensor %d rollup: %w", id, err)
	}
	return hours, nil
}

// addRaw adds the readings in [from, to) and returns how many there were.
func (s *SensorService) addRaw(ctx context.Context, id int, from, to time.Time, g *bucketer, onOff bool) (int, error) {
	z := s.deps.Zones
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT create_dt, value FROM sensor_data
		WHERE sensor_id = $1 AND create_dt >= $2 AND create_dt < $3
		ORDER BY create_dt, id`, id, z.ToSystem(from), z.ToSystem(to))
	if err != nil {
		return 0, fmt.Errorf("sensor %d readings: %w", id, err)
	}
	defer rows.Close()
	n := 0
	prevOn := false
	first := true
	for rows.Next() {
		var (
			at chrono.Stored
			v  float64
		)
		if err := rows.Scan(&at, &v); err != nil {
			return 0, fmt.Errorf("scan reading: %w", err)
		}
		n++
		b := g.addReading(z.FromSystem(at.Time), v)
		if onOff {
			on := v > 0
			if on {
				b.on++
			}
			// The first reading has nothing before it to switch from.
			if on && !prevOn && !first {
				b.starts++
			}
			prevOn, first = on, false
		}
	}
	return n, rows.Err()
}

// bucketer accumulates readings into buckets.
type bucketer struct {
	origin  time.Time
	size    time.Duration
	days    int // size in whole days, or 0
	buckets map[int64]*acc
	order   []int64
}

type acc struct {
	start       time.Time
	lo, hi, sum float64
	count       int
	on, starts  int
}

func newBucketer(from time.Time, size time.Duration) *bucketer {
	g := &bucketer{size: size, buckets: map[int64]*acc{}}
	y, m, d := from.Date()
	// Buckets count from midnight on the first day, in the instance zone.
	g.origin = time.Date(y, m, d, 0, 0, 0, 0, from.Location())
	if size%(24*time.Hour) == 0 {
		g.days = int(size / (24 * time.Hour))
	}
	return g
}

func (g *bucketer) bucket(at time.Time) *acc {
	at = at.In(g.origin.Location())
	var key int64
	var start time.Time
	if g.days > 0 {
		y, m, d := at.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, at.Location())
		n := int64(math.Round(day.Sub(g.origin).Hours() / 24))
		key = n / int64(g.days)
		start = g.origin.AddDate(0, 0, int(key)*g.days)
	} else {
		key = int64(at.Sub(g.origin) / g.size)
		start = g.origin.Add(time.Duration(key) * g.size)
	}
	b, ok := g.buckets[key]
	if !ok {
		b = &acc{start: start, lo: math.Inf(1), hi: math.Inf(-1)}
		g.buckets[key] = b
		g.order = append(g.order, key)
	}
	return b
}

func (g *bucketer) addReading(at time.Time, v float64) *acc {
	b := g.bucket(at)
	b.lo, b.hi = math.Min(b.lo, v), math.Max(b.hi, v)
	b.sum += v
	b.count++
	return b
}

func (g *bucketer) addSummary(at time.Time, lo, hi, avg float64, count int) {
	b := g.bucket(at)
	b.lo, b.hi = math.Min(b.lo, lo), math.Max(b.hi, hi)
	b.sum += avg * float64(count)
	b.count += count
}

func (g *bucketer) result(onOff bool) []Bucket {
	out := make([]Bucket, 0, len(g.order))
	keys := slices.Clone(g.order)
	slices.Sort(keys)
	for _, k := range keys {
		b := g.buckets[k]
		if b.count == 0 {
			continue
		}
		r := Bucket{Start: b.start, Min: b.lo, Max: b.hi, Avg: b.sum / float64(b.count), Count: b.count}
		if onOff {
			frac := float64(b.on) / float64(b.count)
			starts := b.starts
			r.OnFraction, r.Starts = &frac, &starts
		}
		out = append(out, r)
	}
	return out
}
