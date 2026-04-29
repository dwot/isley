// Package fakes provides httptest.Server stand-ins for the external
// services the watcher polls in production. Each constructor takes a
// fixture name (looked up under tests/fixtures/<service>/<name>.json),
// returns an httptest.Server seeded with the fixture body, and
// registers automatic cleanup via t.Cleanup.
package fakes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"isley/tests/testutil"
)

// FakeACI returns an httptest.Server that responds with
// tests/fixtures/aci/<fixture>.json on every request. Used by watcher
// PollACI tests in place of inline httptest.NewServer setups so the
// canned response lives in a versioned file rather than as a string
// literal.
func FakeACI(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	body := testutil.MustReadFixture(t, "aci/"+fixture+".json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}
