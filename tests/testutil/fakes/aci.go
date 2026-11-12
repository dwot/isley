// Package fakes provides httptest.Server stand-ins for the external
// services the watcher polls in production. Each constructor takes a
// fixture name (looked up under tests/fixtures/<service>/<name>.json),
// returns an httptest.Server seeded with the fixture body, and
// registers automatic cleanup via t.Cleanup.
package fakes
