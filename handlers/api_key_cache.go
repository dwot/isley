package handlers

import (
	"crypto/sha256"
	"database/sql"
	"sync"
	"time"
)

// verifiedKeyTTL is how long a successful key check is reused. Checking a
// key is a bcrypt compare (~250ms of CPU at cost 12), which an agent making
// a burst of calls would otherwise pay on every one.
const verifiedKeyTTL = time.Minute

type verifiedKey struct {
	key     AuthenticatedKey
	expires time.Time
}

// verifiedKeys caches successful checks per database (so test engines
// don't share) by the SHA-256 of the presented key; the plaintext isn't
// kept. Any change to a database's keys clears its entries, so a revoked
// or re-scoped key stops working at once.
var verifiedKeys = struct {
	sync.Mutex
	byDB map[*sql.DB]map[[32]byte]verifiedKey
}{byDB: map[*sql.DB]map[[32]byte]verifiedKey{}}

func cachedKey(db *sql.DB, plaintext string, now time.Time) (*AuthenticatedKey, bool) {
	verifiedKeys.Lock()
	defer verifiedKeys.Unlock()
	entry, ok := verifiedKeys.byDB[db][sha256.Sum256([]byte(plaintext))]
	if !ok || now.After(entry.expires) {
		return nil, false
	}
	key := entry.key
	return &key, true
}

func rememberKey(db *sql.DB, plaintext string, key AuthenticatedKey, now time.Time) {
	verifiedKeys.Lock()
	defer verifiedKeys.Unlock()
	m := verifiedKeys.byDB[db]
	if m == nil {
		m = map[[32]byte]verifiedKey{}
		verifiedKeys.byDB[db] = m
	}
	for k, e := range m {
		if now.After(e.expires) {
			delete(m, k)
		}
	}
	m[sha256.Sum256([]byte(plaintext))] = verifiedKey{key: key, expires: now.Add(verifiedKeyTTL)}
}

// ForgetVerifiedKeys drops every cached key check for db. Call it after
// anything that changes api_keys: regenerating, re-scoping or revoking a
// key, or restoring a backup.
func ForgetVerifiedKeys(db *sql.DB) {
	verifiedKeys.Lock()
	defer verifiedKeys.Unlock()
	delete(verifiedKeys.byDB, db)
}
