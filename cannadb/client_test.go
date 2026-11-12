package cannadb

// Unit tests for the CannaDB client, in-package so they can reach the
// unexported plumbing.

import (
	"io"
	"isley/logger"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
)

// The client logs; tests discard it. (testutil sets this up elsewhere, but
// importing it here would be an import cycle.)
func init() {
	l := logrus.New()
	l.SetOutput(io.Discard)
	logger.Log = l
}

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
			if got := WebURL(tc.uri); got != tc.want {
				t.Fatalf("WebURL(%q) = %q, want %q", tc.uri, got, tc.want)
			}
		})
	}
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

	rows, err := SearchStrains(srv.URL+"/xrpc/", "og", 5)
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

	var out searchResponse
	err := get(srv.URL+"/xrpc/", "org.cannadb.getStrain", url.Values{"uri": {"at://x"}}, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
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
	_, _, err := GetStrain(srv.URL+"/xrpc/", uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The :// and / must be percent-encoded in the query string.
	if gotRaw == "" || !strings.Contains(gotRaw, "uri=at%3A%2F%2Fdid") {
		t.Fatalf("AT-URI not URL-encoded in query: %q", gotRaw)
	}
}
