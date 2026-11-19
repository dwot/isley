package theme

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const forest = `/* theme: Forest
 * modes: light dark
 */
[data-theme="forest"] { --color-primary: #166534; }
[data-theme="forest"][data-bs-theme="dark"] { --color-primary: #22c55e; }
`

func themeFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, body := range files {
		m[Dir+"/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return m
}

func TestLoad_ClassicFirstThenByName(t *testing.T) {
	t.Parallel()
	r, err := Load(themeFS(map[string]string{
		"forest.css": forest,
		"amber.css":  "/* theme: Amber\n modes: dark */\n[data-theme=\"amber\"][data-bs-theme=\"dark\"] {}\n",
	}))
	require.NoError(t, err)

	all := r.All()
	require.Len(t, all, 3)
	assert.Equal(t, []string{"classic", "amber", "forest"}, []string{all[0].ID, all[1].ID, all[2].ID})
	assert.Equal(t, "", all[0].File)
	assert.Equal(t, "/static/css/themes/forest.css", all[2].File)
	assert.Equal(t, "light dark", all[2].Modes())
	assert.Equal(t, "dark", all[1].Modes())
	assert.Equal(t, "dark", all[1].DefaultMode())
}

func TestLoad_NoThemesDirIsJustClassic(t *testing.T) {
	t.Parallel()
	r, err := Load(fstest.MapFS{})
	require.NoError(t, err)
	assert.Len(t, r.All(), 1)
}

func TestLoad_RejectsBrokenThemes(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]string{
		"no header":            {"x.css": `[data-theme="x"] {}`},
		"no name":              {"x.css": "/* modes: light */\n[data-theme=\"x\"] {}"},
		"no modes":             {"x.css": "/* theme: X */\n[data-theme=\"x\"] {}"},
		"unknown mode":         {"x.css": "/* theme: X\n modes: dim */\n[data-theme=\"x\"] {}"},
		"dark declared, unset": {"x.css": "/* theme: X\n modes: light dark */\n[data-theme=\"x\"] {}"},
		"light declared, unset": {"x.css": "/* theme: X\n modes: light dark */\n" +
			"[data-theme=\"x\"][data-bs-theme=\"dark\"] {}"},
		"bad file name": {"My Theme.css": "/* theme: X\n modes: light */\n[data-theme=\"My Theme\"] {}"},
		"takes classic": {"classic.css": "/* theme: X\n modes: light */\n[data-theme=\"classic\"] {}"},
	}
	for name, files := range cases {
		_, err := Load(themeFS(files))
		assert.Error(t, err, name)
	}
}

func TestResolve_UnknownFallsBackToClassic(t *testing.T) {
	t.Parallel()
	r, err := Load(themeFS(map[string]string{"forest.css": forest}))
	require.NoError(t, err)
	assert.Equal(t, "forest", r.Resolve("forest").ID)
	assert.Equal(t, DefaultID, r.Resolve("gone").ID)
	assert.Equal(t, DefaultID, r.Resolve("").ID)
}

func TestValidID(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"classic", "high-contrast", "neon2"} {
		assert.True(t, ValidID(ok), ok)
	}
	for _, bad := range []string{"", "Forest", "a b", "../x", "-x", "x-", `x"y`} {
		assert.False(t, ValidID(bad), bad)
	}
}

// The themes this build ships must all load.
func TestLoad_ShippedThemes(t *testing.T) {
	t.Parallel()
	r, err := Load(os.DirFS(".."))
	require.NoError(t, err)
	assert.Equal(t, DefaultID, r.All()[0].ID)
}
