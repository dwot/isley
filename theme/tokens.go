package theme

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A package's values are written into a stylesheet every visitor loads,
// so each kind of token (see reference.go) accepts a small grammar and
// nothing else: no url(), no var(), and none of the characters that end a
// declaration.

// KnownToken reports whether a theme package may set this token.
func KnownToken(name string) bool {
	_, ok := tokenKinds[name]
	return ok
}

const maxValueLen = 600

var (
	colorFuncs = map[string]bool{"rgb": true, "rgba": true, "hsl": true, "hsla": true}
	imageFuncs = map[string]bool{
		"linear-gradient": true, "radial-gradient": true, "conic-gradient": true,
		"repeating-linear-gradient": true, "repeating-radial-gradient": true,
		"repeating-conic-gradient": true,
	}
	imageWords = map[string]bool{
		"to": true, "at": true, "from": true, "top": true, "bottom": true,
		"left": true, "right": true, "center": true, "circle": true,
		"ellipse": true, "closest-side": true, "closest-corner": true,
		"farthest-side": true, "farthest-corner": true, "transparent": true,
	}
	shadowWords = map[string]bool{"inset": true, "transparent": true}
	noWords     = map[string]bool{}

	hexRe     = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	numberRe  = regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:px|rem|em|%|deg|turn|rad)?`)
	identRe   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*`)
	hexTokRe  = regexp.MustCompile(`^#[0-9a-fA-F]+`)
	lengthRe  = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(px|rem|%)$`)
	quotedRe  = regexp.MustCompile(`^(?:"[A-Za-z0-9 ._-]+"|'[A-Za-z0-9 ._-]+')$`)
	familyRe  = regexp.MustCompile(`^-?[A-Za-z][A-Za-z0-9_-]*(?: [A-Za-z0-9_-]+)*$`)
	spacesRe  = regexp.MustCompile(`\s+`)
	colorCall = regexp.MustCompile(`^(rgb|rgba|hsl|hsla)\([0-9a-z.,%/ +-]+\)$`)
)

// checkValue returns value in the form it is written to the stylesheet,
// or says why it is not a value for this token.
func checkValue(name, value string) (string, error) {
	k, ok := tokenKinds[name]
	if !ok {
		return "", fmt.Errorf("unknown token")
	}
	v := strings.TrimSpace(spacesRe.ReplaceAllString(value, " "))
	if v == "" {
		return "", fmt.Errorf("is empty")
	}
	if len(v) > maxValueLen {
		return "", fmt.Errorf("is longer than %d characters", maxValueLen)
	}
	var err error
	switch k {
	case KindColor:
		err = checkColor(v)
	case KindShadow:
		if v != "none" {
			err = checkTokens(v, colorFuncs, shadowWords)
		}
	case KindImage:
		if v != "none" {
			err = checkImage(v)
		}
	case KindFont:
		err = checkFont(v)
	case KindRadius:
		err = checkLength(v, map[string][2]float64{"px": {0, 40}, "rem": {0, 2.5}})
	case KindFontSize:
		err = checkLength(v, map[string][2]float64{"px": {12, 24}, "rem": {0.75, 1.5}})
	case KindPercent:
		err = checkLength(v, map[string][2]float64{"%": {0, 100}})
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func checkColor(v string) error {
	if v == "transparent" || hexRe.MatchString(v) || colorCall.MatchString(v) {
		return nil
	}
	return fmt.Errorf("%q is not a colour: use #rrggbb, rgb(), hsl() or transparent", v)
}

func checkImage(v string) error {
	funcs := map[string]bool{}
	for f := range imageFuncs {
		funcs[f] = true
	}
	for f := range colorFuncs {
		funcs[f] = true
	}
	if err := checkTokens(v, funcs, imageWords); err != nil {
		return err
	}
	// Only gradients, not a bare colour or number.
	if name := identRe.FindString(v); !imageFuncs[name] {
		return fmt.Errorf("must be \"none\" or a gradient")
	}
	return nil
}

// checkTokens reads v as numbers, hex colours, commas, slashes, brackets
// and names, where a name before "(" must be in funcs and any other must
// be in words.
func checkTokens(v string, funcs, words map[string]bool) error {
	if words == nil {
		words = noWords
	}
	depth := 0
	for rest := v; rest != ""; {
		switch c := rest[0]; {
		case c == ' ' || c == ',' || c == '/':
			rest = rest[1:]
		case c == '(':
			return fmt.Errorf("has a \"(\" with no function name before it")
		case c == ')':
			if depth--; depth < 0 {
				return fmt.Errorf("has an unmatched \")\"")
			}
			rest = rest[1:]
		case c == '#':
			tok := hexTokRe.FindString(rest)
			if !hexRe.MatchString(tok) {
				return fmt.Errorf("%q is not a hex colour", tok)
			}
			rest = rest[len(tok):]
		default:
			if tok := numberRe.FindString(rest); tok != "" {
				rest = rest[len(tok):]
				continue
			}
			tok := identRe.FindString(rest)
			if tok == "" {
				return fmt.Errorf("has a character that is not allowed: %q", string(c))
			}
			rest = rest[len(tok):]
			if strings.HasPrefix(rest, "(") {
				if !funcs[tok] {
					return fmt.Errorf("%s() is not allowed", tok)
				}
				depth++
				rest = rest[1:]
			} else if !words[tok] {
				return fmt.Errorf("%q is not allowed here", tok)
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("has an unclosed \"(\"")
	}
	return nil
}

// checkFont accepts a font-family list: names, quoted or not, with commas
// between them.
func checkFont(v string) error {
	for _, family := range strings.Split(v, ",") {
		family = strings.TrimSpace(family)
		if !quotedRe.MatchString(family) && !familyRe.MatchString(family) {
			return fmt.Errorf("%q is not a font name", family)
		}
	}
	return nil
}

func checkLength(v string, ranges map[string][2]float64) error {
	units := make([]string, 0, len(ranges))
	for u := range ranges {
		units = append(units, u)
	}
	sort.Strings(units)
	m := lengthRe.FindStringSubmatch(v)
	if m == nil {
		return fmt.Errorf("%q is not a number with a unit (%s)", v, strings.Join(units, ", "))
	}
	r, ok := ranges[m[2]]
	if !ok {
		return fmt.Errorf("%q must be in %s", v, strings.Join(units, " or "))
	}
	n, _ := strconv.ParseFloat(m[1], 64)
	if n < r[0] || n > r[1] {
		return fmt.Errorf("%q is outside %g%s to %g%s", v, r[0], m[2], r[1], m[2])
	}
	return nil
}
