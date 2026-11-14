#!/bin/sh
# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT
# Generate testdata/corpus from testdata/src/pages.ps.
# Needs: ghostscript, djvulibre-bin, pdf2djvu, minidjvu, python3.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
src=$here/src/pages.ps
out=$here/corpus
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for t in gs cjb2 c44 cpaldjvu djvumake djvuextract djvm djvmcvt djvused djvudump pdf2djvu minidjvu python3; do
    command -v $t >/dev/null || { echo "missing: $t" >&2; exit 1; }
done

rm -rf "$out"
mkdir -p "$out"
cd "$work"

# render device dpi output-pattern
render() {
    gs -q -dNOPAUSE -dBATCH -dSAFER -sDEVICE=$1 -r$2 -sOutputFile=$3 "$src"
}
render pbmraw 300 'p%d.pbm'
render ppmraw 300 'p%d.ppm'
render ppmraw 100 'bg%d.ppm'
render pgmraw 100 'gray%d.pgm'
render ppmraw 25  'fg%d.ppm'
render jpeg 100 'bg%d.jpg'
render jpeg 25 'fg%d.jpg'
gs -q -dNOPAUSE -dBATCH -dSAFER -sDEVICE=tiffg4 -dMaxStripSize=0 -r300 \
    -dFirstPage=1 -dLastPage=1 -sOutputFile=p1.tif "$src"
render pdfwrite 72 pages.pdf

# Single-page documents, one per layer kind.
cjb2 -dpi 300 p1.pbm "$out/bilevel.djvu"
cjb2 -dpi 300 -losslevel 100 p4.pbm "$out/bilevel-lossy.djvu"
c44 -dpi 100 bg2.ppm "$out/photo.djvu"
c44 -dpi 100 gray2.pgm "$out/gray.djvu"
c44 -dpi 100 -crcbhalf bg2.ppm "$out/photo-half.djvu"
cpaldjvu -dpi 300 -colors 4 p3.ppm "$out/palette.djvu"

# Compound page assembled by hand: Sjbz + FG44 + BG44.
cjb2 -dpi 300 p1.pbm mask.djvu
djvuextract mask.djvu Sjbz=mask.sjbz
c44 bg2.ppm bg.djvu
djvuextract bg.djvu BG44=bg.bg44
c44 -slice 74 fg3.ppm fg.djvu
djvuextract fg.djvu BG44=fg.fg44
djvumake "$out/compound.djvu" INFO=2550,3300,300 Sjbz=mask.sjbz FG44=fg.fg44 BG44=bg.bg44

# JPEG layers: BGjp + Sjbz + FGjp.
djvumake "$out/jpeg.djvu" INFO=2550,3300,300 Sjbz=mask.sjbz FGjp=fg3.jpg BGjp=bg2.jpg

# MMR mask: the G4 strip of a one-strip TIFF behind the Smmr header.
python3 - p1.tif p1.mmr <<'EOF'
import struct, sys
d = open(sys.argv[1], 'rb').read()
e = '<' if d[:2] == b'II' else '>'
off = struct.unpack(e + 'I', d[4:8])[0]
tags = {}
for i in range(struct.unpack(e + 'H', d[off:off + 2])[0]):
    t, typ, cnt, val = struct.unpack(e + 'HHII', d[off + 2 + 12 * i:off + 14 + 12 * i])
    if typ == 3:  # SHORT, left-justified in the field
        val = val & 0xffff if e == '<' else val >> 16
    tags[t] = val
s = d[tags[273]:tags[273] + tags[279]]
open(sys.argv[2], 'wb').write(b'MMR\0' + struct.pack('>HH', tags[256], tags[257]) + s)
EOF
djvumake "$out/mmr.djvu" INFO=2550,3300,300 Smmr=p1.mmr

# Bundled multi-page document with text layer, outline and annotations.
djvm -c "$out/bundled.djvu" "$out/bilevel.djvu" "$out/photo.djvu" \
    "$out/palette.djvu" "$out/compound.djvu"
cat > txt1 <<'EOF'
(page 0 0 2550 3300
 (line 300 3010 1060 3060
  (word 300 3010 520 3060 "Page")
  (word 545 3010 700 3060 "One:")
  (word 725 3010 1060 3060 "Bilevel")))
EOF
cat > outline <<'EOF'
(bookmarks
 ("Text" "#1")
 ("Pictures" "#2"
  ("Background" "#2")
  ("Colours" "#3"))
 ("Compound" "#4"))
EOF
cat > ant1 <<'EOF'
(maparea "https://example.com/" "Example link" (rect 300 230 700 70) (none))
(maparea "#3" "Go to page 3" (rect 1250 230 420 70) (none))
EOF
djvused "$out/bundled.djvu" -s -e '
    set-outline outline;
    select 1; set-txt txt1; set-ant ant1;
    select; set-meta;
    Title "dejapdf test corpus"
.
'

# Text pages sharing one Djbz dictionary. Lossy shape matching is
# what fills the dictionary; page 2 (halftone) makes minidjvu crawl.
minidjvu -d 300 p1.pbm p4.pbm "$out/shared-dict.djvu"

# Indirect form of the same document.
mkdir "$out/indirect"
djvmcvt -i "$out/bundled.djvu" "$out/indirect" index.djvu

# pdf2djvu: compound pages, hidden text, links, outline.
pdf2djvu -q --dpi=300 -o "$out/pdf2djvu.djvu" pages.pdf

# Oracle outputs.
cd "$out"
for f in *.djvu indirect/index.djvu; do
    djvudump "$f" > "${f%.djvu}.dump"
done
for f in bundled pdf2djvu; do
    djvused $f.djvu -e output-all > $f.dsed
    djvused $f.djvu -e print-outline > $f.outline
done
# SHA-256 of each page's ddjvu mask (PBM), for pages that have one.
for f in *.djvu; do
    n=$(djvused "$f" -e n)
    for p in $(seq 1 "$n"); do
        ddjvu -mode=mask -format=pbm -page="$p" "$f" "$work/mask.pbm"
        [ -s "$work/mask.pbm" ] && echo "$(sha256sum < "$work/mask.pbm" | cut -c1-64) $f $p"
        rm -f "$work/mask.pbm"
    done
done > masks.sha256
# SHA-256 of each IW44 layer as ddjvu renders it at its own size (PPM):
# BG44 directly, FG44 rewrapped as the background of a page its size.
for f in *.djvu; do
    n=$(djvused "$f" -e n)
    for p in $(seq 1 "$n"); do
        d=$(djvused "$f" -e "select $p; dump")
        sz=$(echo "$d" | grep -m1 'BG44.*#1' | grep -o '[0-9]*x[0-9]*') || true
        if [ -n "$sz" ]; then
            ddjvu -mode=background -aspect=no -size="$sz" -page="$p" -format=ppm "$f" "$work/l.ppm"
            echo "$(sha256sum < "$work/l.ppm" | cut -c1-64) $f $p BG44"
        fi
        sz=$(echo "$d" | grep -m1 'FG44' | grep -o '[0-9]*x[0-9]*') || true
        if [ -n "$sz" ]; then
            djvuextract -page="$p" "$f" FG44="$work/fg.iw" >/dev/null 2>&1
            djvumake "$work/fg.djvu" INFO="${sz%x*},${sz#*x},300" BG44="$work/fg.iw"
            ddjvu -format=ppm "$work/fg.djvu" "$work/l.ppm"
            echo "$(sha256sum < "$work/l.ppm" | cut -c1-64) $f $p FG44"
        fi
    done
done > iw44.sha256
ls -l "$out"
