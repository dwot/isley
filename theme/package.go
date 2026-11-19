package theme

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // logo formats
	_ "image/png"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	_ "golang.org/x/image/webp"
)

// A theme package is how a theme that does not ship with Isley is handed
// to it: a zip with a theme.json and, optionally, logo.png and
// logo-dark.png (or .jpg, .webp). theme.json alone is a package too.
//
//	{
//	  "isley_theme": 1,
//	  "id": "harbour",
//	  "name": "Harbour",
//	  "modes": ["light", "dark"],
//	  "tokens": { "--radius": "4px" },
//	  "light":  { "--color-primary": "#0b5cad" },
//	  "dark":   { "--color-primary": "#4d9aff" }
//	}
//
// "tokens" applies in every mode, "light" and "dark" in their own, over
// "tokens". A token the package leaves out keeps Classic's value for that
// mode. The package holds values
// only; Isley writes the stylesheet (see CSS), so a package cannot carry
// rules of its own.

// FormatVersion is the "isley_theme" this build reads and writes.
const FormatVersion = 1

// Limits on what a package may hold.
const (
	MaxPackageSize  = 2 << 20   // the upload
	maxManifestSize = 128 << 10 // theme.json
	maxLogoSize     = 512 << 10 // each logo
	maxLogoSide     = 2048      // pixels
	maxNameLen      = 40
	maxIDLen        = 40
)

// ManifestName is the manifest's file name inside a package.
const ManifestName = "theme.json"

