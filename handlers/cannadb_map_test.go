package handlers

import (
	"testing"

	"isley/cannadb"
)

func intPtr(n int) *int { return &n }

func TestMapCannadbStrain(t *testing.T) {
	t.Parallel()
	rec := &cannadb.Record{URI: "at://did/org.cannadb.strain/x", IndexedAt: "2026-04-30T20:00:40Z"}

	t.Run("indicaSativa lean and cycleTime max", func(t *testing.T) {
		val := &cannadb.StrainValue{
			Name:         "Wedding Cake",
			IndicaSativa: intPtr(40),
			Autoflower:   true,
			Description:  "# markdown",
			SourceURL:    "https://breeder.example/wc",
			CycleTime:    &cannadb.Range{Min: intPtr(56), Max: intPtr(63)},
		}
		s := mapCannadbStrain(rec, val)
		if s.SativaPct != 40 || s.IndicaPct != 60 {
			t.Fatalf("indica/sativa = %d/%d, want 60/40", s.IndicaPct, s.SativaPct)
		}
		if s.CycleTimeDays != 63 {
			t.Fatalf("cycleTime = %d, want 63 (max)", s.CycleTimeDays)
		}
		if !s.Autoflower || s.Description != "# markdown" || s.URL != "https://breeder.example/wc" {
			t.Fatalf("unexpected mapped fields: %+v", s)
		}
		if s.URI != rec.URI || s.IndexedAt != rec.IndexedAt {
			t.Fatalf("provenance not carried: %+v", s)
		}
	})

	t.Run("absent indicaSativa defaults 50/50", func(t *testing.T) {
		s := mapCannadbStrain(rec, &cannadb.StrainValue{Name: "X"})
		if s.IndicaPct != 50 || s.SativaPct != 50 {
			t.Fatalf("default lean = %d/%d, want 50/50", s.IndicaPct, s.SativaPct)
		}
		if s.IndicaPct+s.SativaPct != 100 {
			t.Fatalf("indica+sativa must equal 100")
		}
	})

	t.Run("cycleTime falls back to min", func(t *testing.T) {
		s := mapCannadbStrain(rec, &cannadb.StrainValue{Name: "X", CycleTime: &cannadb.Range{Min: intPtr(70)}})
		if s.CycleTimeDays != 70 {
			t.Fatalf("cycleTime = %d, want 70 (min fallback)", s.CycleTimeDays)
		}
	})

	t.Run("out-of-range lean is clamped", func(t *testing.T) {
		s := mapCannadbStrain(rec, &cannadb.StrainValue{Name: "X", IndicaSativa: intPtr(150)})
		if s.SativaPct != 100 || s.IndicaPct != 0 {
			t.Fatalf("clamp failed: %d/%d", s.IndicaPct, s.SativaPct)
		}
	})
}
