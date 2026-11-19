package theme

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Installed themes live on disk, one directory each, under the uploads
// folder so they travel with a backup that includes images:
//
//	<uploads>/themes/<id>/theme.json
//	<uploads>/themes/<id>/logo.png        (optional; .jpg, .webp)
//	<uploads>/themes/<id>/logo-dark.png   (optional)
//
// The directory holds what the package held. The stylesheet is not
// stored: it is written from theme.json when the themes are loaded, so a
// newer Isley reads an old package its own way.

// UserDirName is the folder under the uploads directory.
const UserDirName = "themes"

// MaxInstalled is how many themes an instance may have installed.
const MaxInstalled = 50

// ErrNotFound is returned for a theme id the registry does not have, and
// ErrBuiltIn for an operation a built-in theme does not allow.
var (
	ErrNotFound = errors.New("no such theme")
	ErrBuiltIn  = errors.New("the theme ships with Isley")
)

type installed struct {
	theme    Theme
	manifest Manifest
	logo     string // file path, or ""
	logoDark string
}

// SetUserDir points the registry at the directory installed themes are
// kept in and loads them. The directory need not exist yet. The errors
// returned are the themes that were skipped, one each; the rest loaded.
func (r *Registry) SetUserDir(dir string) []error {
	r.mu.Lock()
	r.userDir = dir
	r.mu.Unlock()
	return r.Reload()
}

// Reload reads the installed themes again, after something other than
// Install or Remove changed the directory (a restore).
func (r *Registry) Reload() []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reload()
}

func (r *Registry) reload() []error {
	r.user, r.userCSS, r.userVer = nil, nil, ""
	if r.userDir == "" {
		return nil
	}
	entries, err := os.ReadDir(r.userDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return []error{fmt.Errorf("read installed themes: %w", err)}
	}
	var skipped []error
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		u, err := r.readInstalled(e.Name())
		if err != nil {
			skipped = append(skipped, fmt.Errorf("theme %s: %w", e.Name(), err))
			continue
		}
		r.user = append(r.user, u)
		if len(r.user) == MaxInstalled {
			break
		}
	}
	sort.Slice(r.user, func(i, j int) bool {
		a, b := r.user[i].theme, r.user[j].theme
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})

	var css strings.Builder
	sum := sha256.New()
	for _, u := range r.user {
		css.WriteString(u.manifest.CSS(logoURL(u.theme.ID, "logo", u.logo), logoURL(u.theme.ID, "logo-dark", u.logoDark)))
	}
	r.userCSS = []byte(css.String())
	sum.Write(r.userCSS)
	r.userVer = fmt.Sprintf("%x", sum.Sum(nil)[:4])
	return skipped
}

// logoURL is where the logo is served from, with a version taken from the
// file so a replaced logo is fetched again.
func logoURL(id, name, file string) string {
	if file == "" {
		return ""
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("/themes/%s/%s?v=%x", id, name, sum[:4])
}

func (r *Registry) readInstalled(id string) (installed, error) {
	if _, taken := r.byID[id]; taken {
		return installed{}, fmt.Errorf("the id belongs to a built-in theme")
	}
	dir := filepath.Join(r.userDir, id)
	info, err := os.Stat(filepath.Join(dir, ManifestName))
	if err != nil {
		return installed{}, fmt.Errorf("no theme.json")
	}
	if info.Size() > maxManifestSize {
		return installed{}, fmt.Errorf("theme.json is too large")
	}
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return installed{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return installed{}, fmt.Errorf("theme.json is not valid JSON")
	}
	// Checked again on every load: the file may have been edited in
	// place, or come from a restore.
	if _, err := m.normalize(); err != nil {
		return installed{}, err
	}
	if m.ID != id {
		return installed{}, fmt.Errorf("theme.json names the theme %q", m.ID)
	}
	u := installed{
		manifest: m,
		theme: Theme{
			ID: id, Name: m.Name, Light: m.has("light"), Dark: m.has("dark"), User: true,
			LowContrast: m.contrastIssues(r.classicLight, r.classicDark),
		},
		logo:     findLogo(dir, "logo"),
		logoDark: findLogo(dir, "logo-dark"),
	}
	return u, nil
}

func findLogo(dir, stem string) string {
	for _, ext := range []string{".png", ".jpg", ".webp"} {
		p := filepath.Join(dir, stem+ext)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && info.Size() <= maxLogoSize {
			return p
		}
	}
	return ""
}

// UserCSS is the stylesheet for every installed theme, and a version that
// changes when it does. Both are empty when nothing is installed.
func (r *Registry) UserCSS() (css []byte, version string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.user) == 0 {
		return nil, ""
	}
	return r.userCSS, r.userVer
}

