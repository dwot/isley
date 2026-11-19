package theme

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repo is the real asset tree, for the tests that hold the package format
// to the stylesheets that ship.
func repo() fs.FS { return os.DirFS("..") }

func pngBytes(t *testing.T, side int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, side, side))))
	return buf.Bytes()
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

const harbour = `{
  "isley_theme": 1,
  "name": "Harbour",
  "modes": ["dark", "light"],
  "tokens": { "--radius": "4px", "--font-body": "Inter, \"Segoe UI\", sans-serif" },
  "light":  { "--color-primary": "#0b5cad" },
  "dark":   { "--color-primary": "#4d9aff", "--bg-image": "linear-gradient(160deg, #101820 0%, rgba(0, 0, 0, 0.4) 100%)" }
}`

func TestTokens_MatchTheStylesheet(t *testing.T) {
	t.Parallel()
	css, err := fs.ReadFile(repo(), "web/static/css/tokens.css")
	require.NoError(t, err)
	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+)\s*:`).FindAllStringSubmatch(string(css), -1) {
		declared[m[1]] = true
	}
	// Worked out from --radius, or set by the package's logo file.
	derived := map[string]bool{"--radius-sm": true, "--radius-lg": true, "--radius-xl": true, "--radius-pill": true, "--logo": true}

	for name := range tokenKinds {
		assert.True(t, declared[name], "%s is accepted in a package but is not in tokens.css", name)
	}
	for name := range declared {
		if !derived[name] {
			assert.True(t, KnownToken(name), "%s is in tokens.css but a package cannot set it", name)
		}
	}
}

// Every value a built-in theme uses must be one a package may use, or the
// format could not express the themes Isley ships.
func TestBuiltInThemes_FitTheFormat(t *testing.T) {
	t.Parallel()
	files, err := fs.Glob(repo(), Dir+"/*.css")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		css, err := fs.ReadFile(repo(), file)
		require.NoError(t, err)
		for _, rule := range ruleRe.FindAllStringSubmatch(commentRe.ReplaceAllString(string(css), ""), -1) {
			if !regexp.MustCompile(`^\s*\[data-theme="[a-z0-9-]+"\](\[data-bs-theme="dark"\])?\s*(,|$)`).MatchString(rule[1]) {
				continue // a rule beyond the tokens
			}
			for _, decl := range strings.Split(rule[2], ";") {
				d := declRe.FindStringSubmatch(spacesRe.ReplaceAllString(decl, " "))
				if d == nil || d[1] == "--logo" {
					continue
				}
				_, err := checkValue(d[1], d[2])
				assert.NoError(t, err, "%s: %s: %s", file, d[1], d[2])
			}
		}
	}
}

func TestCheckValue_Rejects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ token, value string }{
		{"--color-bg", "red; } body { display: none"},
		{"--color-bg", "url(https://example.com/x.png)"},
		{"--color-bg", "var(--color-fg)"},
		{"--color-bg", "#12345"},
		{"--color-bg", "rgb(0, 0, 0); color: red"},
		{"--color-bg", "expression(alert(1))"},
		{"--bg-image", `url("/uploads/x.png")`},
		{"--bg-image", "linear-gradient(#fff, url(x))"},
		{"--bg-image", "#fff"},
		{"--bg-image", "linear-gradient(#fff, #000"},
		{"--bg-image", "linear-gradient(#fff, #000))"},
		{"--bg-image", "image-set(#fff)"},
		{"--bg-image", "linear-gradient(#fff, #000) } a { color: red"},
		{"--shadow-sm", "0 0 4px url(x)"},
		{"--shadow-sm", "0 0 4px red"},
		{"--font-body", `Inter; } * { display: none`},
		{"--font-body", `"Inter\"; }"`},
		{"--font-body", "Inter, url(x)"},
		{"--font-body", `"</style><script>"`},
		{"--radius", "400px"},
		{"--radius", "calc(1px * 9999)"},
		{"--font-size", "4rem"},
		{"--font-size", "16"},
		{"--shade", "150%"},
		{"--color-bg", ""},
		{"--color-bg", "#" + strings.Repeat("a", 700)},
		{"--not-a-token", "#fff"},
	} {
		_, err := checkValue(tc.token, tc.value)
		assert.Error(t, err, "%s: %s", tc.token, tc.value)
	}
}

func TestCheckValue_Accepts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ token, value, want string }{
		{"--color-bg", " #FFF ", "#FFF"},
		{"--color-bg", "rgba(18, 16, 14, 0.88)", "rgba(18, 16, 14, 0.88)"},
		{"--color-bg", "hsl(0 70% 50% / 0.5)", "hsl(0 70% 50% / 0.5)"},
		{"--color-bg", "transparent", "transparent"},
		{"--bg-image", "none", "none"},
		{"--bg-image", "radial-gradient(900px 480px at 12% -80px, rgba(139, 61, 255, 0.32), transparent),\n   radial-gradient(800px 420px at 95% 0, #39ff88, transparent)",
			"radial-gradient(900px 480px at 12% -80px, rgba(139, 61, 255, 0.32), transparent), radial-gradient(800px 420px at 95% 0, #39ff88, transparent)"},
		{"--shadow-hover", "0 0 0 1px #b3703f", "0 0 0 1px #b3703f"},
		{"--shadow-sm", "inset 0 1px 2px rgba(0, 0, 0, 0.4), 0 3px 8px #0006", "inset 0 1px 2px rgba(0, 0, 0, 0.4), 0 3px 8px #0006"},
		{"--font-mono", `"JetBrainsMono Nerd Font", 'JetBrains Mono', SFMono-Regular, monospace`, `"JetBrainsMono Nerd Font", 'JetBrains Mono', SFMono-Regular, monospace`},
		{"--radius", "14px", "14px"},
		{"--font-size", "1.0625rem", "1.0625rem"},
		{"--shade", "0%", "0%"},
	} {
		got, err := checkValue(tc.token, tc.value)
		if assert.NoError(t, err, "%s: %s", tc.token, tc.value) {
			assert.Equal(t, tc.want, got)
		}
	}
}

func TestReadPackage_BareManifest(t *testing.T) {
	t.Parallel()
	p, warnings, err := ReadPackage([]byte(harbour))
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, "harbour", p.Manifest.ID, "the id comes from the name")
	assert.Equal(t, []string{"light", "dark"}, p.Manifest.Modes)
	assert.Nil(t, p.Logo)
}

func TestReadPackage_ZipInAFolderWithLogos(t *testing.T) {
	t.Parallel()
	logo := pngBytes(t, 64)
	p, _, err := ReadPackage(zipOf(t, map[string][]byte{
		"harbour/theme.json":    []byte(harbour),
		"harbour/logo.png":      logo,
		"harbour/Logo-Dark.PNG": logo,
		"harbour/README.txt":    []byte("ignored"),
	}))
	require.NoError(t, err)
	require.NotNil(t, p.Logo)
	require.NotNil(t, p.LogoDark)
	assert.Equal(t, ".png", p.Logo.Ext)

	// And back out again.
	data, err := p.Zip()
	require.NoError(t, err)
	again, _, err := ReadPackage(data)
	require.NoError(t, err)
	assert.Equal(t, p.Manifest, again.Manifest)
	assert.Equal(t, logo, again.Logo.Data)
}

func TestReadPackage_UnknownTokensAreDroppedWithAWarning(t *testing.T) {
	t.Parallel()
	p, warnings, err := ReadPackage([]byte(`{"isley_theme":1,"name":"N","modes":["dark"],
		"dark":{"--color-bg":"#000","--from-the-future":"anything; at all"}}`))
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "--from-the-future")
	assert.Equal(t, map[string]string{"--color-bg": "#000"}, p.Manifest.Dark)
}

func TestReadPackage_Rejects(t *testing.T) {
	t.Parallel()
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	for name, data := range map[string][]byte{
		"not a package":    []byte("hello"),
		"no manifest":      zipOf(t, map[string][]byte{"logo.png": pngBytes(t, 8)}),
		"no version":       []byte(`{"name":"N","modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"newer format":     []byte(`{"isley_theme":2,"name":"N","modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"no name":          []byte(`{"isley_theme":1,"modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"no modes":         []byte(`{"isley_theme":1,"name":"N","tokens":{"--color-bg":"#000"}}`),
		"bad mode":         []byte(`{"isley_theme":1,"name":"N","modes":["dim"],"tokens":{"--color-bg":"#000"}}`),
		"mode not listed":  []byte(`{"isley_theme":1,"name":"N","modes":["dark"],"light":{"--color-bg":"#fff"}}`),
		"no tokens":        []byte(`{"isley_theme":1,"name":"N","modes":["dark"]}`),
		"bad id":           []byte(`{"isley_theme":1,"id":"../x","name":"N","modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"id is a selector": []byte(`{"isley_theme":1,"id":"a\"] *","name":"N","modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"bad value":        []byte(`{"isley_theme":1,"name":"N","modes":["dark"],"dark":{"--color-bg":"url(x)"}}`),
		"control in name":  []byte(`{"isley_theme":1,"name":"N\u0000","modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"svg logo":         zipOf(t, map[string][]byte{"theme.json": []byte(harbour), "logo.png": svg}),
		"huge logo":        zipOf(t, map[string][]byte{"theme.json": []byte(harbour), "logo.png": pngBytes(t, 4096)}),
		"too big":          bytes.Repeat([]byte("x"), MaxPackageSize+1),
	} {
		_, _, err := ReadPackage(data)
		assert.Error(t, err, name)
	}
}

func TestCSS_Selectors(t *testing.T) {
	t.Parallel()
	p, _, err := ReadPackage([]byte(harbour))
	require.NoError(t, err)
	css := p.Manifest.CSS("/themes/harbour/logo?v=1", "")
	assert.Contains(t, css, "[data-theme=\"harbour\"] {\n    --font-body: Inter, \"Segoe UI\", sans-serif;\n    --logo: url(\"/themes/harbour/logo?v=1\");\n    --radius: 4px;\n}")
	assert.Contains(t, css, "[data-theme=\"harbour\"][data-bs-theme=\"light\"] {\n    --color-primary: #0b5cad;\n}")
	assert.Contains(t, css, "[data-theme=\"harbour\"][data-bs-theme=\"dark\"] {\n    --bg-image:")

	dark, _, err := ReadPackage([]byte(`{"isley_theme":1,"name":"Pit","modes":["dark"],"dark":{"--color-bg":"#000"}}`))
	require.NoError(t, err)
	assert.Equal(t, "[data-theme=\"pit\"],\n[data-theme=\"pit\"][data-bs-theme=\"dark\"] {\n    --color-bg: #000;\n}\n", dark.Manifest.CSS("", ""))
}

func TestRegistry_InstallReloadRemove(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "themes")
	r, err := Load(themeFS(map[string]string{"forest.css": forest}))
	require.NoError(t, err)
	require.Empty(t, r.SetUserDir(dir))
	css, ver := r.UserCSS()
	assert.Empty(t, css)
	assert.Empty(t, ver)

	p, _, err := ReadPackage(zipOf(t, map[string][]byte{"theme.json": []byte(harbour), "logo.png": pngBytes(t, 32)}))
	require.NoError(t, err)
	got, replaced, err := r.Install(p)
	require.NoError(t, err)
	assert.False(t, replaced)
	assert.Equal(t, Theme{ID: "harbour", Name: "Harbour", Light: true, Dark: true, User: true}, got)

	all := r.All()
	require.Len(t, all, 3)
	assert.Equal(t, "harbour", all[2].ID, "installed themes follow the built-in ones")
	assert.Equal(t, "harbour", r.Resolve("harbour").ID)
	css, ver = r.UserCSS()
	assert.Contains(t, string(css), `--logo: url("/themes/harbour/logo?v=`)
	assert.NotEmpty(t, ver)
	file, ok := r.LogoFile("harbour", false)
	assert.True(t, ok)
	assert.FileExists(t, file)
	_, ok = r.LogoFile("harbour", true)
	assert.False(t, ok)

	// Installing it again replaces it.
	p.Manifest.Name = "Harbour Two"
	_, replaced, err = r.Install(p)
	require.NoError(t, err)
	assert.True(t, replaced)
	assert.Equal(t, "Harbour Two", r.Resolve("harbour").Name)
	_, ver2 := r.UserCSS()
	assert.Equal(t, ver, ver2, "the same rules, the same version")

	// A second registry on the same folder finds it: a restart.
	r2, err := Load(themeFS(map[string]string{"forest.css": forest}))
	require.NoError(t, err)
	require.Empty(t, r2.SetUserDir(dir))
	assert.Equal(t, "Harbour Two", r2.Resolve("harbour").Name)

	// It comes back out as it went in.
	out, err := r.Export("harbour")
	require.NoError(t, err)
	assert.Equal(t, p.Manifest, out.Manifest)
	require.NotNil(t, out.Logo)

	require.NoError(t, r.Remove("harbour"))
	assert.Equal(t, DefaultID, r.Resolve("harbour").ID)
	assert.NoDirExists(t, filepath.Join(dir, "harbour"))
	assert.ErrorIs(t, r.Remove("harbour"), ErrNotFound)
	assert.ErrorIs(t, r.Remove("forest"), ErrBuiltIn)
	assert.ErrorIs(t, r.Remove(DefaultID), ErrBuiltIn)
}

func TestRegistry_BuiltInIDsAreTaken(t *testing.T) {
	t.Parallel()
	r, err := Load(themeFS(map[string]string{"forest.css": forest}))
	require.NoError(t, err)
	require.Empty(t, r.SetUserDir(t.TempDir()))
	for _, id := range []string{"forest", "classic"} {
		p, _, err := ReadPackage([]byte(`{"isley_theme":1,"id":"` + id + `","name":"N","modes":["dark"],"dark":{"--color-bg":"#000"}}`))
		require.NoError(t, err)
		_, _, err = r.Install(p)
		assert.Error(t, err, id)
	}
}

func TestRegistry_ReloadSkipsWhatItCannotRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(id, body string) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, id), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, id, ManifestName), []byte(body), 0o644))
	}
	write("good", `{"isley_theme":1,"id":"good","name":"Good","modes":["dark"],"dark":{"--color-bg":"#000"}}`)
	write("edited", `{"isley_theme":1,"id":"edited","name":"E","modes":["dark"],"dark":{"--color-bg":"red } body { display: none"}}`)
	write("renamed", `{"isley_theme":1,"id":"other","name":"R","modes":["dark"],"dark":{"--color-bg":"#000"}}`)
	write("forest", `{"isley_theme":1,"id":"forest","name":"F","modes":["dark"],"dark":{"--color-bg":"#000"}}`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "empty"), 0o755))

	r, err := Load(themeFS(map[string]string{"forest.css": forest}))
	require.NoError(t, err)
	skipped := r.SetUserDir(dir)
	assert.Len(t, skipped, 4)
	all := r.All()
	require.Len(t, all, 3)
	assert.Equal(t, "good", all[2].ID)
	css, _ := r.UserCSS()
	assert.NotContains(t, string(css), "display")
}

