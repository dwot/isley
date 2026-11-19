package theme

import (
	"fmt"
	"io/fs"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Contrast: can the text a theme sets be read on what it sits on?
//
// A package is measured when it is installed and each time the themes are
// loaded. A pair under the mark is reported, never refused: the mark is
// WCAG's 4.5:1 for body text, and a theme may have its reasons.

// MinContrast is the ratio under which a pair is reported.
const MinContrast = 4.5

// Pair is a text colour and the surface it is read on.
type Pair struct {
	Text       string
	Background string
	// What is where the pair shows up, for the reference page.
	What string
}

// ContrastPairs are the pairs measured.
var ContrastPairs = []Pair{
	{"--color-fg", "--color-card", "Body text on a card"},
	{"--color-fg", "--color-bg", "Body text on the page"},
	{"--color-fg", "--color-table-header", "Text in a table head"},
	{"--color-fg", "--color-input-bg", "Text in a field"},
	{"--color-muted", "--color-card", "Secondary text on a card"},
	{"--color-muted", "--color-bg", "Secondary text on the page"},
	{"--color-accent", "--color-card", "Accent text and icons on a card"},
	{"--color-link", "--color-card", "Links on a card"},
	{"--color-link", "--color-bg", "Links on the page"},
	{"--color-success-text", "--color-card", "Success text on a card"},
	{"--color-warning-text", "--color-card", "Warning text on a card"},
	{"--color-danger-text", "--color-card", "Danger text on a card"},
	{"--color-info-text", "--color-card", "Information text on a card"},
	{"--color-indica-text", "--color-card", "Indica text on a card"},
	{"--color-sativa-text", "--color-card", "Sativa text on a card"},
	{"--color-on-primary", "--color-primary", "Main button"},
	{"--color-on-secondary", "--color-secondary", "Secondary button and badge"},
	{"--color-on-success", "--color-success", "Success button and badge"},
	{"--color-on-warning", "--color-warning", "Warning button and badge"},
	{"--color-on-danger", "--color-danger", "Danger button and badge"},
	{"--color-on-info", "--color-info", "Information button and badge"},
	{"--color-on-stage", "--stage-germ", "Germination badge"},
	{"--color-on-stage", "--stage-seedling", "Seedling badge"},
	{"--color-on-stage", "--stage-veg", "Vegetative badge"},
	{"--color-on-stage", "--stage-flower", "Flowering badge"},
	{"--color-on-stage", "--stage-dry", "Drying badge"},
	{"--color-on-stage", "--stage-cure", "Curing badge"},
	{"--color-on-stage", "--stage-harvest", "Harvested badge"},
	{"--color-on-stage", "--stage-dead", "Dead badge"},
}

// ContrastIssue is a pair a theme leaves hard to read in one mode.
type ContrastIssue struct {
	Mode       string  `json:"mode"`
	Text       string  `json:"text"`
	Background string  `json:"background"`
	Ratio      float64 `json:"ratio"`
}

// RatioText is the ratio as it is usually written: "3.2". It is cut, not
// rounded, so a pair just under the mark does not read as on it.
func (i ContrastIssue) RatioText() string {
	return strconv.FormatFloat(math.Floor(i.Ratio*10)/10, 'f', 1, 64)
}

type rgba struct{ r, g, b, a float64 } // 0..1

var (
	fnRe  = regexp.MustCompile(`^(rgb|rgba|hsl|hsla)\((.*)\)$`)
	sepRe = regexp.MustCompile(`[\s,/]+`)
)

// parseColor reads the colours checkValue accepts. ok is false for
// anything it cannot place (an angle in turns, say); such a pair is left
// unmeasured.
func parseColor(v string) (c rgba, ok bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "transparent" {
		return rgba{}, true
	}
	if strings.HasPrefix(v, "#") {
		h := v[1:]
		if len(h) == 3 || len(h) == 4 {
			var long strings.Builder
			for _, ch := range h {
				long.WriteRune(ch)
				long.WriteRune(ch)
			}
			h = long.String()
		}
		if len(h) != 6 && len(h) != 8 {
			return rgba{}, false
		}
		n, err := strconv.ParseUint(h, 16, 64)
		if err != nil {
			return rgba{}, false
		}
		if len(h) == 6 {
			n = n<<8 | 0xff
		}
		return rgba{float64(n>>24&0xff) / 255, float64(n>>16&0xff) / 255, float64(n>>8&0xff) / 255, float64(n&0xff) / 255}, true
	}
	m := fnRe.FindStringSubmatch(v)
	if m == nil {
		return rgba{}, false
	}
	parts := sepRe.Split(strings.TrimSpace(m[2]), -1)
	if len(parts) != 3 && len(parts) != 4 {
		return rgba{}, false
	}
	// num reads "128", "50%" or "120deg"; whole is what 100% stands for.
	num := func(s string, whole float64) (float64, bool) {
		pct := strings.HasSuffix(s, "%")
		s = strings.TrimSuffix(strings.TrimSuffix(s, "%"), "deg")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		if pct {
			f = f / 100 * whole
		}
		return f, true
	}
	clamp := func(f float64) float64 { return math.Max(0, math.Min(1, f)) }
	c.a = 1
	if len(parts) == 4 {
		a, ok := num(parts[3], 1)
		if !ok {
			return rgba{}, false
		}
		c.a = clamp(a)
	}
	if strings.HasPrefix(m[1], "rgb") {
		var ch [3]float64
		for i := range ch {
			f, ok := num(parts[i], 255)
			if !ok {
				return rgba{}, false
			}
			ch[i] = clamp(f / 255)
		}
		c.r, c.g, c.b = ch[0], ch[1], ch[2]
		return c, true
	}
	h, ok1 := num(parts[0], 360)
	s, ok2 := num(parts[1], 1)
	l, ok3 := num(parts[2], 1)
	if !ok1 || !ok2 || !ok3 || !strings.HasSuffix(parts[1], "%") || !strings.HasSuffix(parts[2], "%") {
		return rgba{}, false
	}
	s, l = clamp(s), clamp(l)
	h = math.Mod(math.Mod(h, 360)+360, 360) / 30
	f := func(n float64) float64 {
		k := math.Mod(n+h, 12)
		return l - s*math.Min(l, 1-l)*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
	}
	c.r, c.g, c.b = f(0), f(8), f(4)
	return c, true
}

// over lays c on an opaque colour.
func (c rgba) over(under rgba) rgba {
	return rgba{c.r*c.a + under.r*(1-c.a), c.g*c.a + under.g*(1-c.a), c.b*c.a + under.b*(1-c.a), 1}
}

func (c rgba) luminance() float64 {
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

// contrastRatio is WCAG's ratio between a text colour and an opaque
// background, from 1 to 21. Text that is partly transparent is measured
// as it lands on the background.
func contrastRatio(text, background rgba) float64 {
	a, b := text.over(background).luminance(), background.luminance()
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

// classicValues reads Classic's plain token values out of tokens.css, for
// the tokens a package leaves alone. Tokens worked out from others are
// left out; none of the measured pairs is one.
func classicValues(assets fs.FS) (light, dark map[string]string) {
	light, dark = map[string]string{}, map[string]string{}
	data, err := fs.ReadFile(assets, "web/static/css/tokens.css")
	if err != nil {
		return light, dark
	}
	for _, rule := range ruleRe.FindAllStringSubmatch(commentRe.ReplaceAllString(string(data), ""), -1) {
		var target map[string]string
		for _, sel := range strings.Split(rule[1], ",") {
			switch strings.TrimSpace(sel) {
			case ":root":
				target = light
			case `[data-bs-theme="dark"]`:
				target = dark
			}
		}
		if target == nil {
			continue
		}
		for _, decl := range strings.Split(rule[2], ";") {
			if d := declRe.FindStringSubmatch(spacesRe.ReplaceAllString(decl, " ")); d != nil {
				target[d[1]] = d[2]
			}
		}
	}
	return light, dark
}

// contrastIssues measures the manifest's modes. Only a pair the package
// has a hand in is reported: one where it sets the text, the background
// or both. What Classic does with its own colours is not the package's to
// answer for.
func (m Manifest) contrastIssues(classicLight, classicDark map[string]string) []ContrastIssue {
	var issues []ContrastIssue
	for _, mode := range m.Modes {
		own := m.Light
		if mode == "dark" {
			own = m.Dark
		}
		value := func(name string) (string, bool) {
			if v, ok := own[name]; ok {
				return v, true
			}
			if v, ok := m.Tokens[name]; ok {
				return v, true
			}
			if mode == "dark" {
				if v, ok := classicDark[name]; ok {
					return v, false
				}
			}
			return classicLight[name], false
		}
		for _, p := range ContrastPairs {
			tv, tOwn := value(p.Text)
			bv, bOwn := value(p.Background)
			if !tOwn && !bOwn {
				continue
			}
			text, ok1 := parseColor(tv)
			bg, ok2 := parseColor(bv)
			if !ok1 || !ok2 || bg.a < 1 {
				continue
			}
			if ratio := contrastRatio(text, bg); ratio < MinContrast {
				issues = append(issues, ContrastIssue{Mode: mode, Text: p.Text, Background: p.Background, Ratio: math.Round(ratio*100) / 100})
			}
		}
	}
	return issues
}

// ContrastSummary is one line for a list of issues: "3 colour pairs are
// below 4.5:1".
func ContrastSummary(issues []ContrastIssue) string {
	noun := "pairs are"
	if len(issues) == 1 {
		noun = "pair is"
	}
	return fmt.Sprintf("%d colour %s below %g:1", len(issues), noun, MinContrast)
}
