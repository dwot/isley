package handlers_test

// Installed themes: POST /settings/themes, GET /settings/themes/:id/export,
// DELETE /settings/themes/:id, and the public /themes/ routes pages load
// them from.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

const harbourManifest = `{
  "isley_theme": 1,
  "name": "Harbour",
  "modes": ["light", "dark"],
  "tokens": { "--radius": "4px" },
  "light": { "--color-primary": "#0b5cad" },
  "dark": { "--color-primary": "#4d9aff", "--from-the-future": "x" }
}`

func themeZip(t *testing.T, files map[string][]byte) []byte {
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

func themeUpload(t *testing.T, c *testutil.Client, apiKey, filename string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("theme", filename)
	require.NoError(t, err)
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/themes", apiKey, &body, mw.FormDataContentType()))
	require.NoError(t, err)
	return resp
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer testutil.DrainAndClose(resp)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}

func TestThemesHTTP_InstallServeExportRemove(t *testing.T) {
	t.Parallel()

	uploadDir := t.TempDir()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(uploadDir))
	const apiKey = "themes-key"
	testutil.SeedAPIKey(t, db, apiKey)
	c := server.NewClient(t)

	// Nothing installed: no stylesheet link, an empty stylesheet.
	assert.NotContains(t, bodyOf(t, c.Get("/login")), "/themes/user.css")

	resp := themeUpload(t, c, apiKey, "harbour.zip", themeZip(t, map[string][]byte{
		"theme.json": []byte(harbourManifest),
		"logo.png":   pngFixture(t),
	}))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var installed struct {
		Theme    struct{ ID, Name, Modes string }
		Replaced bool
		Warnings []string
	}
	require.NoError(t, json.Unmarshal([]byte(bodyOf(t, resp)), &installed))
	assert.Equal(t, "harbour", installed.Theme.ID)
	assert.Equal(t, "light dark", installed.Theme.Modes)
	assert.False(t, installed.Replaced)
	require.Len(t, installed.Warnings, 1)
	assert.Contains(t, installed.Warnings[0], "--from-the-future")
	assert.FileExists(t, filepath.Join(uploadDir, "themes", "harbour", "theme.json"))
	assert.FileExists(t, filepath.Join(uploadDir, "themes", "harbour", "logo.png"))

	// Pages list it and link the stylesheet, without a login.
	anon := server.NewClient(t)
	page := bodyOf(t, anon.Get("/login"))
	assert.Contains(t, page, `harbour:light dark;`)
	assert.Contains(t, page, `href="/themes/user.css?v=`)

	cssResp := anon.Get("/themes/user.css")
	require.Equal(t, http.StatusOK, cssResp.StatusCode)
	assert.Contains(t, cssResp.Header.Get("Content-Type"), "text/css")
	css := bodyOf(t, cssResp)
	assert.Contains(t, css, `[data-theme="harbour"][data-bs-theme="dark"]`)
	assert.Contains(t, css, `--logo: url("/themes/harbour/logo?v=`)
	assert.NotContains(t, css, "from-the-future")

	logo := anon.Get("/themes/harbour/logo")
	require.Equal(t, http.StatusOK, logo.StatusCode)
	assert.Equal(t, "image/png", logo.Header.Get("Content-Type"))
	assert.Equal(t, "nosniff", logo.Header.Get("X-Content-Type-Options"))
	testutil.DrainAndClose(logo)
	for _, path := range []string{"/themes/harbour/logo-dark", "/themes/harbour/theme.json", "/themes/nope/logo"} {
		r := anon.Get(path)
		assert.Equal(t, http.StatusNotFound, r.StatusCode, path)
		testutil.DrainAndClose(r)
	}

	// It can be the instance default.
	set, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/theme", apiKey,
		testutil.JSONBody(t, map[string]interface{}{"theme": "harbour"}), "application/json"))
	require.NoError(t, err)
	testutil.DrainAndClose(set)
	assert.Contains(t, bodyOf(t, anon.Get("/login")), `data-theme="harbour"`)

	// Export gives the package back.
	exp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/themes/harbour/export", apiKey, nil, ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, exp.StatusCode)
	assert.Contains(t, exp.Header.Get("Content-Disposition"), "harbour.isleytheme.zip")
	exported := bodyOf(t, exp)
	zr, err := zip.NewReader(bytes.NewReader([]byte(exported)), int64(len(exported)))
	require.NoError(t, err)
	names := []string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	assert.ElementsMatch(t, []string{"theme.json", "logo.png"}, names)

	// Uploading it again replaces it.
	again := themeUpload(t, c, apiKey, "harbour.zip", []byte(exported))
	require.Equal(t, http.StatusOK, again.StatusCode)
	assert.Contains(t, bodyOf(t, again), `"replaced":true`)

	// Removing it puts the instance back on Classic.
	del, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/settings/themes/harbour", apiKey, nil, ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, del.StatusCode)
	testutil.DrainAndClose(del)
	page = bodyOf(t, anon.Get("/login"))
	assert.Contains(t, page, `data-theme="classic"`)
	assert.NotContains(t, page, "harbour")
	assert.NoDirExists(t, filepath.Join(uploadDir, "themes", "harbour"))

	del, err = c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/settings/themes/harbour", apiKey, nil, ""))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, del.StatusCode)
	testutil.DrainAndClose(del)
}

