#!/bin/sh
# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT
# Generate testdata/pdf: scanned PDFs made from the corpus pages.
# Needs: djvulibre-bin, img2pdf, libtiff-tools, imagemagick, libjpeg-turbo-progs,
# ghostscript, qpdf, mupdf-tools, poppler-utils, python3.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
djvu=$here/corpus/pdf2djvu.djvu
ps=$here/src/pages.ps
out=$here/pdf
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for t in ddjvu img2pdf tiff2pdf tiffcp magick cjpeg gs qpdf mutool pdfimages python3; do
    command -v $t >/dev/null || { echo "missing: $t" >&2; exit 1; }
done

# Reproducible dates and IDs.
export SOURCE_DATE_EPOCH=1767225600
date=20260101000000

rm -rf "$out"
mkdir -p "$out"
cd "$work"

# ImageMagick options: no dates; bilevel TIFFs coded as scanners do, with
# black (ink) as the fax black runs.
png='-strip -define png:exclude-chunks=date,time'
tif='-strip -define tiff:photometric=min-is-white'

# Scans of the corpus pages (1, 4: text; 2: picture; 3: coloured text).
ddjvu -format=pbm -page=1 "$djvu" p1.pbm
ddjvu -format=pbm -page=4 "$djvu" p4.pbm
ddjvu -format=pbm -scale=100 -page=1 "$djvu" p1-100.pbm
ddjvu -format=pgm -scale=150 -page=4 "$djvu" p4.pgm
ddjvu -format=ppm -scale=100 -page=2 "$djvu" p2.ppm
ddjvu -format=ppm -scale=100 -page=3 "$djvu" p3.ppm
ddjvu -format=pgm -scale=150 -page=3 "$djvu" p3.pgm
ddjvu -format=ppm -scale=150 -page=3 "$djvu" p3-150.ppm
for p in 1 4; do
    magick p$p.pbm $tif -density 300 -units PixelsPerInch -compress Group4 p$p.tif
    magick p$p.pbm $png -density 300 -units PixelsPerInch -depth 1 p$p.png
done
magick p4.pbm $tif -resize 50% -threshold 50% -density 150 -units PixelsPerInch \
    -compress Group4 p4-150.tif
cjpeg -quality 50 -dct int p4.pgm > p4.jpg
cjpeg -quality 50 -dct int p3.ppm > p3.jpg
cjpeg -quality 50 -dct int p2.ppm > p2.jpg
magick p3.ppm -strip -colorspace CMYK -quality 50 -density 100 -units PixelsPerInch p3-cmyk.jpg
magick p4.pgm $png -density 150 -units PixelsPerInch -depth 8 p4-gray.png
magick p3-150.ppm $png -colors 8 -density 150 -units PixelsPerInch \
    -define png:bit-depth=4 -define png:color-type=3 p3-pal.png

# The strips of a TIFF, concatenated.
cat > strips.py <<'EOF'
import struct, sys
d = open(sys.argv[1], 'rb').read()
e = '<' if d[:2] == b'II' else '>'
off = struct.unpack(e + 'I', d[4:8])[0]
tags = {}
for i in range(struct.unpack(e + 'H', d[off:off + 2])[0]):
    t, typ, cnt, val = struct.unpack(e + 'HHII', d[off + 2 + 12 * i:off + 14 + 12 * i])
    f = 'H' if typ == 3 else 'I'
    if cnt * struct.calcsize(f) <= 4:  # inline, left-justified
        raw = d[off + 10 + 12 * i:off + 14 + 12 * i]
    else:
        raw = d[val:val + cnt * struct.calcsize(f)]
    tags[t] = struct.unpack(e + f * cnt, raw[:cnt * struct.calcsize(f)])
open(sys.argv[2], 'wb').write(b''.join(d[o:o + n] for o, n in zip(tags[273], tags[279])))
EOF

