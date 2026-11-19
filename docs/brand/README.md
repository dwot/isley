# Isley logos

Each theme has its own mark, drawn from `--logo` in the theme's stylesheet and served from
`web/static/img/logos/`. They replaced the stock leaf Isley had carried since the start (2026-11-18).

| Theme | Mark | Source |
|---|---|---|
| Classic | a fan leaf, blue to green | `classic_leaf` seed 101, cut out with BiRefNet |
| Walnut | a leaf growing from an open notebook, copper line on the dotfiles icon tile | `walnut_journal` seed 202; the drawing lifted by brightness and set on `walnut-tile.svg` |
| Clarity | a leaf in a water drop, one colour (`clarity.png` blue for light, `clarity-dark.png` light blue for dark) | `clarity_drop` seed 202, recoloured |
| Blacklight | a neon leaf outline with its glow | `blacklight_leaf` seed 202 |
| Sherbet | a sprout sticker | `sherbet_sprout` seed 101, cut out with BiRefNet |

## How they were made

`run.sh` renders every idea in `briefs.txt` (`id|subject`) in every style in `styles.txt` (`theme|style`) with
**Z-Image Turbo** on ComfyUI (`image_zimage-turbo_t2i`, enhance off, no LoRA, 1 MP, seeds 101 and 202), through the
headless runner in the ComfyUI workflows repo. Four ideas (leaf, sprout, journal, drop) by five styles by two seeds
made the 40 candidates of round 1; the five above were picked from those.

`build.sh` turns the picked renders into the shipped files. It expects them at `src/<theme>.png`, and the BiRefNet
cutouts (`image_util_cutout`) of Classic and Sherbet at `cut/<theme>.png`. Renders are not kept in the repo: the
seeds above reproduce them.

Licence: Z-Image Turbo output is Apache-2.0. The Walnut tile is the icon tile from dwot/dotfiles.

## Favicon

`web/static/img/favicon.svg` is drawn by hand, no model: a green five-leaflet leaf on a dark tile. It belongs to no
theme, so a tab looks the same whichever one the browser is showing. `favicon.ico` (16, 32 and 48 px, for browsers
that ask for it) is rendered from the SVG:

    magick -background none -density 1200 favicon.svg -resize 256x256 fav.png
    magick fav.png -define icon:auto-resize=48,32,16 favicon.ico
