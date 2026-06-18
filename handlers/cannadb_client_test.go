package handlers

// Unit tests for the CannaDB client (handlers/cannadb_client.go). These are in
// package handlers (not handlers_test) so they can exercise the unexported
// client surface directly without standing up the HTTP server.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCannadbWebURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "strain",
			uri:  "at://did:plc:j45zu6wau62ffxgcmld4xhln/org.cannadb.strain/3ml53leq7722n",
			want: "https://cannadb.org/strain/did:plc:j45zu6wau62ffxgcmld4xhln/3ml53leq7722n",
		},
		{
			name: "breeder",
			uri:  "at://did:plc:abc/org.cannadb.breeder/xyz",
			want: "https://cannadb.org/breeder/did:plc:abc/xyz",
		},
		{name: "empty", uri: "", want: ""},
		{name: "malformed", uri: "at://did:plc:abc", want: ""},
		{name: "not-an-at-uri", uri: "https://example.com", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cannadbWebURL(tc.uri); got != tc.want {
				t.Fatalf("cannadbWebURL(%q) = %q, want %q", tc.uri, got, tc.want)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

func TestMapCannadbStrain(t *testing.T) {
	t.Parallel()
	rec := &cannadbRecord{URI: "at://did/org.cannadb.strain/x", IndexedAt: "2026-04-30T20:00:40Z"}

	t.Run("indicaSativa lean and cycleTime max", func(t *testing.T) {
		val := &cannadbStrainValue{
			Name:         "Wedding Cake",
			IndicaSativa: intPtr(40),
			Autoflower:   true,
			Description:  "# markdown",
			SourceURL:    "https://breeder.example/wc",
			CycleTime:    &cannadbRange{Min: intPtr(56), Max: intPtr(63)},
		}
		s := mapCannadbStrain(rec, val)
		if s.Sativa != 40 || s.Indica != 60 {
			t.Fatalf("indica/sativa = %d/%d, want 60/40", s.Indica, s.Sativa)
		}
		if s.CycleTime != 63 {
			t.Fatalf("cycleTime = %d, want 63 (max)", s.CycleTime)
		}
		if !s.Autoflower || s.Description != "# markdown" || s.Url != "https://breeder.example/wc" {
			t.Fatalf("unexpected mapped fields: %+v", s)
		}
		if s.CannadbURI != rec.URI || s.CannadbIndexedAt != rec.IndexedAt {
			t.Fatalf("provenance not carried: %+v", s)
		}
	})

	t.Run("absent indicaSativa defaults 50/50", func(t *testing.T) {
		s := mapCannadbStrain(rec, &cannadbStrainValue{Name: "X"})
		if s.Indica != 50 || s.Sativa != 50 {
			t.Fatalf("default lean = %d/%d, want 50/50", s.Indica, s.Sativa)
		}
		if s.Indica+s.Sativa != 100 {
			t.Fatalf("indica+sativa must equal 100")
		}
	})

	t.Run("cycleTime falls back to min", func(t *testing.T) {
		s := mapCannadbStrain(rec, &cannadbStrainValue{Name: "X", CycleTime: &cannadbRange{Min: intPtr(70)}})
		if s.CycleTime != 70 {
			t.Fatalf("cycleTime = %d, want 70 (min fallback)", s.CycleTime)
		}
	})

	t.Run("out-of-range lean is clamped", func(t *testing.T) {
		s := mapCannadbStrain(rec, &cannadbStrainValue{Name: "X", IndicaSativa: intPtr(150)})
		if s.Sativa != 100 || s.Indica != 0 {
			t.Fatalf("clamp failed: %d/%d", s.Indica, s.Sativa)
		}
	})
}

func TestCannadbGet_RetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	// Each 429 carries Retry-After: 0, so the client retries immediately with
	// no real sleep — keeping the test fast and parallel-safe (no shared state).
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"RateLimited","retry_after_seconds":0}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"strains":[{"uri":"at://x","name":"OG","similarity":0.9}]}`))
	}))
	defer srv.Close()

	rows, err := cannadbSearchStrains(srv.URL+"/xrpc/", "og", 5)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
	if len(rows) != 1 || rows[0].Name != "OG" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestCannadbGet_NonRetryableError(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"RecordNotFound","message":"nope"}`))
	}))
	defer srv.Close()

	var out cannadbSearchResponse
	err := cannadbGet(srv.URL+"/xrpc/", "org.cannadb.getStrain", url.Values{"uri": {"at://x"}}, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*cannadbError)
	if !ok {
		t.Fatalf("expected *cannadbError, got %T: %v", err, err)
	}
	if apiErr.Code != "RecordNotFound" || apiErr.HTTPStatus != http.StatusNotFound {
		t.Fatalf("unexpected error envelope: %+v", apiErr)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("404 must not be retried, got %d calls", calls)
	}
}

func TestCannadbGet_EncodesATURI(t *testing.T) {
	t.Parallel()

	var gotRaw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRaw = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	uri := "at://did:plc:abc/org.cannadb.strain/wedding-cake"
	_, _, err := cannadbGetStrain(srv.URL+"/xrpc/", uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The :// and / must be percent-encoded in the query string.
	if gotRaw == "" || !strings.Contains(gotRaw, "uri=at%3A%2F%2Fdid") {
		t.Fatalf("AT-URI not URL-encoded in query: %q", gotRaw)
	}
}