# img2pdf; its /ID comes from the clock, so overwrite it in place.
i2p() {
    img2pdf --nodate "$@"
    while [ "$1" != -o ]; do shift; done
    python3 -I -c 'import re, sys
d = open(sys.argv[1], "rb").read()
d = re.sub(rb"/ID \[<[0-9a-f]{32}><[0-9a-f]{32}>\]", b"/ID [<" + b"0" * 32 + b"><" + b"0" * 32 + b">]", d)
open(sys.argv[1], "wb").write(d)' "$2"
}

# CCITT G4: tiff2pdf, and img2pdf for a two-page book.
tiff2pdf -e $date -o "$out/g4.pdf" p1.tif 2>/dev/null
i2p -o "$out/g4-book.pdf" p1.tif p4.tif

# CCITT G3: no tool writes it, so put a G3 strip into the G4 file by
# qpdf JSON. 1-D (K=0) with EOLs, byte-aligned EOLs, and 2-D (K>0).
g3() {  # name tiffcp-options decodeparms
    tiffcp -r 100000 -c "$2" p1.tif g3.tif 2>/dev/null
    python3 -I strips.py g3.tif g3.bin
    cat > g3.json <<EOF
{"qpdf": [{"jsonversion": 2}, {"obj:7 0 R": {"stream": {"datafile": "g3.bin", "dict": {
  "/Type": "/XObject", "/Subtype": "/Image", "/Width": 2550, "/Height": 3300,
  "/BitsPerComponent": 1, "/ColorSpace": "/DeviceGray", "/Filter": "/CCITTFaxDecode",
  "/DecodeParms": {"/Columns": 2550, "/Rows": 3300, $3}}}}}]}
EOF
    qpdf --deterministic-id "$out/g4.pdf" --update-from-json=g3.json "$out/$1.pdf"
}
g3 g3 g3:1d '"/K": 0, "/EndOfLine": true'
g3 g3-align g3:1d:fill '"/K": 0, "/EndOfLine": true, "/EncodedByteAlign": true'
g3 g3-2d g3:2d '"/K": 4, "/EndOfLine": true'

# Stencil: the G4 page as /ImageMask; BlackIs1 makes ink 1, so /Decode [1 0].
python3 -I strips.py p1.tif p1.g4
cat > mask.json <<'EOF'
{"qpdf": [{"jsonversion": 2}, {"obj:7 0 R": {"stream": {"datafile": "p1.g4", "dict": {
  "/Type": "/XObject", "/Subtype": "/Image", "/Width": 2550, "/Height": 3300,
  "/ImageMask": true, "/Decode": [1, 0], "/Filter": "/CCITTFaxDecode",
  "/DecodeParms": {"/Columns": 2550, "/Rows": 3300, "/BlackIs1": true, "/K": -1}}}}}]}
EOF
qpdf --deterministic-id "$out/g4.pdf" --update-from-json=mask.json "$out/imagemask.pdf"

# Flate (PNG predictors), DCT, Indexed (4 bits).
i2p -o "$out/flate1.pdf" p4.png
i2p -o "$out/flate8.pdf" p4-gray.png
i2p -o "$out/jpeg-gray.pdf" -s 150dpi p4.jpg
i2p -o "$out/jpeg-rgb.pdf" -s 100dpi p3.jpg
i2p -o "$out/jpeg-cmyk.pdf" p3-cmyk.jpg
i2p -o "$out/indexed.pdf" p3-pal.png

# The text of pages.ps, invisible (3 Tr), with its links and outline.
{ echo '%!'; echo '<< /BeginPage { pop 3 .settextrenderingmode } >> setpagedevice'
  tail -n +2 "$ps"; } > text.ps
gs -q -dNOPAUSE -dBATCH -dSAFER -sDEVICE=pdfwrite -o text.pdf text.ps

