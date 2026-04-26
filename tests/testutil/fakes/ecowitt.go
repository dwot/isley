package fakes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"isley/tests/testutil"
)

// FakeEcoWitt returns an httptest.Server that serves
// tests/fixtures/ecowitt/<fixture>.json on /get_livedata_info. The
// path-checking matches the production EcoWitt client which only ever
// hits /get_livedata_info. Other paths get a 404 so a regression that
// changes the request path will surface as a missing-row failure in
// the calling test rather than a silent pass.
func FakeEcoWitt(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	body := testutil.MustReadFixture(t, "ecowitt/"+fixture+".json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get_livedata_info" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}
