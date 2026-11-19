package theme

// The tokens a theme package may set: what each is for and what kind of
// value it takes. This list is the allow-list (tokens.go checks values
// against it) and the reference page in Settings is drawn from it. The
// names are the ones in web/static/css/tokens.css; a test keeps the two
// together.

// Token is one entry in the reference.
type Token struct {
	Name string
	Kind Kind
	// Desc says what the token paints.
	Desc string
}

// Group is a run of tokens under one heading, in the order of tokens.css.
type Group struct {
	Title  string
	Note   string
	Tokens []Token
}

// Kind is the sort of value a token takes.
type Kind int

const (
	KindColor Kind = iota
	KindShadow
	KindImage
	KindFont
	KindRadius
	KindFontSize
	KindPercent
)

// String is a short name for the kind, used as a class on the reference
// page: "color", "shadow", ...
func (k Kind) String() string {
	return [...]string{"color", "shadow", "image", "font", "radius", "font-size", "percent"}[k]
}

// Accepts says what a value of this kind may be.
func (k Kind) Accepts() string {
	switch k {
	case KindColor:
		return "#rgb, #rrggbb, #rrggbbaa, rgb(), hsl() or transparent"
	case KindShadow:
		return "none, or box shadows: lengths, a colour, inset"
	case KindImage:
		return "none, or gradients: linear-, radial-, conic-gradient()"
	case KindFont:
		return "a list of font names, ending in a generic family"
	case KindRadius:
		return "0px to 40px, or up to 2.5rem"
	case KindFontSize:
		return "0.75rem to 1.5rem, or 12px to 24px"
	default:
		return "0% to 100%"
	}
}

func colors(pairs ...string) []Token {
	out := make([]Token, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Token{Name: pairs[i], Kind: KindColor, Desc: pairs[i+1]})
	}
	return out
}