# OCR: the scans with the text overlaid as a form XObject (the scan stays
# in the page content); and the text PDF with the scans underlaid (each
# scan inside a form XObject), which keeps its outline and links.
i2p -o scan3.pdf -s 100dpi p3.jpg
qpdf --deterministic-id --empty --pages "$out/g4-book.pdf" 1 scan3.pdf "$out/g4-book.pdf" 2 -- scans3.pdf
qpdf --deterministic-id scans3.pdf --overlay text.pdf --from=1,3,4 --to=1-3 -- "$out/ocr.pdf"
i2p -o scan2.pdf -s 100dpi p2.jpg
qpdf --deterministic-id --empty --pages "$out/g4.pdf" scan2.pdf scan3.pdf "$out/g4-book.pdf" 2 -- scans4.pdf
qpdf --deterministic-id text.pdf --underlay scans4.pdf -- "$out/links.pdf"

# Generic filters: an LZW and a RunLength scan (ImageMagick) under
# text in ASCII85 (Ghostscript); then the LZW image is wrapped as
# [/ASCII85Decode /LZWDecode] and a content stream recoded as
# [/ASCIIHexDecode /FlateDecode].
magick p4.pgm -strip -density 150 -units PixelsPerInch -compress LZW lzw.pdf
magick p1-100.pbm -strip -density 100 -units PixelsPerInch -compress RLE rle.pdf
gs -q -dNOPAUSE -dBATCH -dSAFER -sDEVICE=pdfwrite -dASCII85EncodePages=true \
    -o text85.pdf text.ps
qpdf --stream-data=preserve --deterministic-id --empty --pages lzw.pdf rle.pdf -- scans2.pdf
qpdf --stream-data=preserve --deterministic-id scans2.pdf --overlay text85.pdf --from=4,1 \
    --to=1-2 -- filters0.pdf
qpdf --json=2 --json-key=qpdf --json-stream-data=file --json-stream-prefix=st \
    --decode-level=none filters0.pdf > filters0.json
cat > chains.py <<'EOF'
import base64, binascii, json, sys, zlib
objs = json.load(open(sys.argv[1]))['qpdf'][1]
upd = {}
hexed = False
for key in sorted(objs):
    st = objs[key].get('stream')
    if not st:
        continue
    d = st['dict']
    data = open(st['datafile'], 'rb').read()
    if d.get('/Filter') in ('/LZWDecode', ['/LZWDecode']):
        d['/Filter'] = ['/ASCII85Decode', '/LZWDecode']
        data = base64.a85encode(data, wrapcol=75) + b'~>'
    elif d.get('/Filter') == ['/ASCII85Decode', '/FlateDecode'] and '/Subtype' not in d and not hexed:
        hexed = True
        raw = zlib.decompress(base64.a85decode(data, adobe=True))
        d['/Filter'] = ['/ASCIIHexDecode', '/FlateDecode']
        data = binascii.hexlify(zlib.compress(raw, 9)) + b'>'
    else:
        continue
    d.pop('/Length', None)
    open(st['datafile'] + '.new', 'wb').write(data)
    upd[key] = {'stream': {'datafile': st['datafile'] + '.new', 'dict': d}}
json.dump({'qpdf': [{'jsonversion': 2}, upd]}, open(sys.argv[2], 'w'))
EOF
python3 -I chains.py filters0.json chains.json
qpdf --stream-data=preserve --deterministic-id filters0.pdf --update-from-json=chains.json \
    "$out/filters.pdf"

# Rotated pages: 90, 180 and 270.
qpdf --deterministic-id "$out/ocr.pdf" --rotate=+90:1 --rotate=180:2 --rotate=-90:3 \
    -- "$out/rotated.pdf"

# Inline image: a 150 dpi G4 page in the content stream.
python3 -I strips.py p4-150.tif inline.bin
{ printf 'q 612 0 0 792 0 0 cm\nBI /W 1275 /H 1650 /BPC 1 /CS /G /F /CCF /DP << /K -1 /Columns 1275 /Rows 1650 >> ID\n'
  cat inline.bin; printf '\nEI Q\n'; } > inline.content
