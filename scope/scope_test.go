package scope

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Canonicalises(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"read":              "read",
		"WRITE":             "write",
		" read , write ":    "write",
		"admin,ingest":      "admin",
		"admin,read":        "admin",
		"ingest":            "ingest",
		"ingest,read":       "read,ingest",
		"write,ingest,read": "write,ingest",
		"read,,":            "read",
	}
	for in, want := range cases {
		got, err := Parse(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got.String(), in)
	}
}

func TestParse_Rejects(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", " , ", "owner", "read,delete"} {
		_, err := Parse(in)
		assert.Error(t, err, in)
	}
}

func TestAllows(t *testing.T) {
	t.Parallel()
	type row struct {
		set                        string
		read, write, admin, ingest bool
	}
	rows := []row{
		{"read", true, false, false, false},
		{"write", true, true, false, false},
		{"admin", true, true, true, true},
		{"ingest", false, false, false, true},
		{"read,ingest", true, false, false, true},
	}
	for _, r := range rows {
		s, err := Parse(r.set)
		require.NoError(t, err)
		assert.Equal(t, r.read, s.Allows(Read), r.set+" read")
		assert.Equal(t, r.write, s.Allows(Write), r.set+" write")
		assert.Equal(t, r.admin, s.Allows(Admin), r.set+" admin")
		assert.Equal(t, r.ingest, s.Allows(Ingest), r.set+" ingest")
	}
}

func TestZeroSetAllowsNothing(t *testing.T) {
	t.Parallel()
	var s Set
	for _, sc := range []Scope{Read, Write, Admin, Ingest, "bogus"} {
		assert.False(t, s.Allows(sc), sc)
	}
	assert.Equal(t, "", s.String())
}

func TestFull(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "admin", Full.String())
	assert.True(t, Full.Allows(Ingest))
}
