# Portable Document Format

What dejapdf's PDF writer must know about PDF, and nothing more: the file
structure, the objects, images and masks, the JBIG2 and JPEG 2000 filters,
invisible text, outlines, links and metadata. Numbers in parentheses are
sections of ISO 32000-1:2008 (PDF 1.7), [PDF32000_2008.pdf](PDF32000_2008.pdf);
see [README.md](README.md) to get it. Every claim marked *checked* was tried
on a hand-made file with `qpdf --check`, MuPDF (`mutool`) and Poppler
(`pdftoppm`, `pdftotext`).

How DjVu maps to PDF, in one table:

| DjVu | PDF |
|:---|:---|
| page, INFO size and dpi | page object, MediaBox in points (§6) |
| BG44 background | image XObject, JPXDecode (§11), stretched to the page |
| Sjbz mask, Djbz dictionary | stencil image, JBIG2Decode (§10); Djbz → JBIG2Globals |
| FG44 foreground | JPXDecode image whose /Mask is the stencil (§8) |
| FGbz palette | one stencil per colour (§9) |
| BGjp, FGjp | DCTDecode, passed through (§3) |
| Smmr mask | CCITTFaxDecode, K −1 (§3) |
| TXTz | invisible text, render mode 3 (§12) |
| NAVM | outline (§13) |
| ANTz maparea | Link annotation (§14) |
| DIRM title, metadata | Info dictionary (§15) |

## 1. The file at a glance

A PDF file is a header, a body of numbered objects, a cross-reference table
giving each object's byte offset, and a trailer that names the root object and
says where the table starts (7.5). A reader starts at the end: `%%EOF`, then
`startxref`, then the table, then any object it needs, by offset.

This is a complete, valid one-page file, 462 bytes; it paints a blue square
(*checked*):

```
%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 288 288]
   /Resources << >> /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 27 >>
stream
0 0 1 rg 72 72 144 144 re f
endstream
endobj
xref
0 5
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
0000000222 00000 n 
trailer
<< /Size 5 /Root 1 0 R >>
startxref
299
%%EOF
```

![The minimal file. Each xref entry holds the byte offset of an object;
startxref holds the offset of the xref table; the trailer's /Root names the
catalog.](figures/pdf_file.svg)

The writer therefore works in one pass: write objects, remember each offset,
then write the table and trailer. Object numbers can be assigned before the
objects are written, so forward references (`3 0 R` before object 3 exists)
cost nothing.

## 2. Objects

Eight types (7.3):

| Type | Syntax | Notes |
|:---|:---|:---|
| boolean | `true` `false` | |
| number | `612` `-3` `0.5` `.5` | no exponents; integers and reals |
| string | `(text)` or `<48656C6C6F>` | `\(` `\)` `\\` escape in literal strings |
| name | `/Type` `/A#20B` | `#xx` escapes any byte outside `!`–`~` and delimiters |
| array | `[1 2 /X (s)]` | any mix of types |
| dictionary | `<< /Key value ... >>` | keys are names; a null value means absent |
| stream | dictionary + `stream` … `endstream` | always an indirect object |
| null | `null` | |

An *indirect object* is `N G obj … endobj`; it is referred to as `N G R`.
Generation G is always 0 in a file written once. Whitespace is space, tab, CR,
LF, FF, NUL; `%` starts a comment to the end of the line. Delimiters
`( ) < > [ ] { } / %` end a token, so `/Type/Page` needs no space. Keep lines
outside streams under 255 bytes (7.5.1).

Write numbers without needless digits: `0.5`, not `0.500000`. Coordinates need
at most two or three decimals.

## 3. Streams and filters

A stream is a dictionary, the keyword `stream`, an end-of-line (LF or CR LF,
never CR alone), exactly `Length` bytes, an optional EOL, and `endstream`
(7.3.8). `Length` counts the encoded bytes.