func TestThemesHTTP_RejectsBadPackages(t *testing.T) {
	t.Parallel()

	uploadDir := t.TempDir()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(uploadDir))
	const apiKey = "themes-bad-key"
	testutil.SeedAPIKey(t, db, apiKey)
	c := server.NewClient(t)

	for name, data := range map[string][]byte{
		"css injection": []byte(`{"isley_theme":1,"name":"X","modes":["dark"],"dark":{"--color-bg":"#000; } body { display: none"}}`),
		"remote image":  []byte(`{"isley_theme":1,"name":"X","modes":["dark"],"dark":{"--bg-image":"url(https://example.com/a.png)"}}`),
		"built-in id":   []byte(`{"isley_theme":1,"id":"walnut","name":"X","modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"classic id":    []byte(`{"isley_theme":1,"id":"classic","name":"X","modes":["dark"],"dark":{"--color-bg":"#000"}}`),
		"not a package": []byte("body { color: red }"),
		"svg logo": themeZip(t, map[string][]byte{
			"theme.json": []byte(harbourManifest),
			"logo.png":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		}),
	} {
		resp := themeUpload(t, c, apiKey, "x.zip", data)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
		assert.Contains(t, bodyOf(t, resp), `"error"`, name)
	}
	assert.NoDirExists(t, filepath.Join(uploadDir, "themes", "x"))
	assert.NotContains(t, bodyOf(t, c.Get("/login")), "/themes/user.css")
}

func TestThemesHTTP_BuiltInThemes(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()))
	const apiKey = "themes-builtin-key"
	testutil.SeedAPIKey(t, db, apiKey)
	c := server.NewClient(t)

	// A built-in theme cannot be removed.
	del, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/settings/themes/walnut", apiKey, nil, ""))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, del.StatusCode)
	testutil.DrainAndClose(del)

	// It downloads as a copy that installs beside it.
	exp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/themes/walnut/export", apiKey, nil, ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, exp.StatusCode)
	assert.Contains(t, exp.Header.Get("Content-Disposition"), "walnut-custom.isleytheme.zip")
	resp := themeUpload(t, c, apiKey, "walnut-custom.zip", []byte(bodyOf(t, exp)))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := bodyOf(t, resp)
	assert.Contains(t, body, `"id":"walnut-custom"`)
	assert.Contains(t, body, `"warnings":[]`)
}

func TestThemesHTTP_RequireLogin(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()))
	c := server.NewClient(t)

	for _, req := range []struct{ method, path string }{
		{http.MethodPost, "/settings/themes"},
		{http.MethodGet, "/settings/themes/walnut/export"},
		{http.MethodDelete, "/settings/themes/walnut"},
	} {
		r, err := http.NewRequest(req.method, c.BaseURL+req.path, nil)
		require.NoError(t, err)
		resp, err := c.Do(r)
		require.NoError(t, err)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusFound, http.StatusSeeOther}, resp.StatusCode, "%s %s", req.method, req.path)
		testutil.DrainAndClose(resp)
	}
}

// A theme with a pair that is hard to read installs, and says so: in the
// answer, and on its card in Settings.
func TestThemesHTTP_LowContrastIsToldNotRefused(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()))
	const apiKey = "themes-contrast-key"
	testutil.SeedAPIKey(t, db, apiKey)
	c := server.NewClient(t)

	resp := themeUpload(t, c, apiKey, "theme.json", []byte(`{"isley_theme":1,"name":"Dim","modes":["dark"],
		"dark":{"--color-card":"#202020","--color-muted":"#555555"}}`))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got struct {
		Contrast []struct {
			Mode, Text, Background string
			Ratio                  float64
		}
	}
	require.NoError(t, json.Unmarshal([]byte(bodyOf(t, resp)), &got))
	require.NotEmpty(t, got.Contrast)
	assert.Equal(t, "dark", got.Contrast[0].Mode)
	assert.Equal(t, "--color-muted", got.Contrast[0].Text)
	assert.Equal(t, "--color-card", got.Contrast[0].Background)
	assert.Less(t, got.Contrast[0].Ratio, 4.5)

	testutil.SeedAdmin(t, db, "contrast-pw")
	page := bodyOf(t, server.LoginAsAdmin(t, "contrast-pw").Get("/settings"))
	assert.Contains(t, page, `class="theme-contrast"`)
	assert.Contains(t, page, `<code>--color-muted</code>`)
}

func TestThemesHTTP_ReferencePage(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()))
	testutil.SeedAdmin(t, db, "reference-pw")

	// Like the rest of Settings, it is not a guest page.
	anon := server.NewClient(t).Get("/settings/themes/reference")
	assert.NotEqual(t, http.StatusOK, anon.StatusCode)
	testutil.DrainAndClose(anon)

	resp := server.LoginAsAdmin(t, "reference-pw").Get("/settings/themes/reference")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page := bodyOf(t, resp)
	for _, want := range []string{
		`data-token="--color-bg"`, `data-token="--bg-image" data-kind="image"`, `data-value-of="--radius"`,
		`data-pair-text="--color-on-stage" data-pair-background="--stage-veg"`, `data-min-contrast="4.5"`,
		"Cards, dialogs, menus and tables.",
	} {
		assert.Contains(t, page, want)
	}
}

// The editor: its page, the check it asks for as the theme changes, and
// the save.
func TestThemesHTTP_Editor(t *testing.T) {
	t.Parallel()

	uploadDir := t.TempDir()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(uploadDir))
	const apiKey = "themes-editor-key"
	testutil.SeedAPIKey(t, db, apiKey)
	testutil.SeedAdmin(t, db, "editor-pw")
	c := server.NewClient(t)
	session := server.LoginAsAdmin(t, "editor-pw")

	// A built-in theme opens as a new one, under a free id.
	page := bodyOf(t, session.Get("/settings/themes/walnut/edit"))
	assert.Contains(t, page, `data-from="walnut" data-new="true"`)
	assert.Contains(t, page, `&#34;id&#34;: &#34;walnut-custom&#34;`)
	assert.Contains(t, page, `data-token="--color-bg" data-kind="color"`)
	missing := session.Get("/settings/themes/nope/edit")
	assert.Equal(t, http.StatusFound, missing.StatusCode)
	testutil.DrainAndClose(missing)

	check := func(manifest string) string {
		resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/themes/check", apiKey, bytes.NewBufferString(manifest), "application/json"))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		return bodyOf(t, resp)
	}
	assert.Contains(t, check(`{"isley_theme":1,"name":"Ok","modes":["dark"],"dark":{"--color-bg":"#000000"}}`), `"ok":true`)
	low := check(`{"isley_theme":1,"name":"Low","modes":["dark"],"dark":{"--color-muted":"#444444"}}`)
	assert.Contains(t, low, `"ok":true`)
	assert.Contains(t, low, `"text":"--color-muted"`)
	bad := check(`{"isley_theme":1,"name":"Bad","modes":["dark"],"dark":{"--color-bg":"url(x)"}}`)
	assert.Contains(t, bad, `"ok":false`)
	assert.Contains(t, bad, "--color-bg")
	assert.NoDirExists(t, filepath.Join(uploadDir, "themes"), "a check stores nothing")

	save := func(fields map[string]string, files map[string][]byte) *http.Response {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for k, v := range fields {
			require.NoError(t, mw.WriteField(k, v))
		}
		for k, v := range files {
			fw, err := mw.CreateFormFile(k, k+".png")
			require.NoError(t, err)
			_, err = fw.Write(v)
			require.NoError(t, err)
		}
		require.NoError(t, mw.Close())
		resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/themes/save", apiKey, &body, mw.FormDataContentType()))
		require.NoError(t, err)
		return resp
	}
	const copper = `{"isley_theme":1,"id":"copper","name":"Copper","modes":["dark"],"dark":{"--color-primary":"#b3703f"}}`

	// New, from Walnut: Walnut's logo comes along.
	resp := save(map[string]string{"manifest": copper, "from": "walnut", "new": "true"}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	assert.FileExists(t, filepath.Join(uploadDir, "themes", "copper", "logo.png"))

	// The same id again as new is refused; so is a built-in id.
	resp = save(map[string]string{"manifest": copper, "from": "walnut", "new": "true"}, nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)
	resp = save(map[string]string{"manifest": `{"isley_theme":1,"id":"sherbet","name":"S","modes":["dark"],"dark":{"--color-bg":"#000000"}}`, "from": "classic", "new": "true"}, nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)

	// An installed theme opens to be edited in place.
	page = bodyOf(t, session.Get("/settings/themes/copper/edit"))
	assert.Contains(t, page, `data-from="copper" data-new="false"`)

	// Saving it with a new logo replaces the logo and the values.
	own := pngFixture(t)
	resp = save(map[string]string{
		"manifest": `{"isley_theme":1,"id":"copper","name":"Copper","modes":["dark"],"dark":{"--color-primary":"#c9a05a"}}`,
		"from":     "copper", "new": "false",
	}, map[string][]byte{"logo": own})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	stored, err := os.ReadFile(filepath.Join(uploadDir, "themes", "copper", "logo.png"))
	require.NoError(t, err)
	assert.Equal(t, own, stored)
	assert.Contains(t, bodyOf(t, c.Get("/themes/user.css")), "--color-primary: #c9a05a")

	// Saved again without one, the logo stays.
	resp = save(map[string]string{"manifest": copper, "from": "copper", "new": "false"}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	stored, err = os.ReadFile(filepath.Join(uploadDir, "themes", "copper", "logo.png"))
	require.NoError(t, err)
	assert.Equal(t, own, stored)

	// An edit cannot save over another theme, or one that is gone.
	resp = save(map[string]string{"manifest": copper, "from": "walnut", "new": "false"}, nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)
	resp = save(map[string]string{"manifest": `{"isley_theme":1,"id":"gone","name":"G","modes":["dark"],"dark":{"--color-bg":"#000000"}}`, "from": "gone", "new": "false"}, nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)

	// A bad value or a bad logo is refused like an upload.
	resp = save(map[string]string{"manifest": `{"isley_theme":1,"id":"x","name":"X","modes":["dark"],"dark":{"--color-bg":"#000; } * { display: none"}}`, "from": "classic", "new": "true"}, nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)
	resp = save(map[string]string{"manifest": `{"isley_theme":1,"id":"x","name":"X","modes":["dark"],"dark":{"--color-bg":"#000000"}}`, "from": "classic", "new": "true"},
		map[string][]byte{"logo": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)
	assert.NoDirExists(t, filepath.Join(uploadDir, "themes", "x"))
}