// LogoFile is the path of an installed theme's logo on disk.
func (r *Registry) LogoFile(id string, dark bool) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.user {
		if u.theme.ID != id {
			continue
		}
		file := u.logo
		if dark {
			file = u.logoDark
		}
		return file, file != ""
	}
	return "", false
}

// Install stores a package and adds its theme, replacing an installed
// theme with the same id. replaced says whether it did.
func (r *Registry) Install(p *Package) (t Theme, replaced bool, err error) {
	id := p.Manifest.ID
	if _, taken := r.byID[id]; taken {
		return Theme{}, false, fmt.Errorf("the id %q belongs to a theme that ships with Isley; give the package another id", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.userDir == "" {
		return Theme{}, false, fmt.Errorf("this instance has nowhere to keep themes")
	}
	for _, u := range r.user {
		if u.theme.ID == id {
			replaced = true
		}
	}
	if !replaced && len(r.user) >= MaxInstalled {
		return Theme{}, false, fmt.Errorf("%d themes are installed already; remove one first", MaxInstalled)
	}

	manifest, err := p.Manifest.JSON()
	if err != nil {
		return Theme{}, false, err
	}
	if err := os.MkdirAll(r.userDir, 0o755); err != nil {
		return Theme{}, false, fmt.Errorf("create themes folder: %w", err)
	}
	// Written whole beside its place, then moved in, so a failure leaves
	// the theme that was there.
	tmp, err := os.MkdirTemp(r.userDir, ".new-")
	if err != nil {
		return Theme{}, false, fmt.Errorf("store theme: %w", err)
	}
	defer os.RemoveAll(tmp)
	if err := os.Chmod(tmp, 0o755); err != nil {
		return Theme{}, false, fmt.Errorf("store theme: %w", err)
	}
	files := map[string][]byte{ManifestName: manifest}
	if p.Logo != nil {
		files["logo"+p.Logo.Ext] = p.Logo.Data
	}
	if p.LogoDark != nil {
		files["logo-dark"+p.LogoDark.Ext] = p.LogoDark.Data
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0o644); err != nil {
			return Theme{}, false, fmt.Errorf("store theme: %w", err)
		}
	}
	dest := filepath.Join(r.userDir, id)
	if err := os.RemoveAll(dest); err != nil {
		return Theme{}, false, fmt.Errorf("store theme: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		r.reload()
		return Theme{}, false, fmt.Errorf("store theme: %w", err)
	}
	r.reload()
	for _, u := range r.user {
		if u.theme.ID == id {
			return u.theme, replaced, nil
		}
	}
	return Theme{}, false, fmt.Errorf("the theme was stored but could not be read back")
}

// Remove deletes an installed theme.
func (r *Registry) Remove(id string) error {
	if _, builtin := r.byID[id]; builtin {
		return ErrBuiltIn
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// The directory removed is named by the registry's own record of the
	// theme, never by the id the caller passed in.
	dir := ""
	for _, u := range r.user {
		if u.theme.ID == id {
			dir = u.theme.ID
		}
	}
	if dir == "" {
		return ErrNotFound
	}
	if err := os.RemoveAll(filepath.Join(r.userDir, dir)); err != nil {
		return fmt.Errorf("remove theme: %w", err)
	}
	r.reload()
	return nil
}

// Export returns a theme as a package. An installed theme comes back as
// it was stored. A built-in theme is read out of its stylesheet and given
// a new id and name ("walnut-custom", "Walnut Custom"), so the package
// installs beside the original as a starting point for a new theme.
func (r *Registry) Export(id string) (*Package, error) {
	if t, ok := r.byID[id]; ok {
		return r.exportBuiltIn(t)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.user {
		if u.theme.ID != id {
			continue
		}
		p := &Package{Manifest: u.manifest}
		p.Manifest.Format = FormatVersion
		for _, l := range []struct {
			file string
			dst  **Image
		}{{u.logo, &p.Logo}, {u.logoDark, &p.LogoDark}} {
			if l.file == "" {
				continue
			}
			data, err := os.ReadFile(l.file)
			if err != nil {
				return nil, fmt.Errorf("read logo: %w", err)
			}
			img, err := readImage(data)
			if err != nil {
				return nil, fmt.Errorf("logo %w", err)
			}
			*l.dst = img
		}
		return p, nil
	}
	return nil, ErrNotFound
}

var (
	commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	ruleRe    = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	declRe    = regexp.MustCompile(`^\s*(--[a-z0-9-]+)\s*:\s*(.+?)\s*$`)
	logoRe    = regexp.MustCompile(`^url\("(/static/[A-Za-z0-9/_.-]+)"\)$`)
)

// exportBuiltIn turns a built-in theme's stylesheet into a package. Only
// token values carry over: a rule a built-in theme adds beyond its tokens
// (Clarity underlines links) has no place in a package.
func (r *Registry) exportBuiltIn(t Theme) (*Package, error) {
	file := "web/static/css/tokens.css"
	lightSel, darkSel := ":root", `[data-bs-theme="dark"]`
	if t.ID != DefaultID {
		file = Dir + "/" + t.ID + ".css"
		lightSel = `[data-theme="` + t.ID + `"]`
		darkSel = lightSel + `[data-bs-theme="dark"]`
	}
	data, err := fs.ReadFile(r.assets, file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}

	light, dark := map[string]string{}, map[string]string{}
	logos := map[string]string{}
	for _, rule := range ruleRe.FindAllStringSubmatch(commentRe.ReplaceAllString(string(data), ""), -1) {
		var targets []map[string]string
		var modes []string
		for _, sel := range strings.Split(rule[1], ",") {
			switch strings.TrimSpace(sel) {
			case lightSel:
				targets, modes = append(targets, light), append(modes, "light")
			case darkSel:
				targets, modes = append(targets, dark), append(modes, "dark")
			}
		}
		for _, decl := range strings.Split(rule[2], ";") {
			d := declRe.FindStringSubmatch(spacesRe.ReplaceAllString(decl, " "))
			if d == nil {
				continue
			}
			if d[1] == "--logo" {
				if l := logoRe.FindStringSubmatch(d[2]); l != nil {
					for _, mode := range modes {
						logos[mode] = l[1]
					}
				}
				continue
			}
			// Classic's component tokens are worked out from other
			// tokens; a package holds plain values, and leaving one out
			// keeps the default.
			clean, err := checkValue(d[1], d[2])
			if err != nil {
				continue
			}
			for _, target := range targets {
				target[d[1]] = clean
			}
		}
	}

	m := Manifest{
		Format: FormatVersion,
		ID:     t.ID + "-custom",
		Name:   t.Name + " Custom",
	}
	if t.Light {
		m.Modes = append(m.Modes, "light")
		m.Light = light
	}
	if t.Dark {
		m.Modes = append(m.Modes, "dark")
		m.Dark = dark
	}
	if t.Light && t.Dark {
		// In a stylesheet the light rule also holds in dark wherever the
		// dark rule is silent. A package's modes are separate, so dark
		// states those values itself, and what the two share is said
		// once.
		for name, value := range light {
			if _, set := dark[name]; !set {
				dark[name] = value
			}
		}
		m.Tokens = map[string]string{}
		for name, value := range light {
			if dark[name] == value {
				m.Tokens[name] = value
				delete(light, name)
				delete(dark, name)
			}
		}
	}
	p := &Package{Manifest: m}
	first := "light"
	if !t.Light {
		first = "dark"
	}
	if p.Logo, err = r.builtInLogo(logos[first]); err != nil {
		return nil, err
	}
	if t.Light && t.Dark && logos["dark"] != logos["light"] {
		if p.LogoDark, err = r.builtInLogo(logos["dark"]); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (r *Registry) builtInLogo(url string) (*Image, error) {
	if url == "" {
		return nil, nil
	}
	data, err := fs.ReadFile(r.assets, "web"+url)
	if err != nil {
		return nil, fmt.Errorf("read logo %s: %w", url, err)
	}
	img, err := readImage(data)
	if err != nil {
		return nil, fmt.Errorf("logo %s %w", url, err)
	}
	return img, nil
}

// Has reports whether a theme with this id exists, built-in or installed.
func (r *Registry) Has(id string) bool {
	return r.Resolve(id).ID == id
}

// BuiltIn reports whether id is a theme that ships with Isley.
func (r *Registry) BuiltIn(id string) bool {
	_, ok := r.byID[id]
	return ok
}

// FreeID returns id, or id with a number after it, whichever no theme has
// yet: "walnut-custom", "walnut-custom-2".
func (r *Registry) FreeID(id string) string {
	if !r.Has(id) {
		return id
	}
	for n := 2; ; n++ {
		if next := fmt.Sprintf("%s-%d", id, n); !r.Has(next) {
			return next
		}
	}
}

// Contrast measures a checked manifest the way an installed theme is
// measured, for a theme that is still being edited.
func (r *Registry) Contrast(m Manifest) []ContrastIssue {
	return m.contrastIssues(r.classicLight, r.classicDark)
}
