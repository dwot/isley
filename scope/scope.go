// Package scope defines what an API key is allowed to do.
//
// A key carries an access level (read < write < admin) and, separately, the
// right to push sensor readings (ingest). Higher levels include lower ones,
// and admin includes ingest, so a key's powers are always one of a small,
// easy-to-explain set:
//
//	read           read data
//	write          read data and record things (activities, measurements, ...)
//	admin          everything a logged-in user can do through the API
//	ingest         push sensor readings only
//	read,ingest    read data and push sensor readings
//	write,ingest   write and push sensor readings
//
// Scopes are stored in api_keys.scopes as the canonical comma-separated form
// produced by Set.String.
package scope

import (
	"fmt"
	"strings"
)

// Scope is one named permission a route can require.
type Scope string

const (
	Read   Scope = "read"
	Write  Scope = "write"
	Admin  Scope = "admin"
	Ingest Scope = "ingest"
)

// level orders the access levels; ingest is not a level.
var level = map[Scope]int{Read: 1, Write: 2, Admin: 3}

// Set is the parsed scopes of one key. The zero value allows nothing.
type Set struct {
	level  Scope // "", Read, Write or Admin
	ingest bool
}

// Full is the scope set of a key with every permission.
var Full = Set{level: Admin}

// Parse reads a comma-separated scope list ("write,ingest"). Whitespace and
// case are ignored, and redundant entries ("read,write") collapse to the
// highest level. An empty list or an unknown name is an error.
func Parse(s string) (Set, error) {
	var set Set
	seen := false
	for _, part := range strings.Split(s, ",") {
		name := Scope(strings.ToLower(strings.TrimSpace(part)))
		if name == "" {
			continue
		}
		seen = true
		switch name {
		case Ingest:
			set.ingest = true
		case Read, Write, Admin:
			if level[name] > level[set.level] {
				set.level = name
			}
		default:
			return Set{}, fmt.Errorf("unknown scope %q", name)
		}
	}
	if !seen {
		return Set{}, fmt.Errorf("no scopes given")
	}
	return set, nil
}

// FromList is Parse for a list of names, as sent in JSON request bodies.
func FromList(names []string) (Set, error) {
	return Parse(strings.Join(names, ","))
}

// Allows reports whether the set grants the required scope.
func (s Set) Allows(required Scope) bool {
	switch required {
	case Ingest:
		return s.ingest || s.level == Admin
	case Read, Write, Admin:
		return level[s.level] >= level[required]
	default:
		return false
	}
}

// List returns the canonical scope names, highest level first.
func (s Set) List() []string {
	if s.level == Admin {
		return []string{string(Admin)}
	}
	out := []string{}
	if s.level != "" {
		out = append(out, string(s.level))
	}
	if s.ingest {
		out = append(out, string(Ingest))
	}
	return out
}

// String returns the canonical stored form, e.g. "write,ingest".
func (s Set) String() string {
	return strings.Join(s.List(), ",")
}