`Filter` names the decoding to apply, `DecodeParms` its parameters; arrays of
both chain filters (7.4). The filters dejapdf uses:

| Filter | Since | Use | Parameters |
|:---|:---|:---|:---|
| FlateDecode | 1.2 | content streams, small images, object streams | none needed |
| JBIG2Decode | 1.4 | the mask (§10) | `JBIG2Globals` |
| JPXDecode | 1.5 | background, foreground (§11) | none: the JP2 file carries them |
| DCTDecode | 1.0 | BGjp/FGjp layers, as is | `ColorTransform` only if the JPEG lacks the Adobe marker |
| CCITTFaxDecode | 1.0 | Smmr masks, with DjVu's 8-byte MMR header removed (*checked*) | `K -1` (G4), `Columns`, `Rows`, `BlackIs1` |

JBIG2Decode and JPXDecode only work in image XObjects, never in inline images
(7.4.7, 7.4.9). DCT data is baseline or progressive JPEG with one byte per
component (7.4.8): DjVu's JPEG layers need no change.

## 4. File structure in depth

**Header** (7.5.2). `%PDF-1.5` (JPXDecode and object streams need 1.5), then a
comment with at least four bytes ≥ 128 so that tools treat the file as binary:
`%\xe2\xe3\xcf\xd3`.

**Cross-reference table** (7.5.4). `xref`, then `first count`, then one entry
per object, *exactly 20 bytes*: a 10-digit offset, a space, a 5-digit
generation, a space, `n`, and a two-byte EOL (`SP LF`, `SP CR` or `CR LF`).
Entry 0 is always `0000000000 65535 f`. There must be an entry for every
number from 0 to the highest.

**Trailer** (7.5.5). `trailer << /Size N /Root R /Info R /ID [...] >>`,
`startxref`, the offset of `xref`, `%%EOF`. `Size` is the highest object
number plus one. `ID` (two random 16-byte strings) is optional but expected by
some workflows (14.4).

**Object streams** (7.5.7, PDF 1.5). Many small dictionaries (pages, image
dictionaries, outline items, annotations) can be packed into one stream and
Flate-compressed together. The stream dictionary is
`<< /Type /ObjStm /N count /First offset >>`; the data begins with `N` pairs
"object-number offset" (offsets relative to `First`), followed by the objects
without `obj`/`endobj`. Streams themselves cannot go into an object stream, and
neither can the `Length` of an object stream.

**Cross-reference streams** (7.5.8). With object streams the xref table is
replaced by a stream `<< /Type /XRef /Size N /W [1 4 2] /Root ... >>` whose
data is binary rows of `W` bytes: type 1 = (offset, generation), type 2 =
(object stream number, index in it), type 0 = free. The xref stream also plays
the trailer: `Root`, `Info`, `ID` go in its dictionary; `startxref` points at
it, and the keywords `xref` and `trailer` disappear (*checked*).

![Classic layout versus PDF 1.5. Left: every dictionary as plain text and a
20-byte text entry per object. Right: the dictionaries compressed together in
an object stream, and binary xref entries in a compressed xref
stream.](figures/pdf_objstm.svg)

For a scanned book the image streams dominate, but each page also costs a page
dictionary, two or three image dictionaries, a content stream and their xref
entries: a few hundred bytes. Over 500 pages that is ~100 KB; object streams
shrink it several times. We never update files incrementally (7.5.6).

`internal/pdf` packs every non-stream object into object streams of 100 and
ends with a Flate-coded xref stream, `/W [1 4 2]` (*checked*: `qpdf --check`,
MuPDF and Poppler read it). On three books it saves 164–182 bytes a page,
about 100 KB per book.

## 5. Document structure

![The objects of a two-page output. The catalog leads to the page tree and the
outline; each page has its contents and resources; JBIG2 images share one
globals stream; outline items and links point at pages.](figures/pdf_tree.svg)