cat > inline.json <<'EOF'
{"qpdf": [{"jsonversion": 2}, {
  "obj:5 0 R": {"stream": {"datafile": "inline.content", "dict": {}}},
  "obj:4 0 R": {"value": {"/Type": "/Page", "/Parent": "3 0 R",
    "/MediaBox": [0, 0, 612, 792], "/Contents": "5 0 R", "/Resources": {}}}}]}
EOF
qpdf --deterministic-id "$out/g4.pdf" --update-from-json=inline.json \
    --remove-unreferenced-resources=yes "$out/inline.pdf"

# Encrypted with an owner password only: RC4 40 and 128, AES 128 and 256.
# AES-256 keys have random salts, so that file differs on each run.
qpdf --deterministic-id --empty --pages "$out/ocr.pdf" 1 -- ocr1.pdf
enc() {  # name bits options
    n=$1 b=$2; shift 2
    qpdf --static-id --static-aes-iv --allow-weak-crypto ocr1.pdf "$out/$n.pdf" \
        --encrypt --user-password= --owner-password=owner --bits=$b "$@" --
}
enc encrypt-rc4-40 40
enc encrypt-rc4-128 128 --use-aes=n
enc encrypt-aes-128 128 --use-aes=y
enc encrypt-aes-256 256

# Incremental update: the second revision rotates page 1, sets a title
# and replaces the image of page 2.
cp "$out/g4-book.pdf" "$out/incremental.pdf"
cat > update.js <<'EOF'
var doc = mupdf.PDFDocument(scriptArgs[0]);
doc.findPage(0).put("Rotate", 90);
doc.getTrailer().get("Info").put("Title", doc.newString("Second revision"));
doc.findPage(1).get("Resources").get("XObject").get("Im0").writeRawStream(readFile(scriptArgs[1]));
doc.save(scriptArgs[0], "incremental");
EOF
mutool run update.js "$out/incremental.pdf" p1.g4
# mutool's new /ID comes from the clock.
python3 -I -c 'import re, sys
d = open(sys.argv[1], "rb").read()
d = re.sub(rb"><[0-9A-F]{32}>\]", b"><" + b"1" * 32 + b">]", d)
open(sys.argv[1], "wb").write(d)' "$out/incremental.pdf"

# Look-alike characters for shape matching: one character per row, 13 pt
# apart from 36 pt, rows as in internal/shapes TestLookalikes; Times 7
# and 10, Helvetica 9, Courier 9; scanned clean, dark and rough.
python3 -I - > digits.ps <<'EOF'
rows = "0 O o D Q C G 1 l I i j ! | 7 T 3 8 6 9 5 S s 2 Z z B E F P R c e a n h u b d p q g , . : ; ' - r f t".split()
print("%!PS")
for f, s in [("Times-Roman", 7), ("Times-Roman", 10), ("Helvetica", 9), ("Courier", 9)]:
    print(f"/{f} findfont {s} scalefont setfont")
    for i, c in enumerate(rows):
        c = c.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")
        print(f"36 {756 - 13 * i} moveto ({' '.join([c] * 40)}) show")
    print("showpage")
EOF
gs -q -dNOPAUSE -dBATCH -dSAFER -sDEVICE=pgmraw -r1200 -dTextAlphaBits=4 -o d-%d.pgm digits.ps
for q in clean dark rough; do
    case $q in
    clean) op='-threshold 50%' ;;
    dark) op='-blur 0x1 -threshold 65%' ;;
    rough) op='-seed 1 -attenuate 0.6 +noise Gaussian -threshold 45%' ;;
    esac
    for n in 1 2 3 4; do
        magick d-$n.pgm -filter Triangle -resize 25% $op $tif -density 300 -units PixelsPerInch \
            -compress Group4 d-$q-$n.tif
    done
done
i2p -o "$out/digits.pdf" d-clean-?.tif d-dark-?.tif d-rough-?.tif

# Oracle outputs.
cd "$out"
for f in *.pdf; do
    qpdf --check "$f" >/dev/null
    pdfimages -list "$f" > "${f%.pdf}.images"
    echo "$(qpdf --show-npages "$f") $f"
done > npages.txt
ls -l "$out"
