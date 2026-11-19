# Theme packages

Isley ships with five themes. An instance can have more: an admin adds a
theme package in **Settings → Appearance → Add a theme**, and it shows up
beside the built-in themes, in the default-theme picker and in the theme
menu in the header.

There are two ways to make one, and both end in the same package.

**In the editor.** Every theme in Settings → Appearance has a
**Customize** button (**Edit**, for a theme you installed), and **Create a
theme** starts from Classic. The editor lists every token with a colour
picker, a slider or a text field, shows the result in a preview as you
go, and says which colour pairs are hard to read. Customizing a built-in
theme makes a new theme and leaves the original alone; editing an
installed theme changes it in place. A value shown in grey is Classic's
and is not part of the theme until you change it.

**By hand.** Every theme also has a **Download** button. A built-in theme
downloads as a copy under a new id (`walnut-custom`), so it can be changed
in a text editor and added back without clashing with the original. The
rest of this page is the format.

## What a package is

A zip file with:

| File | |
| --- | --- |
| `theme.json` | Required. The theme's name, its modes and its values. |
| `logo.png` | Optional. The mark in the header, shown 60px square. |
| `logo-dark.png` | Optional. The mark in dark mode, when it differs. |

Logos may be PNG, JPEG or WebP (`logo.jpg`, `logo.webp`), up to 512 KB and
2048 pixels a side. SVG is not accepted. A theme with no logo uses
Classic's. The files may sit in one folder inside the zip, and anything
else in the zip is ignored. A `theme.json` on its own is a package too.

## theme.json

```json
{
  "isley_theme": 1,
  "id": "harbour",
  "name": "Harbour",
  "author": "you",
  "description": "Blues and greys.",
  "modes": ["light", "dark"],
  "tokens": {
    "--radius": "4px",
    "--font-heading": "Georgia, serif"
  },
  "light": {
    "--color-bg": "#eef3f7",
    "--color-primary": "#0b5cad"
  },
  "dark": {
    "--color-bg": "#0a1420",
    "--color-card": "#12202f",
    "--color-primary": "#2f8fe0",
    "--bg-image": "linear-gradient(180deg, #0a1420 0%, #0d2136 100%)"
  }
}
```

| Field | |
| --- | --- |
| `isley_theme` | The format version. `1`. |
| `name` | Shown in the pickers. Up to 40 characters. |
| `id` | Lower-case letters, digits and hyphens, up to 40. Left out, it is made from the name. It cannot be the id of a built-in theme. Adding a package whose id is already installed replaces that theme. |
| `author`, `description` | Optional. Kept with the package. |
| `modes` | `["light"]`, `["dark"]` or both. A theme with one mode is shown in that mode whatever the visitor's light/dark choice. |
| `tokens` | Values for every mode the theme has. |
| `light`, `dark` | Values for one mode, over `tokens`. |

A token the package leaves out keeps Classic's value for that mode, so a
theme can be six colours or all of them.

## Tokens

**Settings → Appearance → "See every token a theme can set"** opens the
reference: every token, what it paints, what it accepts, and its value in
the theme on screen. Switch theme or mode in the header and the page
follows, so it is also the place to look a new theme over.

The tokens are the ones in
[`web/static/css/tokens.css`](../web/static/css/tokens.css). A package may
set any of them except `--logo` (that is the logo file) and the
`--radius-*` sizes (they follow `--radius`).

A package holds values, not styles: Isley checks each value and writes the
stylesheet itself. What each kind of token accepts:

| Tokens | Accepts |
| --- | --- |
| Colours: `--color-*`, `--stage-*`, `--chart-*` | `#rgb`, `#rrggbb`, `#rrggbbaa`, `rgb()`, `rgba()`, `hsl()`, `hsla()`, `transparent` |
| `--bg-image` | `none`, or one or more gradients (`linear-gradient()`, `radial-gradient()`, `conic-gradient()` and their `repeating-` forms). No `url()`. |
| `--shadow-sm`, `--shadow-md`, `--shadow-hover`, `--color-dropdown-menu-shadow` | `none`, or box shadows: lengths, a colour, `inset` |
| `--font-body`, `--font-heading`, `--font-mono` | A list of font names. A package cannot carry font files; name fonts the visitor may have and end with a generic family (`sans-serif`). |
| `--radius` | `0px` to `40px`, or up to `2.5rem` |
| `--font-size` | `0.75rem` to `1.5rem`, or `12px` to `24px` |
| `--shade` | `0%` to `100%` |

`var()`, `calc()`, `color-mix()` and colour names are not accepted. A value
that does not fit is refused, and the message names the token. A token
this version of Isley does not know is ignored with a warning, so a
package written for a newer Isley still loads.

Two things worth knowing when picking colours:

- Roles come as a fill and a text colour. `--color-primary` is a button's
  background, with `--color-on-primary` as the text on it;
  `--color-accent` is the same role as text or an icon on a card. On a
  dark theme the text flavour usually needs to be lighter than the fill.

## Contrast

When a theme is added, Isley measures the text colours it sets against the
surfaces they are read on: body, muted, link and status text on the card
and the page, the text on each button and badge fill, and
`--color-on-stage` on every stage colour. A pair under 4.5:1 is reported,
never refused: the theme installs, and the pairs are listed under it in
Settings → Appearance for as long as it is installed.

Only pairs the package has a hand in are reported, where it sets the text,
the background or both. The Contrast table at the foot of the reference
page shows every pair for the theme on screen, built-in themes included.
A background that is partly transparent is not measured.

## Where installed themes are kept

Under the uploads folder, one directory each:
`uploads/themes/<id>/theme.json` and its logos. They are included in a
backup made with **include images**, and come back with a restore of one.
Removing a theme deletes its directory; an instance that had it as the
default goes back to Classic.

Adding, removing and downloading themes needs a login (or an API key),
like the rest of Settings:

```
POST   /settings/themes              form field "theme": the package
POST   /settings/themes/check        JSON body: a theme.json; says whether it
                                     would be accepted, and its contrast
POST   /settings/themes/save         the editor's save: form fields "manifest",
                                     "from", "new", optional "logo", "logo_dark"
GET    /settings/themes/<id>/export  the theme as a package
DELETE /settings/themes/<id>
```
