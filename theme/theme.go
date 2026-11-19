// Package theme finds the themes an Isley build ships with, and keeps the
// ones an instance installs.
//
// A theme is one stylesheet in web/static/css/themes. It gives new values
// to the tokens in web/static/css/tokens.css, under [data-theme="<id>"] for
// light and [data-theme="<id>"][data-bs-theme="dark"] for dark, and starts
// with a comment that names it:
//
//	/* theme: Forest
//	 * modes: light dark
//	 */
//
// The file name is the theme's id. Adding a theme is adding a file; nothing
// else lists them. The default theme, Classic, is tokens.css itself and has
// no file.
//
// An instance can install more themes from packages: token values in a
// theme.json, from which this package writes the stylesheet. See
// package.go for the format and user.go for where they are kept.
package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// DefaultID is the theme an instance uses until an admin picks another, and
// the one a page falls back to when its theme is unknown.
const DefaultID = "classic"

// Dir is where theme stylesheets live in the assets file system.
const Dir = "web/static/css/themes"

// Theme is one entry in the registry.
type Theme struct {
	ID   string
	Name string
	// Light and Dark say which modes the theme has. At least one is true.
	Light bool
	Dark  bool
	// File is the stylesheet's URL path under /static, or "" for Classic
	// and for installed themes, whose rules all come from UserCSS.
	File string
	// User is true for a theme installed from a package, false for one
	// that ships with Isley.
	User bool
	// LowContrast lists the colour pairs an installed theme leaves hard
	// to read. It is not worked out for built-in themes.
	LowContrast []ContrastIssue
}

// Modes lists the theme's modes as the page's theme script reads them:
// "light dark", "light" or "dark".
func (t Theme) Modes() string {
	switch {
	case t.Light && t.Dark:
		return "light dark"
	case t.Dark:
		return "dark"
	default:
		return "light"
	}
}

// DefaultMode is the mode a page starts in before the browser's own choice
// applies. Isley has always started dark.
func (t Theme) DefaultMode() string {
	if t.Dark {
		return "dark"
	}
	return "light"
}

// Registry is the set of themes: the ones that ship with Isley, Classic
// first and the rest by name, then the installed ones by name. The
// built-in set never changes; the installed set does (see user.go).
type Registry struct {
	assets fs.FS
	themes []Theme
	byID   map[string]Theme

	// Classic's plain values, for measuring a package's contrast.
	classicLight, classicDark map[string]string

	mu      sync.RWMutex
	userDir string
	user    []installed
	userCSS []byte
	userVer string
}

var (
	idRe     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	headerRe = regexp.MustCompile(`(?s)^\s*/\*(.*?)\*/`)
	fieldRe  = regexp.MustCompile(`(?m)^[\s*]*(theme|modes)\s*:\s*(.+?)\s*$`)
)

// ValidID reports whether id has the shape of a theme id. It does not say
// the theme exists.
func ValidID(id string) bool {
	return len(id) <= 64 && idRe.MatchString(id)
}

// Load reads the themes from assets. A theme file that cannot be read as
// one is an error: it would otherwise be a theme that silently never
// appears.
func Load(assets fs.FS) (*Registry, error) {
	r := &Registry{assets: assets, byID: map[string]Theme{}}
	r.classicLight, r.classicDark = classicValues(assets)
	r.add(Theme{ID: DefaultID, Name: "Classic", Light: true, Dark: true})

	entries, err := fs.ReadDir(assets, Dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read themes: %w", err)
	}
	var found []Theme
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".css" {
			continue
		}
		data, err := fs.ReadFile(assets, path.Join(Dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read theme %s: %w", e.Name(), err)
		}
		t, err := parse(strings.TrimSuffix(e.Name(), ".css"), string(data))
		if err != nil {
			return nil, fmt.Errorf("theme %s: %w", e.Name(), err)
		}
		found = append(found, t)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	for _, t := range found {
		if _, dup := r.byID[t.ID]; dup {
			return nil, fmt.Errorf("theme %s: the id is taken", t.ID)
		}
		r.add(t)
	}
	return r, nil
}

func (r *Registry) add(t Theme) {
	r.themes = append(r.themes, t)
	r.byID[t.ID] = t
}

func parse(id, css string) (Theme, error) {
	if !ValidID(id) {
		return Theme{}, fmt.Errorf("file name must be lower-case letters, digits and hyphens")
	}
	header := headerRe.FindStringSubmatch(css)
	if header == nil {
		return Theme{}, fmt.Errorf("must start with a /* theme: ... */ comment")
	}
	t := Theme{ID: id, File: "/static/css/themes/" + id + ".css"}
	for _, f := range fieldRe.FindAllStringSubmatch(header[1], -1) {
		switch f[1] {
		case "theme":
			t.Name = f[2]
		case "modes":
			for _, m := range strings.Fields(f[2]) {
				switch m {
				case "light":
					t.Light = true
				case "dark":
					t.Dark = true
				default:
					return Theme{}, fmt.Errorf("unknown mode %q", m)
				}
			}
		}
	}
	if t.Name == "" {
		return Theme{}, fmt.Errorf("header has no \"theme:\" name")
	}
	if !t.Light && !t.Dark {
		return Theme{}, fmt.Errorf("header has no \"modes:\" (light, dark or both)")
	}
	// A declared mode with no rules would show Classic under the theme's
	// name.
	if t.Light && !hasRule(css, id, false) {
		return Theme{}, fmt.Errorf("declares light but has no [data-theme=%q] rule", id)
	}
	if t.Dark && !hasRule(css, id, true) {
		return Theme{}, fmt.Errorf("declares dark but has no [data-theme=%q][data-bs-theme=\"dark\"] rule", id)
	}
	return t, nil
}

func hasRule(css, id string, dark bool) bool {
	sel := `\[data-theme="` + regexp.QuoteMeta(id) + `"\]`
	if dark {
		return regexp.MustCompile(sel + `\[data-bs-theme="dark"\]`).MatchString(css)
	}
	return regexp.MustCompile(sel + `\s*[,{]`).MatchString(css)
}

// All returns the themes, Classic first.
func (r *Registry) All() []Theme {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := append([]Theme(nil), r.themes...)
	for _, u := range r.user {
		all = append(all, u.theme)
	}
	return all
}

// Resolve returns the theme with this id, or Classic when there is none: a
// saved setting can outlive the theme it names.
func (r *Registry) Resolve(id string) Theme {
	if t, ok := r.byID[id]; ok {
		return t
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.user {
		if u.theme.ID == id {
			return u.theme
		}
	}
	return r.byID[DefaultID]
}