**Catalog** (7.7.2): `<< /Type /Catalog /Pages R >>`, plus `/Outlines R`,
`/PageMode /UseOutlines` (open the bookmarks pane), `/PageLabels`.

**Page tree** (7.7.3): `<< /Type /Pages /Kids [...] /Count n >>`. A flat
`Kids` array of all pages is legal; for thousands of pages a two-level tree
(say 100 pages per node) helps viewers. `Count` is the number of leaf pages
below the node.

**Page**: `<< /Type /Page /Parent R /MediaBox [...] /Resources ... /Contents R >>`,
optionally `/Rotate`, `/Annots`. `Resources` is required; it may be inherited
from a `Pages` node (qpdf warns if it is missing, *checked*).
`MediaBox`, `Resources`, `Rotate` and `CropBox` are inheritable.

**Resources** (7.8.3): `<< /XObject << /Bg R /Mask R >> /Font << /F1 R >> >>`.
Content streams refer to resources by these names: `/Bg Do`, `/F1 12 Tf`.

**Content stream** (7.8.2): the page's drawing as postfix operators,
operands first: `q 612 0 0 792 0 0 cm /Bg Do Q`. Flate-compress it.

## 6. Coordinates

User space is measured in points, 1/72 inch, with the origin at the lower
left, y up (8.3.2). A DjVu page of W × H pixels at `dpi` is
`/MediaBox [0 0 W·72/dpi H·72/dpi]`: 2550 × 3300 at 300 dpi gives 612 × 792,
US Letter.

![A DjVu page in pixels becomes a MediaBox in points. An image always fills
its own unit square; the cm operator maps that square onto the page: a is the
width, d the height, (e, f) the lower left corner.](figures/pdf_coords.svg)

**The current transformation matrix** (8.3.4). `a b c d e f cm` multiplies the
CTM. `q` saves the graphics state, `Q` restores it (8.4.2); bracket each image
with them.

**Images** are always painted into the unit square of user space (8.9.4). The
image's row 0 is the *top* of that square. So `q w 0 0 h x y cm /Im Do Q`
paints the image upright, w × h points, with its lower left corner at (x, y).
Every DjVu layer covers the whole page, whatever its own resolution: draw it
with `q W 0 0 H 0 0 cm` where W × H is the MediaBox. A 213 × 275 FG44 and a
2550 × 3300 mask then cover the same area.

**Rotation.** INFO's rotation goes to the page's `/Rotate` (multiple of 90,
clockwise), so the content stays unrotated (see `djvu.Info.Rotate`).

**DjVu text and annotation coordinates** are pixels with the origin at the
lower left (DjVu spec, annotations), like PDF: multiply by 72/dpi.

## 7. Images

An image XObject is a stream (8.9.5):

```
<< /Type /XObject /Subtype /Image /Width 2550 /Height 3300
   /ColorSpace /DeviceRGB /BitsPerComponent 8
   /Filter /FlateDecode /Length ... >>
```

| Key | Meaning |
|:---|:---|
| Width, Height | samples; must match the encoded data |
| ColorSpace | DeviceGray, DeviceRGB, Indexed …; absent for masks and optional for JPX |
| BitsPerComponent | 1, 2, 4, 8, 16; absent for JPX; 1 for masks |
| Decode | maps sample values to colour components; `[1 0]` inverts a 1-bit image |
| ImageMask | true: a stencil (§8) |
| Mask | an explicit stencil, or a colour-key range (§8) |
| SMask | a soft mask: a grayscale image of alpha (§8) |
| Interpolate | smooth when scaled up; for stencils, smooths the edges (8.9.6.2) |

Samples are rows from top to bottom, left to right, components interleaved,
high-order bit first. Each row starts on a byte boundary; the unused bits at
the end of a row are ignored (8.9.3). Inline images (`BI … ID … EI`, 8.9.7)
save an object but cannot use JBIG2 or JPX: avoid them.