// The themes that ship come out as packages that go straight back in.
func TestExport_BuiltInThemesRoundTrip(t *testing.T) {
	t.Parallel()
	r, err := Load(repo())
	require.NoError(t, err)
	require.Empty(t, r.SetUserDir(t.TempDir()))
	for _, bt := range r.All() {
		p, err := r.Export(bt.ID)
		require.NoError(t, err, bt.ID)
		assert.Equal(t, bt.ID+"-custom", p.Manifest.ID)
		assert.NotNil(t, p.Logo, "%s has a logo", bt.ID)

		data, err := p.Zip()
		require.NoError(t, err)
		back, warnings, err := ReadPackage(data)
		require.NoError(t, err, bt.ID)
		assert.Empty(t, warnings, bt.ID)
		got, _, err := r.Install(back)
		require.NoError(t, err, bt.ID)
		assert.Equal(t, bt.Modes(), got.Modes(), bt.ID)
	}

	walnut, err := r.Export("walnut")
	require.NoError(t, err)
	assert.Equal(t, []string{"dark"}, walnut.Manifest.Modes)
	assert.Equal(t, "#b3703f", walnut.Manifest.Dark["--color-primary"])
	assert.Empty(t, walnut.Manifest.Light)

	sherbet, err := r.Export("sherbet")
	require.NoError(t, err)
	assert.Equal(t, "14px", sherbet.Manifest.Tokens["--radius"], "what light and dark share is in tokens")
	assert.NotContains(t, sherbet.Manifest.Light, "--radius")

	clarity, err := r.Export("clarity")
	require.NoError(t, err)
	assert.NotNil(t, clarity.LogoDark)

	_, err = r.Export("nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestParseColor(t *testing.T) {
	t.Parallel()
	near := func(want, got rgba) bool {
		d := func(a, b float64) bool { return a-b < 0.003 && b-a < 0.003 }
		return d(want.r, got.r) && d(want.g, got.g) && d(want.b, got.b) && d(want.a, got.a)
	}
	for value, want := range map[string]rgba{
		"#fff":                    {1, 1, 1, 1},
		"#0d6efd":                 {13.0 / 255, 110.0 / 255, 253.0 / 255, 1},
		"#0006":                   {0, 0, 0, 0.4},
		"#11223380":               {17.0 / 255, 34.0 / 255, 51.0 / 255, 128.0 / 255},
		"rgb(255, 0, 0)":          {1, 0, 0, 1},
		"rgba(18, 16, 14, 0.88)":  {18.0 / 255, 16.0 / 255, 14.0 / 255, 0.88},
		"rgb(100% 0% 0% / 50%)":   {1, 0, 0, 0.5},
		"hsl(0 70% 50%)":          {0.85, 0.15, 0.15, 1},
		"hsl(120deg, 100%, 25%)":  {0, 0.5, 0, 1},
		"hsla(240, 100%, 50%, 0)": {0, 0, 1, 0},
		"transparent":             {0, 0, 0, 0},
	} {
		got, ok := parseColor(value)
		if assert.True(t, ok, value) {
			assert.True(t, near(want, got), "%s: got %+v", value, got)
		}
	}
	for _, value := range []string{"red", "hsl(0.5turn 50% 50%)", "rgb(1, 2)", "#12", "hsl(120, 1, 0.5)"} {
		_, ok := parseColor(value)
		assert.False(t, ok, value)
	}
}

func TestContrastRatio(t *testing.T) {
	t.Parallel()
	c := func(v string) rgba { x, _ := parseColor(v); return x }
	assert.InDelta(t, 21, contrastRatio(c("#000"), c("#fff")), 0.01)
	assert.InDelta(t, 4.54, contrastRatio(c("#767676"), c("#fff")), 0.01)
	assert.InDelta(t, 4.54, contrastRatio(c("#fff"), c("#767676")), 0.01)
	// Half-transparent black on white lands as mid grey.
	assert.InDelta(t, contrastRatio(c("#808080"), c("#fff")), contrastRatio(c("rgba(0, 0, 0, 0.5)"), c("#fff")), 0.05)
}

func TestContrastIssues_OnlyPairsThePackageTouches(t *testing.T) {
	t.Parallel()
	classicLight, classicDark := classicValues(repo())
	require.Equal(t, "#212529", classicLight["--color-fg"])
	require.Equal(t, "#ffffff", classicDark["--color-fg"])

	// One bad colour in dark, nothing else set: only the pairs it is in.
	p, _, err := ReadPackage([]byte(`{"isley_theme":1,"name":"Dim","modes":["light","dark"],
		"dark":{"--color-muted":"#555555"},"light":{"--color-primary":"#0b5cad"}}`))
	require.NoError(t, err)
	issues := p.Manifest.contrastIssues(classicLight, classicDark)
	require.Len(t, issues, 2)
	for _, i := range issues {
		assert.Equal(t, "dark", i.Mode)
		assert.Equal(t, "--color-muted", i.Text)
		assert.Less(t, i.Ratio, MinContrast)
	}
	assert.Equal(t, "--color-card", issues[0].Background)
	assert.Equal(t, "--color-bg", issues[1].Background)
	assert.Equal(t, "2 colour pairs are below 4.5:1", ContrastSummary(issues))

	// "tokens" counts in both modes, and beats Classic's dark value.
	p, _, err = ReadPackage([]byte(`{"isley_theme":1,"name":"Ink","modes":["light","dark"],
		"tokens":{"--color-fg":"#222222"}}`))
	require.NoError(t, err)
	modes := map[string]int{}
	for _, i := range p.Manifest.contrastIssues(classicLight, classicDark) {
		assert.Equal(t, "--color-fg", i.Text)
		modes[i.Mode]++
	}
	assert.Zero(t, modes["light"], "dark grey on Classic's light surfaces reads")
	assert.Equal(t, 4, modes["dark"], "and on none of its dark ones")

	// A see-through background cannot be measured and is left alone.
	p, _, err = ReadPackage([]byte(`{"isley_theme":1,"name":"Glass","modes":["dark"],
		"dark":{"--color-card":"rgba(0, 0, 0, 0.2)","--color-fg":"#111111"}}`))
	require.NoError(t, err)
	for _, i := range p.Manifest.contrastIssues(classicLight, classicDark) {
		assert.NotEqual(t, "--color-card", i.Background)
	}
}

func TestContrast_InstalledThemesCarryTheirIssues(t *testing.T) {
	t.Parallel()
	r, err := Load(repo())
	require.NoError(t, err)
	require.Empty(t, r.SetUserDir(t.TempDir()))

	walnut, err := r.Export("walnut")
	require.NoError(t, err)
	got, _, err := r.Install(walnut)
	require.NoError(t, err)
	// Walnut's muted text is a known 4.1:1 on its surface.
	var muted *ContrastIssue
	for i := range got.LowContrast {
		if got.LowContrast[i].Text == "--color-muted" && got.LowContrast[i].Background == "--color-card" {
			muted = &got.LowContrast[i]
		}
	}
	if assert.NotNil(t, muted) {
		assert.Equal(t, "4.1", muted.RatioText())
	}
	assert.Equal(t, got.LowContrast, r.Resolve("walnut-custom").LowContrast)

	clarity, err := r.Export("clarity")
	require.NoError(t, err)
	got, _, err = r.Install(clarity)
	require.NoError(t, err)
	assert.Empty(t, got.LowContrast, "the high-contrast theme has none")
}

func TestReference_PairsAndTokensAreKnown(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, g := range Reference() {
		assert.NotEmpty(t, g.Title)
		for _, tok := range g.Tokens {
			assert.False(t, seen[tok.Name], "%s is listed twice", tok.Name)
			seen[tok.Name] = true
			assert.NotEmpty(t, tok.Desc, tok.Name)
			assert.NotEmpty(t, tok.Kind.Accepts(), tok.Name)
		}
	}
	for _, p := range ContrastPairs {
		assert.Equal(t, KindColor, tokenKinds[p.Text], p.Text)
		assert.Equal(t, KindColor, tokenKinds[p.Background], p.Background)
		assert.True(t, seen[p.Text] && seen[p.Background])
	}
}
