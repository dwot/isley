#!/bin/bash
# build.sh: the picked renders (src/<theme>.png, and cut/<theme>.png for the two cut out with BiRefNet)
# -> web/static/img/logos/<theme>.png, 256 px, transparent, pngquant'd. Needs ImageMagick 7 and pngquant.
set -e
D=$(cd "$(dirname "$0")" && pwd); OUT=$D/../../web/static/img/logos; T=$(mktemp -d)
sq() { magick "$1" -trim +repage -background none -gravity center -extent "%[fx:max(w,h)*1.06]x%[fx:max(w,h)*1.06]" -resize 256x256 "$2"; }

# Classic, Sherbet: the BiRefNet cutout as it is.
sq "$D/cut/classic.png" $T/classic.png
sq "$D/cut/sherbet.png" $T/sherbet.png

# Walnut: the copper drawing lifted off the render by brightness, set on the dotfiles icon tile.
magick "$D/src/walnut.png" -colorspace gray -level 36%,52% $T/wa.png
magick "$D/src/walnut.png" $T/wa.png -alpha off -compose CopyOpacity -composite -trim +repage $T/wg.png
magick -background none -density 384 "$D/walnut-tile.svg" -resize 512x512 $T/wt.png
magick $T/wt.png \( $T/wg.png -resize 380x380 \) -gravity center -composite -resize 256x256 $T/walnut.png

# Clarity: one colour. Alpha from darkness, filled with the theme's blue (light) and light blue (dark).
magick "$D/src/clarity.png" -colorspace gray -negate -level 8%,70% $T/ca.png
magick -size 1024x1024 xc:'#0047b3' $T/ca.png -alpha off -compose CopyOpacity -composite $T/c1.png; sq $T/c1.png $T/clarity.png
magick -size 1024x1024 xc:'#7ab8ff' $T/ca.png -alpha off -compose CopyOpacity -composite $T/c2.png; sq $T/c2.png $T/clarity-dark.png

# Blacklight: keep the glow. Alpha from the distance to the render's background colour.
magick "$D/src/blacklight.png" \( +clone -fill 'srgb(26,4,40)' -colorize 100 \) -compose difference -composite -colorspace gray -level 14%,42% $T/ba.png
magick "$D/src/blacklight.png" $T/ba.png -alpha off -compose CopyOpacity -composite $T/b.png
G=$(magick $T/b.png \( +clone -alpha extract -threshold 8% \) -compose CopyOpacity -composite -trim -format '%wx%h%O' info:)
magick $T/b.png -crop $G +repage -background none -gravity center -extent "%[fx:max(w,h)*1.08]x%[fx:max(w,h)*1.08]" -resize 256x256 $T/blacklight.png

mkdir -p "$OUT"
for f in classic walnut clarity clarity-dark blacklight sherbet; do pngquant --quality 70-95 --speed 1 --force --output "$OUT/$f.png" $T/$f.png; done
rm -rf $T