`/Type /XObject` is optional in a stream's dictionary, and `BitsPerComponent`
is optional for masks and JPX. dejapdf leaves them out. That saves 33 bytes an
image, which matters for pages with many palette stencils.

## 8. Masks

![Stencil: the mask paints the current fill colour. Explicit mask: it decides
where the base image is painted. Soft mask: each sample is an opacity, white
opaque, black transparent.](figures/pdf_masks.svg)

**Stencil** (8.9.6.2): `/ImageMask true`, 1 bit per sample, no ColorSpace.
Painting it fills the current non-stroking colour (`rg`, `g`) through the
mask; the rest of the page is untouched. With the default `/Decode [0 1]`
*sample 0 paints*; `/Decode [1 0]` makes 1 paint.

**Explicit mask** (8.9.6.3, PDF 1.3): the base image's `/Mask` is a stencil
image. The base image is painted only where the stencil would paint, with the
same Decode rule. The two need not have the same resolution: both fill the unit
square (*checked*: an 8 × 8 mask over a 1 × 1 image). This is FG44 over its
JB2 mask: the low-resolution colours appear only on the shapes.

**Colour key** (8.9.6.4): `/Mask [min max ...]`, a colour range to skip.
Unreliable with lossy JPX; not used.

**Soft mask** (11.6.5.3, PDF 1.4): `/SMask` is a DeviceGray image of alpha.
Partial transparency; we need only on/off, so the explicit mask is enough.
JPX can also carry alpha (`SMaskInData`); not used.

**Polarity, checked.** JBIG2Decode delivers *black as 0*, the PDF convention:
a JBIG2 image used as a DeviceGray image or as a stencil with the *default*
Decode renders identical to `jbig2dec` in both MuPDF and Poppler; with
`/Decode [1 0]` the stencil paints the background instead. So a JBIG2 stencil
needs no Decode. A Flate-coded 1-bit mask written with 1 = black (as in a PBM
file) needs `/Decode [1 0]`, or write it with 0 = black.

## 9. A DjVu page in PDF

![The painting order. Each layer is one image filling the page; the text and
links lie on top.](figures/pdf_layers.svg)

The content stream of a compound page (W × H in points):

```
q W 0 0 H 0 0 cm /Bg0 Do Q         % background, JPX
q W 0 0 H 0 0 cm /Fg1 Do Q         % foreground JPX with /Mask = stencil
BT 3 Tr ... ET                     % hidden text (§12)
```

Without FG44, step two is `0 g q W 0 0 H 0 0 cm /Sm1 Do Q`: the stencil in
black. With an FGbz palette, the blits are split by colour, one stencil each:
`r g b rg q W 0 0 H 0 0 cm /Sm1 Do Q`, then the next. Bitonal pages have only
the stencil; photo pages only the background. Annotations are not in the
content: they are in `/Annots`. What `internal/convert` adds:

- **Layer geometry.** A layer reduced k times is ⌈W/k⌉ × ⌈H/k⌉ pixels, each
  covering k × k page pixels from the bottom left, so its image is drawn
  k·⌈W/k⌉ by k·⌈H/k⌉ pixels large and sticks out at the top and right; the
  MediaBox clips it. The stencil used as FG44's `/Mask` is padded to the same
  size, since an explicit mask fills the image's unit square. A layer whose
  size is no reduction of the page is stretched to it.
- **Uniform layers.** A layer whose samples all lie within ±2 is painted as
  one colour: a filled rectangle for the background (nothing if white), the
  stencil in that colour for the foreground. Most book pages are white with
  black or one-colour text.
- **Unused foreground.** FG44 samples under no mask pixel are never shown.
  dejapdf replaces them with their shown neighbours' average, spreading out
  breadth-first, before the uniform test and the JPX coding: black text with
  noise around it becomes one colour.
- **Budget.** A JPX layer gets the bytes of its IW44 chunks times `-q`.
- **Parallel.** Pages are built concurrently and written in order; output is
  the same for any number of threads.