// Manifest is theme.json.
type Manifest struct {
	Format      int               `json:"isley_theme"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Author      string            `json:"author,omitempty"`
	Description string            `json:"description,omitempty"`
	Modes       []string          `json:"modes"`
	Tokens      map[string]string `json:"tokens,omitempty"`
	Light       map[string]string `json:"light,omitempty"`
	Dark        map[string]string `json:"dark,omitempty"`
}

// Image is a logo as it arrived: the bytes and the extension its content
// calls for (".png", ".jpg", ".webp").
type Image struct {
	Data []byte
	Ext  string
}

// Package is a theme package, read and checked.
type Package struct {
	Manifest Manifest
	Logo     *Image
	LogoDark *Image
}

func (m Manifest) has(mode string) bool {
	for _, x := range m.Modes {
		if x == mode {
			return true
		}
	}
	return false
}

var slugDropRe = regexp.MustCompile(`[^a-z0-9]+`)

// slug makes a theme id out of a name: "Deep Sea!" is "deep-sea".
func slug(name string) string {
	return strings.Trim(slugDropRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

func cleanText(s string, max int, field string) (string, error) {
	s = strings.TrimSpace(s)
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%s has a control character", field)
		}
	}
	if len([]rune(s)) > max {
		return "", fmt.Errorf("%s is longer than %d characters", field, max)
	}
	return s, nil
}

// normalize checks the manifest and puts it in the form it is stored in.
// Tokens this build does not know are dropped and named in the warnings: a
// package made for a newer Isley still loads.
func (m *Manifest) normalize() (warnings []string, err error) {
	if m.Format == 0 {
		return nil, fmt.Errorf("theme.json has no \"isley_theme\" version")
	}
	if m.Format > FormatVersion {
		return nil, fmt.Errorf("theme.json is format %d; this Isley reads up to %d", m.Format, FormatVersion)
	}
	if m.Name, err = cleanText(m.Name, maxNameLen, "name"); err != nil {
		return nil, err
	}
	if m.Name == "" {
		return nil, fmt.Errorf("theme.json has no \"name\"")
	}
	if m.Author, err = cleanText(m.Author, 80, "author"); err != nil {
		return nil, err
	}
	if m.Description, err = cleanText(m.Description, 300, "description"); err != nil {
		return nil, err
	}
	if m.ID == "" {
		m.ID = slug(m.Name)
	}
	if !ValidID(m.ID) || len(m.ID) > maxIDLen {
		return nil, fmt.Errorf("id %q must be lower-case letters, digits and hyphens, %d at most", m.ID, maxIDLen)
	}

	var light, dark bool
	for _, mode := range m.Modes {
		switch mode {
		case "light":
			light = true
		case "dark":
			dark = true
		default:
			return nil, fmt.Errorf("unknown mode %q", mode)
		}
	}
	m.Modes = nil
	if light {
		m.Modes = append(m.Modes, "light")
	}
	if dark {
		m.Modes = append(m.Modes, "dark")
	}
	if len(m.Modes) == 0 {
		return nil, fmt.Errorf("theme.json needs \"modes\": [\"light\"], [\"dark\"] or both")
	}
	if !light && len(m.Light) > 0 {
		return nil, fmt.Errorf("\"light\" has tokens but \"modes\" does not list light")
	}
	if !dark && len(m.Dark) > 0 {
		return nil, fmt.Errorf("\"dark\" has tokens but \"modes\" does not list dark")
	}

	unknown := map[string]bool{}
	for _, set := range []struct {
		name   string
		tokens map[string]string
	}{{"tokens", m.Tokens}, {"light", m.Light}, {"dark", m.Dark}} {
		for name, value := range set.tokens {
			if !KnownToken(name) {
				unknown[name] = true
				delete(set.tokens, name)
				continue
			}
			clean, err := checkValue(name, value)
			if err != nil {
				return nil, fmt.Errorf("%s in \"%s\": %w", name, set.name, err)
			}
			set.tokens[name] = clean
		}
	}
	if len(m.Tokens)+len(m.Light)+len(m.Dark) == 0 {
		return nil, fmt.Errorf("the theme sets no tokens")
	}
	if len(unknown) > 0 {
		names := make([]string, 0, len(unknown))
		for name := range unknown {
			// The name goes back to the browser; keep it plain.
			if len(name) <= 60 && tokenNameRe.MatchString(name) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		warnings = append(warnings, fmt.Sprintf("%d unknown token(s) ignored: %s", len(unknown), strings.Join(names, ", ")))
	}
	return warnings, nil
}

var tokenNameRe = regexp.MustCompile(`^--[a-z0-9-]+$`)

// ReadPackage reads an upload: a zip, or a bare theme.json.
func ReadPackage(data []byte) (*Package, []string, error) {
	if len(data) > MaxPackageSize {
		return nil, nil, fmt.Errorf("the package is larger than %d MB", MaxPackageSize>>20)
	}
	p := &Package{}
	var manifest []byte

	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '{' {
		manifest = trimmed
	} else {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, nil, fmt.Errorf("not a zip or a theme.json")
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			// Files may sit in one folder, as a zipped directory does.
			base := strings.ToLower(path.Base(strings.ReplaceAll(f.Name, `\`, "/")))
			stem := strings.TrimSuffix(base, path.Ext(base))
			switch {
			case base == ManifestName:
				if manifest != nil {
					return nil, nil, fmt.Errorf("the package has more than one theme.json")
				}
				if manifest, err = readEntry(f, maxManifestSize); err != nil {
					return nil, nil, err
				}
			case stem == "logo" || stem == "logo-dark":
				raw, err := readEntry(f, maxLogoSize)
				if err != nil {
					return nil, nil, err
				}
				img, err := readImage(raw)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: %w", base, err)
				}
				if stem == "logo" {
					p.Logo = img
				} else {
					p.LogoDark = img
				}
			}
		}
		if manifest == nil {
			return nil, nil, fmt.Errorf("the package has no theme.json")
		}
	}
	if len(manifest) > maxManifestSize {
		return nil, nil, fmt.Errorf("theme.json is larger than %d KB", maxManifestSize>>10)
	}
	if err := json.Unmarshal(manifest, &p.Manifest); err != nil {
		return nil, nil, fmt.Errorf("theme.json is not valid JSON: %v", err)
	}
	warnings, err := p.Manifest.normalize()
	if err != nil {
		return nil, nil, err
	}
	if p.LogoDark != nil && p.Logo == nil {
		// One logo is the logo, whatever it was called.
		p.Logo, p.LogoDark = p.LogoDark, nil
	}
	return p, warnings, nil
}

func readEntry(f *zip.File, max int) ([]byte, error) {
	name := path.Base(f.Name)
	if f.UncompressedSize64 > uint64(max) {
		return nil, fmt.Errorf("%s is larger than %d KB", name, max>>10)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read", name)
	}
	defer rc.Close()
	// The declared size is the archive's word; read no more than the cap.
	data, err := io.ReadAll(io.LimitReader(rc, int64(max)+1))
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read", name)
	}
	if len(data) > max {
		return nil, fmt.Errorf("%s is larger than %d KB", name, max>>10)
	}
	return data, nil
}

var imageExt = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}

// readImage accepts a PNG, JPEG or WebP that decodes, by its content and
// not its name. SVG is left out on purpose: served from Isley's own
// address it could carry script.
func readImage(data []byte) (*Image, error) {
	ext, ok := imageExt[http.DetectContentType(data)]
	if !ok {
		return nil, fmt.Errorf("must be a PNG, JPEG or WebP image")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("is not a readable image")
	}
	if cfg.Width > maxLogoSide || cfg.Height > maxLogoSide {
		return nil, fmt.Errorf("is larger than %d pixels a side", maxLogoSide)
	}
	return &Image{Data: data, Ext: ext}, nil
}

// Zip writes the package as it is uploaded.
func (p *Package) Zip() ([]byte, error) {
	manifest, err := p.Manifest.JSON()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := add(ManifestName, manifest); err != nil {
		return nil, err
	}
	if p.Logo != nil {
		if err := add("logo"+p.Logo.Ext, p.Logo.Data); err != nil {
			return nil, err
		}
	}
	if p.LogoDark != nil {
		if err := add("logo-dark"+p.LogoDark.Ext, p.LogoDark.Data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// JSON is the manifest as theme.json.
func (m Manifest) JSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CSS writes the theme's stylesheet: the manifest's values under the
// selectors a built-in theme uses. logo and logoDark are URLs, or "".
// Every value has been through checkValue, and the id through ValidID, so
// nothing here can close a rule early.
func (m Manifest) CSS(logo, logoDark string) string {
	sel := `[data-theme="` + m.ID + `"]`
	darkSel := sel + `[data-bs-theme="dark"]`
	var b strings.Builder
	block := func(selector string, logoURL string, sets ...map[string]string) {
		merged := map[string]string{}
		for _, set := range sets {
			for name, value := range set {
				merged[name] = value
			}
		}
		if logoURL != "" {
			merged["--logo"] = `url("` + logoURL + `")`
		}
		if len(merged) == 0 {
			return
		}
		names := make([]string, 0, len(merged))
		for name := range merged {
			names = append(names, name)
		}
		sort.Strings(names)
		b.WriteString(selector + " {\n")
		for _, name := range names {
			b.WriteString("    " + name + ": " + merged[name] + ";\n")
		}
		b.WriteString("}\n")
	}
	switch {
	case m.has("light") && m.has("dark"):
		// The shared block comes after Classic's dark values at the same
		// weight, so it holds in both modes; a mode's own block outweighs
		// it.
		block(sel, logo, m.Tokens)
		block(sel+`[data-bs-theme="light"]`, "", m.Light)
		block(darkSel, logoDark, m.Dark)
	case m.has("dark"):
		if logoDark == "" {
			logoDark = logo
		}
		block(sel+",\n"+darkSel, logoDark, m.Tokens, m.Dark)
	default:
		block(sel, logo, m.Tokens, m.Light)
	}
	return b.String()
}

// NewPackage makes a package from its parts, as the theme editor sends
// them: theme.json and the logos, either of which may be nil. They are
// checked exactly as an upload is.
func NewPackage(manifest, logo, logoDark []byte) (*Package, []string, error) {
	if len(manifest) > maxManifestSize {
		return nil, nil, fmt.Errorf("theme.json is larger than %d KB", maxManifestSize>>10)
	}
	p := &Package{}
	if err := json.Unmarshal(manifest, &p.Manifest); err != nil {
		return nil, nil, fmt.Errorf("theme.json is not valid JSON: %v", err)
	}
	warnings, err := p.Manifest.normalize()
	if err != nil {
		return nil, nil, err
	}
	for _, l := range []struct {
		name string
		data []byte
		dst  **Image
	}{{"logo", logo, &p.Logo}, {"dark logo", logoDark, &p.LogoDark}} {
		if l.data == nil {
			continue
		}
		if len(l.data) > maxLogoSize {
			return nil, nil, fmt.Errorf("the %s is larger than %d KB", l.name, maxLogoSize>>10)
		}
		img, err := readImage(l.data)
		if err != nil {
			return nil, nil, fmt.Errorf("the %s %w", l.name, err)
		}
		*l.dst = img
	}
	return p, warnings, nil
}