var groups = []Group{
	{
		Title: "Surfaces and text",
		Tokens: append(colors(
			"--color-bg", "The page behind everything.",
			"--color-card", "Cards, dialogs, menus and tables.",
			"--color-card-border", "The edge of a card.",
			"--color-table-header", "A raised strip on a card: table heads and card headers.",
			"--color-row-hover", "A table row or list item under the pointer.",
			"--color-border", "Dividers and the edges of controls.",
			"--color-fg", "Body text.",
			"--color-muted", "Secondary text: hints, captions, timestamps.",
			"--color-input-bg", "The inside of a text field or select.",
			"--color-input-border", "The edge of a text field or select.",
			"--color-spinner-overlay", "The veil over the page while it waits. Usually partly transparent.",
		), Token{"--bg-image", KindImage, "Drawn over --color-bg on the page itself: a gradient or a glow."}),
	},
	{
		Title: "Roles",
		Note:  "Each role is a fill, with the text that sits on it, and the same role as text or an icon on a card. On a dark theme the text flavour usually needs to be lighter than the fill.",
		Tokens: colors(
			"--color-primary", "Fill of the main button, the active tab, a checked switch.",
			"--color-on-primary", "Text on --color-primary.",
			"--color-accent", "Primary as text, an icon or a thin border on a card.",
			"--color-accent-hover", "--color-accent under the pointer.",
			"--color-link", "Links.",
			"--color-link-hover", "A link under the pointer.",
			"--color-secondary", "Fill of a secondary button or badge.",
			"--color-on-secondary", "Text on --color-secondary.",
			"--color-success", "Fill for success: buttons, badges, toasts.",
			"--color-on-success", "Text on --color-success.",
			"--color-success-text", "Success as text or an icon on a card.",
			"--color-warning", "Fill for warnings.",
			"--color-on-warning", "Text on --color-warning.",
			"--color-warning-text", "Warning as text or an icon on a card: a plant overdue for water.",
			"--color-danger", "Fill for destructive actions and errors.",
			"--color-on-danger", "Text on --color-danger.",
			"--color-danger-text", "Danger as text or an icon on a card.",
			"--color-info", "Fill for information.",
			"--color-on-info", "Text on --color-info.",
			"--color-info-text", "Information as text or an icon on a card.",
		),
	},
	{
		Title: "Grow stages and strain types",
		Note:  "Stage colours are badge fills; --color-on-stage is the text on every one of them, so pick stage colours that all read against it.",
		Tokens: colors(
			"--stage-germ", "Germination.",
			"--stage-seedling", "Seedling.",
			"--stage-veg", "Vegetative.",
			"--stage-flower", "Flowering.",
			"--stage-dry", "Drying.",
			"--stage-cure", "Curing.",
			"--stage-harvest", "Harvested.",
			"--stage-dead", "Dead.",
			"--color-on-stage", "Text on any stage colour.",
			"--color-auto", "Autoflower strains.",
			"--color-photo", "Photoperiod strains.",
			"--color-fem", "Feminised seed.",
			"--color-regular", "Regular seed.",
			"--color-indica", "Indica, as a bar or a fill.",
			"--color-indica-text", "Indica as text on a card.",
			"--color-sativa", "Sativa, as a bar or a fill.",
			"--color-sativa-text", "Sativa as text on a card.",
			"--color-star", "A lit star in a smoke-report rating.",
			"--color-star-preview", "A star under the pointer, before it is chosen. Weaker than --color-star.",
			"--color-star-empty", "An unlit star.",
		),
	},
	{
		Title: "Shape",
		Tokens: []Token{
			{"--radius", KindRadius, "The corner of a control. Cards, pills and small controls scale from it."},
			{"--shadow-sm", KindShadow, "The resting shadow of a card."},
			{"--shadow-md", KindShadow, "The shadow of something lifted: a dialog, a dragged item."},
		},
	},
	{
		Title: "Type",
		Note:  "A package cannot carry font files. Name fonts a visitor may have installed, then fall back.",
		Tokens: []Token{
			{"--font-size", KindFontSize, "The size of body text. Everything else scales from it."},
			{"--font-body", KindFont, "Body text."},
			{"--font-heading", KindFont, "Headings. Follows --font-body unless set."},
			{"--font-mono", KindFont, "Code, logs and API keys."},
		},
	},
	{
		Title: "Charts",
		Note:  "Series colours in order. A chart with more than six series starts again from the first.",
		Tokens: colors(
			"--chart-1", "First series.",
			"--chart-2", "Second series.",
			"--chart-3", "Third series.",
			"--chart-4", "Fourth series.",
			"--chart-5", "Fifth series.",
			"--chart-6", "Sixth series.",
			"--chart-text", "Axis labels and legends. Follows --color-muted unless set.",
			"--chart-grid", "Grid lines. A faint tint of --color-fg unless set.",
		),
	},
	{
		Title: "Single components",
		Note:  "These follow the tokens above. Set one only where that component should differ.",
		Tokens: append([]Token{
			{"--shade", KindPercent, "How far a hover or a gradient end darkens its colour."},
			{"--shadow-hover", KindShadow, "The glow of a card under the pointer."},
			{"--color-dropdown-menu-shadow", KindShadow, "The shadow under an open menu."},
		}, colors(
			"--color-primary-hover", "The main button under the pointer.",
			"--color-focus-ring", "The ring around the focused control. Usually partly transparent.",
			"--color-table-bg", "The body of a table.",
			"--color-table-th", "A table's header row.",
			"--color-dropdown-bg", "An open menu.",
			"--color-dropdown-fg", "Text in a menu.",
			"--color-dropdown-border", "The edge of a menu.",
			"--color-input-fg", "Text typed into a field.",
			"--color-input-focus-border", "The edge of the focused field.",
			"--color-btn-secondary-bg", "The quiet button: Cancel, Close.",
			"--color-btn-secondary-fg", "Text on the quiet button.",
			"--color-btn-secondary-border", "The edge of the quiet button.",
			"--color-btn-secondary-hover-bg", "The quiet button under the pointer.",
			"--color-view-btn-border", "The edge of the grid/list view switch.",
			"--color-view-btn-hover-bg", "The view switch under the pointer.",
			"--color-view-btn-hover-fg", "Its icon under the pointer.",
			"--color-view-btn-active-bg", "The chosen view.",
			"--color-view-btn-active-fg", "The chosen view's icon.",
			"--color-view-btn-active-border", "The chosen view's edge.",
			"--color-thumb-hover", "A photo thumbnail's frame under the pointer.",
			"--color-success-bg", "A soft success tint behind a row or a note.",
		)...),
	},
}

// Reference returns the tokens a package may set, grouped and in order.
func Reference() []Group { return groups }

var tokenKinds = func() map[string]Kind {
	m := map[string]Kind{}
	for _, g := range groups {
		for _, t := range g.Tokens {
			m[t.Name] = t.Kind
		}
	}
	return m
}()