*Checked* against `ddjvu` at the page resolution, rendering with `mutool draw`:
the bilevel, Smmr and pdf2djvu text pages are identical; palette pages 69 dB;
photo pages 45–54 dB (the JPX at 1×); compound pages 29.5 dB, the JPEG page
35 dB. On mask pixels the difference is the viewers': MuPDF and Poppler
interpolate an FG layer upscaled 12 times, ddjvu takes the nearest sample.
Sample pages of the books: text pages identical, colour pages 39–47 dB.

All 15 books convert and pass `qpdf --check`, in 0.5–14 s on 16 threads.
With hidden text, bookmarks and links, and the colour layers at the default
`-q 1.2`, the PDFs are 1.06–1.17× the DjVu size; at `-q 1`, 0.99–1.11×. One
scanned book stands out at 1.29× (1.22× at `-q 1`). Its FGbz palettes average
about six grey shades a page, and each shade is a stencil. That costs about
210 bytes of image dictionary, segment headers and page information per
stencil, or 0.6 MB in a 7.5 MB book. Before symbol
dictionaries they were 1.9–13.6× with Flate masks, 1.04–4.6× with one JBIG2
generic region a page. The masks are 1.04–1.13× their JB2 data, the colour
layers 0.65–1.00× their IW44 data, the hidden text 0.90–1.08× its TXTz.

## 10. JBIG2Decode

7.4.7; the format itself is ITU-T T.88 ([T-REC-T.88](README.md)). PDF uses the
*embedded* organisation of JBIG2:

- no file header, no end-of-page segments, no end-of-file segment;
- one image XObject per JBIG2 page, holding that page's segments, with the
  segment page association set to 1;
- global segments (page association 0) go in a separate stream, named by
  `/DecodeParms << /JBIG2Globals R >>`; one globals stream may serve many
  images.

![DjVu's shared Djbz becomes the JBIG2Globals stream: a symbol dictionary with
page association 0. Each page becomes an image with a page information
segment, an optional local symbol dictionary and a text region of
blits.](figures/pdf_jbig2.svg)

