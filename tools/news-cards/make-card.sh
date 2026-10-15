#!/bin/sh
# make-card.sh PHOTO LABEL OUT.jpg — console photo (white or transparent background) on the
# OpenPak indigo gradient, 1280x720, with a label. Used for channels/screenshots/<id>.jpg.
# Needs ImageMagick 7. Photos: Wikimedia Commons (see scratch/news-screens/*/source.txt).
set -e
photo=$1; label=$2; out=$3; t=${TMPDIR:-.}/card.$$
# photos with a real alpha channel are used as they are; white backdrops are flood-filled away
if [ "$(magick "$photo" -format '%[opaque]' info:)" = "False" ]; then pre=""; else
  pre="-background white -alpha remove -alpha on -fuzz ${FUZZ:-7}% -fill none -draw 'alpha 0,0 floodfill' -draw 'alpha %[fx:w-1],0 floodfill' -draw 'alpha 0,%[fx:h-1] floodfill' -draw 'alpha %[fx:w-1],%[fx:h-1] floodfill'"; fi
eval magick '"$photo"' $pre -trim +repage -resize 900x440 '"$t.png"'
magick -size 1280x720 radial-gradient:'#4f46e5-#0b0b24' \
  \( "$t.png" \( +clone -background black -shadow 60x14+0+14 \) +swap -background none -layers merge +repage \) \
  -gravity center -geometry +0-30 -composite \
  -gravity southwest -font /usr/share/fonts/Adwaita/AdwaitaSans-Regular.ttf -stroke white -strokewidth 1 -pointsize 44 -fill white -annotate +60+50 "$label" \
  -quality 90 "$out"
rm -f "$t.png"
