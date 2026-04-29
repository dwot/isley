// Package config holds the per-engine runtime configuration Store.
//
// All settings the application reads at request time live on a *Store
// instance constructed by app.NewEngine. The Store is threaded through
// the Gin context so handlers retrieve it via
// handlers.ConfigStoreFromContext(c). One process can run multiple
// engines (e.g. parallel tests) without colliding on global state.
//
// The lone surviving package-global is RestoreInProgress, an atomic
// coordination primitive used by the watcher loops to skip iterations
// during a backup restore. It is a flag, not configuration.
package config

import "sync/atomic"

// RestoreInProgress is set to true while a backup restore is running.
// Watchers check this flag and skip their iteration to avoid DB
// contention. It is a process-wide coordination signal — restores are
// already a singleton operation guarded by *handlers.BackupService and
// the watcher only cares whether one is happening anywhere in the
// process.
var RestoreInProgress atomic.Bool