The image dictionary: `/Width`, `/Height` (equal to the page information
segment's), `/BitsPerComponent 1`, and `/ImageMask true` or
`/ColorSpace /DeviceGray`. The experiment split a T.88 file this way
(header dropped, page association rewritten to 1, end-of-page and end-of-file
dropped) and both MuPDF and Poppler rendered it exactly (*checked*).

## 11. JPXDecode

7.4.9; PDF 1.5. The stream holds a JPEG 2000 *file* (JP2 or JPX), not a bare
codestream: the spec requires the file structure. In practice both MuPDF and
Poppler draw a bare codestream too, but Poppler logs "Malformed JP2 file
format: first box must be JPEG 2000 signature box". The JP2 boxes cost about 85
bytes (*checked* with OpenJPEG's `opj_compress`). Write JP2.

- `Width`, `Height` must match the codestream.
- `BitsPerComponent` is omitted: the data has it.
- `ColorSpace` is optional; without it the JP2 colour specification is used
  (*checked*: RGB and gray). If present, it overrides the file's.
- Only the JPX baseline features; colours enumerated sRGB or greyscale are fine.
- `Decode` is ignored unless the image is a mask.
- `SMaskInData 1` would take alpha from the file; not used.

## 12. Invisible text

![A word is fitted to its box: the font size is the box height, the start is
the box's lower left, and horizontal scaling stretches the font's natural
width to the box width. Render mode 3 makes it invisible.](figures/pdf_text.svg)

**Operators** (9.3, 9.4). `BT … ET` brackets text. `/F1 size Tf` selects the
font, `x y Td` moves to a position (the first `Td` in a `BT` is absolute),
`k Tz` sets horizontal scaling in percent, `3 Tr` makes glyphs neither filled
nor stroked, `<…> Tj` shows a string. Text in mode 3 is still selectable,
searchable and extractable.

**Fitting a word.** Each glyph advances `w0/1000 × size × Tz/100` (9.4.4). With
a constant width `DW` for every glyph, a word of n characters needs
`k = 100 × boxwidth / (n × DW/1000 × size)`.

What dejapdf writes, per page:

```
q s 0 0 s 0 0 cm BT 3 Tr           % s = 72/dpi: units are DjVu pixels
/Tx 50 Tf 300 3010 Td 196 Tz (…)Tj % line height; first word, absolute
245 0 Td 144 Tz (…)Tj              % next word, relative to the last
ET Q
```

- Words of a line take the line's bottom and height, so `Tf` and the `Td` y
  change once a line; the selection highlight is one even band.
- A word that has a next word on its line gets a trailing space, and its `Tz`
  stretches it to that word's start: copied text keeps the gaps, and
  extractors need not guess them.
- Integer pixel coordinates and integer `Tz`; strings are literal binary
  UTF-16BE (only `(`, `)`, `\` and CR escaped).

Compared with `Tm` per word in points and hex strings, this halves the
text's size: from 1.5–1.9× the TXTz chunks to 0.90–1.08×. Flate at its
best level saves another 1.6% of the whole file at no visible cost in time.

**Fonts.** The 14 standard fonts (Helvetica …) need no embedding but cover
Latin only. Russian books need a *composite font* (9.7):

```
<< /Type /Font /Subtype /Type0 /BaseFont /GlyphLess
   /Encoding /Identity-H
   /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2
       /BaseFont /GlyphLess /DW 500 /FontDescriptor R
       /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> >>]
   /ToUnicode R >>
```

With `Identity-H` the string bytes are 2-byte CIDs; use the UTF-16 code of
each character as its CID, and a ToUnicode CMap mapping every CID to itself
(9.10.3):

```
/CIDInit /ProcSet findresource begin 12 dict begin begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def /CMapType 2 def
1 begincodespacerange <0000> <FFFF> endcodespacerange
1 beginbfrange <0000> <FFFF> <0000> endbfrange
endcmap CMapName currentdict /CMap defineresource pop end end
```

Then `Привет` is `<041F04400438043204350442> Tj` (or the same bytes as a
literal string), and `pdftotext` returns
`Привет` with the word box exactly where it was put (*checked*: both MuPDF and
Poppler report x from 50 to 350 for a 300-point box). A non-embedded CIDFont
needs a predefined CMap such as Identity-H (9.7.6) and a font descriptor
(`/FontName /Flags /FontBBox /ItalicAngle /Ascent /Descent /CapHeight
/StemV`, 9.8); its Ascent and Descent set the height of the selection
highlight. Embedding a tiny glyph-less TrueType font instead makes the file
self-contained (PDF/A wants this); characters outside the BMP would need
surrogate pairs in ToUnicode.

dejapdf embeds a 296-byte TrueType (`FontFile2`, written once per file, only
if some page has text): `head`, `hhea`, `hmtx`, `maxp`, `loca`, `glyf` and
`post`, no `cmap` (a CIDFontType2 maps CIDs to glyphs by `/CIDToGIDMap`, here
a Flate stream sending all 65536 CIDs to glyph 1). Glyph 1 is a 500 × 1000
*box*, not empty: MuPDF gives empty glyphs zero-height boxes and drops some
of their characters from extraction (*checked*: page 1 of the bundled corpus
file came out empty). Render mode 3 never paints the box (*checked*: MuPDF,
Poppler and Ghostscript renders unchanged).

## 13. Outlines and destinations

![The outline is a tree of linked lists: First and Last go down a level, Next
and Prev go across, every item names its Parent. Counts are of open
descendants.](figures/pdf_outline.svg)

The catalog's `/Outlines` is `<< /Type /Outlines /First R /Last R /Count n >>`
(12.3.3). Each item: `/Title` (text string, §15), `/Parent`, `/Prev`, `/Next`,
`/First`, `/Last`, `/Count`, and a destination. `Count` of an item is the
number of its visible descendants; negative means the item starts closed.

A destination (12.3.2) is an array: `[page /XYZ left top zoom]` (`null`
keeps the current value; a zoom of 0 means the same, but write `null`:
`mutool show outline` prints 0 as `nan`), `[page /Fit]`, `[page /FitH top]`. `page` is the page object. DjVu's NAVM
bookmarks `("Title" "#n" children…)` map one to one, `#n` being page n.
DjVu URLs also name a page by its component id or title (`#p0003.djvu`, as
pdf2djvu writes them) or relative to the current page (`#+1`, `#-1`); ids
and titles are tried first. dejapdf writes `[page /XYZ null null null]`,
every item closed, and `/PageMode /UseOutlines`.

## 14. Links

A link is an annotation in the page's `/Annots` (12.5.6.5):

```
<< /Type /Annot /Subtype /Link /Rect [x1 y1 x2 y2] /Border [0 0 0]
   /A << /S /URI /URI (https://example.com/) >> >>
<< /Type /Annot /Subtype /Link /Rect [...] /Border [0 0 0] /Dest [page /Fit] >>
```

`Rect` is in user space; `/Border [0 0 0]` hides the default border.
External links use a URI action (12.6.4.7), internal ones a destination or a
GoTo action (12.6.4.2). DjVu's `(maparea "url" "comment" (rect x y w h))` gives
`Rect [x y x+w y+h] × 72/dpi`; a url `#n` is page n. The comment can go into
`/Contents`. Oval and poly areas become their bounding rectangle; text and
line areas (notes and drawings, not links) are dropped, as are mapareas
with an empty url. Page targets are allocated before any page is written,
so a link can point forward.

## 15. Metadata and strings

The trailer's `/Info` dictionary (14.3.3): `/Title`, `/Author`, `/Subject`,
`/Keywords`, `/Creator`, `/Producer`, `/CreationDate`, `/ModDate`. DjVu
metadata (ANTz `metadata`) maps to it by name.

**Text strings** (7.9.2.2) are either PDFDocEncoding (Latin-1-like) or UTF-16BE
with a byte order mark: `<FEFF0422043504410442>` is "Тест" (*checked* with
`pdfinfo`). Write ASCII as a literal string, anything else as UTF-16BE hex.

**Dates** (7.9.4): `(D:20261113120000Z)` or `(D:20261113120000+03'00')`.
djvused writes `2026-11-13 19:38:50-08:00`; pdf2djvu copies the PDF's dates
this way. dejapdf converts that and RFC 3339 dates and drops the rest;
`/Producer` is always dejapdf's.

## 16. Keeping it small, and checking it

Size checklist:

- Pre-encoded JBIG2 and JPX streams pass through; never re-encode them with
  Flate.
- One JBIG2Globals per Djbz, shared by its pages; one font object per file.
- Flate content streams and small images; object streams and an xref stream
  for the many small dictionaries (§4).
- Numbers with few digits; no indentation, no comments.
- No duplicate objects: a background used twice is one object.

Checks, run but never linked:

| Tool | Checks |
|:---|:---|
| `qpdf --check file.pdf` | syntax, xref, streams decode |
| `mutool draw -o p.png file.pdf 3` | renders a page (MuPDF) |
| `pdftoppm -r 300 file.pdf p` | renders pages (Poppler) |
| `pdftotext -bbox file.pdf -` | text and word boxes |
| `firefox --headless --screenshot p.png file:///…/f.pdf` | renders page 1 in pdf.js |
| `mutool show file.pdf outline` | the outline |
| `jbig2dec -t pbm` | decodes our JBIG2 streams, given a file header |
| `opj_dump -i f.jp2` | inspects our JP2 files |
