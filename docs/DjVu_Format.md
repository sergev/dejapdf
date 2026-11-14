# DjVu Format

What dejapdf's decoder must know about DjVu: the container, the documents,
the ZP coder and BZZ, the JB2 mask, the IW44 images, the colour palette, the
hidden text, annotations and outlines. Numbers in parentheses are sections of
the Lizardtech DjVu v3 spec (2005), [DjVu3Spec.djvu](DjVu3Spec.djvu). The
spec is old and partly wrong; [README.md](README.md) lists its errata. A
claim marked *checked* was tested on `testdata/corpus` and real books against
DjVuLibre's tools (`djvudump`, `djvused`, `ddjvu`). The PDF side is in
[Portable_Document_Format.md](Portable_Document_Format.md).

Where each part goes, in one table:

| DjVu | Decoded by | Becomes in PDF |
|:---|:---|:---|
| IFF chunks, DIRM, INCL, INFO | `internal/iff`, `internal/djvu` | pages, MediaBox, /Rotate |
| ZP coder | `internal/zp` | (used by BZZ, JB2, IW44) |
| BZZ | `internal/bzz` | (used by DIRM, NAVM, ANTz, TXTz, FGbz) |
| Sjbz, Djbz (JB2) | `internal/jb2` | JBIG2 stencil, JBIG2Globals |
| BG44, FG44 (IW44) | `internal/iw44` | JPX images |
| FGbz | `internal/djvu` | one stencil per colour |
| BGjp, FGjp | passed through | DCTDecode |
| Smmr | `internal/djvu`, G4 by `internal/ccitt` | a stencil, JBIG2 generic region |
| TXTz | `internal/djvu` | invisible text |
| ANTz | `internal/djvu` | Link annotations, Info |
| NAVM | `internal/djvu` | outlines |

## 1. The layered page

A DjVu page follows the Mixed Raster Content model (3.1, 7.1). It has up to
three layers, each coded with its own method:

- the **background**: a colour image of paper and pictures, coded with the
  IW44 wavelet coder in one or more BG44 chunks, usually at 1/3 of the page
  resolution;
- the **mask**: a full-resolution bilevel image of the text and line art,
  coded with JB2 in one Sjbz chunk;
- the **foreground colour**: the colour of the masked pixels, either a small
  IW44 image in one FG44 chunk (usually 1/12 resolution) or one colour per
  mask shape from a palette in an FGbz chunk.

Where the mask is black, the page shows the foreground colour; elsewhere it
shows the background. A page without a mask is a *photo* page (BG44 only); a
page with a mask only is *bilevel*, black on white; a page with both is
*compound*.

![A compound page. The background is smooth and low-resolution, the mask
is sharp and bilevel, the foreground colour is coarse. Each one compresses
well with its own coder.](figures/djvu_layers.svg)

A layer coded at lower resolution has size ⌈W/k⌉ × ⌈H/k⌉ for an integer
k from 1 to 12, where W × H is the page size from INFO (10.3). The 2550×3300
corpus page has a background of 850×1100 (k = 3) and a foreground of 213×275
(k = 12) (*checked*). dejapdf keeps every layer at its own resolution: the
PDF stretches each image to the page.

Each layer has an alternative coding (7.1.3.3): a JPEG background (BGjp) or
foreground (FGjp), and a G4 fax mask (Smmr). Annotations (ANTz), hidden text
(TXTz) and an outline (NAVM) may be added to any document.

## 2. The IFF container

A DjVu file is an EA IFF 85 file (8.2). It starts with the 4 bytes `AT&T`,
which are not part of IFF. Then there is a single FORM chunk. Every chunk has
the same layout:

| Bytes | Field |
|:---|:---|
| 4 | chunk id, ASCII, such as `INFO` |
| 4 | data length, big-endian; does not count itself, the id or the pad byte |
| length | data |
| 0 or 1 | a 0x00 pad byte when the length is odd, so the next chunk starts at an even offset |

A FORM is a composite chunk. Its data starts with a 4-byte form type, and the
rest is more chunks. `FORM` with type `DJVU` is written `FORM:DJVU`. Readers
skip chunks they do not know (8.2.1).

![The first bytes of a one-page file, and the same file as djvudump shows
it. The FORM size counts the form type and everything after
it.](figures/djvu_iff.svg)

The magic is required at the start of a file (8.1); it is 4 bytes, so it does
not change which offsets are even. A component inside a bundle has no magic.
`internal/iff` accepts files with and without it.

The chunks (8.2.2, Table 1):

| Chunk | Where | Contents | Coding |
|:---|:---|:---|:---|
| FORM:DJVM | top | a multipage document | composite |
| FORM:DJVU | top, or in DJVM | one page | composite |
| FORM:DJVI | in DJVM | shared data, reached by INCL | composite |
| FORM:THUM | in DJVM | thumbnails | composite |
| DIRM | first in DJVM | the document directory (§7) | header + BZZ |
| NAVM | in DJVM, after DIRM | the outline (§14) | BZZ |
| INFO | first in DJVU | page size, dpi, rotation (§4) | raw |
| INCL | DJVU, DJVI | id of a DJVI to include (§3) | raw UTF-8 |
| Sjbz | DJVU | the mask (§8) | JB2 |
| Djbz | DJVU, DJVI | a shared shape dictionary (§8) | JB2 |
| FGbz | DJVU | foreground palette (§10) | raw + BZZ |
| BG44 | DJVU | background, one or more, in order (§9) | IW44 |
| FG44 | DJVU | foreground colour (§9) | IW44 |
| TH44 | THUM | one page thumbnail | IW44 |
| BGjp, FGjp | DJVU | JPEG background, foreground (§11) | JPEG |
| Smmr | DJVU | G4 fax mask (§11) | MMR |
| ANTa, ANTz | DJVU, DJVI | annotations (§13) | text, BZZ |
| TXTa, TXTz | DJVU | hidden text (§12) | raw, BZZ |

INFO must be the first chunk of a page. After it, chunks may come in any
order, except that the BG44 chunks must stay in their order (8.3.1.2).
dejapdf ignores thumbnails; a PDF viewer makes its own. The `a` chunks
(ANTa, TXTa) are the uncompressed forms of the `z` chunks; the spec discourages
them, but they are valid.

## 3. Documents

A *document* is one of three things (3.2, 7.2):

- **single page**: one `FORM:DJVU` in one file;
- **bundled**: one file holding a `FORM:DJVM`. Its first chunk is the DIRM
  directory; then an optional NAVM; then the *components*: pages (`DJVU`),
  shared data (`DJVI`) and thumbnails (`THUM`), each one a FORM;
- **indirect**: an index file holding a `FORM:DJVM` with DIRM (and NAVM) but
  no components. Each component is a file of its own, named by DIRM, in the
  same directory.

Each component is itself a valid IFF file (7.2.1); a page component of an
indirect document is a valid one-page DjVu file.

![Bundled and indirect documents. In a bundle, DIRM gives each component's
offset; in an indirect document, its file name. The dashed arrows are INCL
chunks: both pages include the shared DJVI.](figures/djvu_docs.svg)

A page shares data through an **INCL** chunk (7.2.2, 8.3.12). Its data is the
UTF-8 id of a component, which must be a `FORM:DJVI`. The decoder then reads
the chunks of that DJVI *as if they were in the page*. A DJVI may itself hold
INCL chunks. This is how pages share a JB2 shape dictionary (Djbz) or common
annotations (ANTz).

The order of pages is the order of the page components in DIRM. An INCL id
is looked up in DIRM; in a single-page file there is no DIRM, and an INCL
names a file next to it. `djvu.Open` handles all three kinds and returns, for
each page, its chunks with the included ones appended (`Page.Chunks`).

Thumbnails: a `FORM:THUM` holds TH44 chunks for the pages that follow it
(8.3.1.4). dejapdf skips them.

## 4. INFO

The INFO chunk is 10 bytes (8.3.11):

| Bytes | Field | Notes |
|:---|:---|:---|
| 2 | width | pixels, big-endian |
| 2 | height | pixels, big-endian |
| 1 | minor version | 26 in 2005; DjVuLibre writes 24–26 |
| 1 | major version | 0 |
| 2 | dpi | **little-endian**, unlike every other field |
| 1 | gamma × 10 | 22 means 2.2 |
| 1 | flags | low 3 bits: rotation |

Rotation flags and the matching PDF `/Rotate` (clockwise):

| Flags & 7 | Meaning | /Rotate |
|:---|:---|:---|
| 1 | upright | 0 |
| 6 | 90° counter-clockwise | 270 |
| 2 | 180° | 180 |
| 5 | 90° clockwise | 90 |

The stored image is always unrotated; the flag says how to show it. Old files
may have a shorter INFO; extra bytes are ignored. `djvu.ParseInfo` uses 300
dpi when the field is missing or outside 25–6000, and clamps gamma to
0.3–5.0. The page size in PDF points is width × 72 / dpi.

The minor version tracks the format's history (8.3.11): 20 is the first
DjVu 3, 21 adds the new indirect format and text, 22 orientation and colour
JB2, 25 NAVM, 26 text and line annotations.

## 5. The ZP coder

All the compressed data except JPEG and MMR goes through the ZP coder
(Appendix 3): an adaptive binary arithmetic coder, close to the QM coder of
JPEG. `internal/zp` implements the decoder.

The decoder has two 16-bit registers, A (the interval) and C (the code).
C starts as the first two bytes of the stream. A *context* is one byte, a
probability state from 0 to 250; all contexts start at 0. The MPS (more
probable bit) of a state is its low bit. The state table (Table 9) gives four
values for each state: Δ (the LPS probability), θ (an adaptation threshold),
μ (the next state after an MPS) and λ (the next state after an LPS).

![One decode step. A bit is usually decided by the fast path without
touching the state. Otherwise the coder compares C with Z, adapts the state,
and renormalises.](figures/djvu_zp.svg)

There is also a *pass-through* mode: a bit with probability 1/2 and no
context. It is the same step with a fixed Z and no adaptation. BZZ uses
Z = 0x8000 + A/2; IW44 uses Z = 0x8000 + 3A/8 (*checked*). When the decoder runs
past the end of the data, it reads 1 bits (12.1).

The spec's decoder pseudo-code (Figures 1 and 2) and its state table are
wrong in places; the corrected rules are in the figure above and in
[README.md](README.md). The decoder was checked three ways: on streams from
DjVuLibre's `bzz`, against test vectors from the MIT-licensed `djvu-zp`
encoder, and against that crate's state table.

## 6. BZZ

BZZ is a general-purpose compressor (Appendix 4) used for DIRM, NAVM, ANTz,
TXTz and the FGbz indices. It is a Burrows–Wheeler transform, then
move-to-front coding, then ZP coding of the ranks. `internal/bzz` implements
it.

![BZZ decoding. One ZP stream covers the whole chunk. Each block has a
header, then ranks that the move-to-front list turns into bytes, then the
inverse BWT.](figures/djvu_bzz.svg)

The stream is a sequence of blocks, all in one ZP stream:

1. **Block size**: 24 bits in pass-through mode, MSB first. 0 ends the
   stream. The size counts one extra symbol, the end-of-block (EOB) marker.
2. **Speed**: one pass-through bit; if it is 1, a second one. The speed is 0,
   1 or 2 (bits 0, 10, 11). The spec reads it as 2 bits, wrongly.
3. **Ranks**: one per symbol, each coded with the 262 contexts of the figure.
   The contexts are kept across blocks. Rank 256 is the EOB marker, which
   must occur exactly once per block.
4. **Move-to-front**: the rank picks a byte from a 256-entry list. The list
   is not plain MTF. The first four entries carry frequency estimates. A
   decoded byte moves up to position 3, then on towards the front past each
   entry whose estimate is not above its own. The estimates grow by
   `fadd += fadd >> speed` and are scaled down when they overflow.
5. **Inverse BWT**: count each byte, number the occurrences of each byte in
   order, then walk backwards from the EOB position. A block of size n
   yields n − 1 bytes.

In DIRM and FGbz the BZZ stream follows a raw header; in NAVM, ANTz and TXTz
it is the whole chunk. Decoding is checked on DjVuLibre's `bzz -e` output
(single and multi-block) and on every BZZ chunk in the corpus.

## 7. DIRM

DIRM has a raw header and a BZZ body (8.3.2):

| Bytes | Field |
|:---|:---|
| 1 | bit 7: bundled; bits 0–6: version, 1 |
| 2 | number of components n, big-endian |
| 4 × n | *bundled only*: offset of each component's FORM, from the start of the file (counting AT&T) |
| rest | BZZ body |

The BZZ body:

| Bytes | Field |
|:---|:---|
| 3 × n | size of each component, big-endian; may be 0 in indirect documents |
| 1 × n | flags: bit 7 has-name, bit 6 has-title, bits 0–5 type |
| … | per component: id, then name if has-name, then title if has-title; each NUL-terminated UTF-8 |

The type is 0 for an included DJVI, 1 for a page, 2 for thumbnails, and 3
for a DJVI of shared annotations. The spec lists only 0–2; djvudump prints
3 as `[S]` (*checked*, `bundled.djvu`). Name and title default to the id. In
an indirect document, the name (or the id) is the file name. A title is a
page label, such as `iv`, worth keeping as a PDF page label.

## 8. JB2: the mask

Sjbz holds the mask, coded with JB2 (Appendix 2). The whole chunk is one ZP
stream. JB2 is a *symbol* coder: it codes each distinct shape once, then
places copies. `internal/jb2` keeps this structure instead of only drawing
pixels, because the same structure becomes a JBIG2 symbol dictionary and
text region in the PDF. That is what keeps the mask as small as in DjVu.

### Records

The stream is a sequence of records. Each starts with a record type, coded
as an integer (11.2.3, Table 6):

| Type | Record | Fields after the type |
|:---|:---|:---|
| 0 | start of image | width, height; refinement flag (always 0) |
| 1 | new shape, to library and page | size, direct bitmap, position |
| 2 | new shape, to library only | size, direct bitmap |
| 3 | new shape, to page only | size, direct bitmap, position |
| 4 | refined shape, to library and page | match index, size change, refined bitmap, position |
| 5 | refined shape, to library only | match index, size change, refined bitmap |
| 6 | refined shape, to page only | match index, size change, refined bitmap, position |
| 7 | copy of a library shape to page | match index, position |
| 8 | non-symbol bitmap, to page only | size, direct bitmap, absolute position |
| 9 | before type 0: shared dictionary; after: reset | dictionary size, or nothing |
| 10 | comment | length, bytes |
| 11 | end of data | — |

The **library** is a list of shapes, numbered from 0 in the order they are
added. Before a shape goes into the library, its empty edge rows and columns
are removed (11.5.10). The **page** starts white; each placed bitmap is ORed
onto it (11.3).

![The records of a shared dictionary and of a page. Djbz fills the library
with shapes #0 and #1. The page's record 9 says it needs those 2 shapes; its
own new shapes get numbers from 2 on.](figures/djvu_jb2.svg)

### Shared dictionaries

A record 9 *before* the start record says the page needs a dictionary of
that many shapes (11.4.10.1). The dictionary is a Djbz chunk in the page or,
usually, in a DJVI reached by INCL. The decoder loads its shapes first, as
library entries 0 to size − 1. A Djbz is itself a JB2 stream that uses only
records 9, 0, 2, 5, 10 and 11 (11.4.10.3). Its own record 9 would include
yet another dictionary.

A record 9 *after* the start record is a reset: forget all the integer
contexts (below), and go on (11.4.10.2). Encoders insert it when there are
too many contexts.

### Integers

Every number is coded with a binary tree of ZP contexts (11.2.2), one tree
per kind of number (Table 7): record type, image size, match index, shape
width and height, width and height change, absolute column and row, the four
relative offsets (same line or new line, column or row), comment length,
comment byte and dictionary size. Each node of a tree has its own context,
created when first used. To decode a value n in a known range [low, high]:

1. **Sign**: one decision, 1 for n ≥ 0. Skipped when the range decides it.
2. **Magnitude class**: v = n for n ≥ 0, −n − 1 for n < 0. Test the ranges 0,
   1–2, 3–6, 7–14, …, [2^k − 1, 2^(k+1) − 2] in turn; a **0** means v is in
   this one, a 1 means go on. The spec (11.2.5.2) says the opposite; streams
   decode only this way (*checked*).
3. **Value**: the bits of v within its range, MSB first, down a complete
   binary subtree.
4. Return v, or −v − 1 if negative.

A decision the range already decides is not coded, but the walk still steps
to that child node. So a node is fixed by the path of decisions from the
root, coded or not, and each node has its own context. The phase 3 subtree
spans the whole range [2^k − 1, 2^(k+1) − 2], even where [low, high] cuts it
short; its midpoints do not depend on [low, high] (*checked*).

A record 9 after the start record resets all the trees. `internal/jb2`
implements it, and records 3, 6, 8 and 10, from the spec alone: neither
cjb2, minidjvu nor pdf2djvu writes them, even for a 12000 × 12000 page of
50,000 random shapes.

### Positions

Coordinates are pixels, with column 1 at the left and row 1 at the **bottom**
of the image (11.3). A shape is placed in one of three ways:

- **absolute** (records of type 8): column of the left edge, row of the top
  edge;
- **new line** (offset flag 1): relative to the *first* shape of the current
  line. The left edge is offset from that shape's left edge; the top edge
  from its bottom edge. The shape becomes the first of a new line.
- **same line** (offset flag 0): relative to the *previous* shape. The left
  edge is offset from its right edge; the bottom edge from the line's
  baseline. The baseline is the median of the bottom edges of the last three
  shapes on the line, or the first shape's bottom if there are fewer.

The first shape of the image is placed as a new line, relative to a
pretend shape with its bottom edge on the top row (row = height) and its
left edge on column **0**, one left of the image. The spec (11.5.8) puts it
on column 1; every mask then decodes one pixel too far right (*checked*).
The previous shape's right edge is its left column + width − 1. Library
copies place the library shape, which has its empty edges removed.

### Bitmaps

A bitmap is coded row by row from the top, pixel by pixel from the left; 1
is black (11.5.9). Each pixel uses a context made from nearby pixels already
decoded:

![The two pixel templates. Direct coding uses 10 neighbours in the same
bitmap. Refinement uses 4 from the new bitmap and 7 from the matching library
shape, centred on the pixel being coded.](figures/djvu_jb2tpl.svg)

- **Direct coding**: 10 neighbours, 1024 contexts. Pixels outside the bitmap
  are white.
- **Refinement**: for a shape close to a library shape. 4 neighbours from the
  new bitmap and 7 from the library shape, aligned by their centres; 2048
  contexts. For an even size, the centre is the left (or lower) of the two
  middle pixels.

The spec does not give the order of the bits in the context number. The pixel
numbered 1 in the figure is the most significant bit, 10 (or 11) the least
(*checked*). Both pixel
templates are close to JBIG2's generic and refinement templates, but not the
same, so dejapdf decodes JB2 bitmaps and re-encodes them as JBIG2.

Decoded this way, every mask of the corpus and all 2420 masked pages of 8
books are pixel-identical to `ddjvu -mode=mask` (*checked*).

## 9. IW44: background and foreground

BG44, FG44 and TH44 chunks hold images coded with IW44 (Appendix 1), a
wavelet coder in the style of SPIHT/EZW. The image is transformed into one
wavelet coefficient per pixel, and the coefficients are sent bit plane by bit
plane, most significant first. Each extra chunk sharpens the image.
`internal/iw44` decodes to pixels; dejapdf then re-encodes as JPEG 2000 at
the same resolution.

The spec gets several details of IW44 wrong (marked *erratum* below),
found by testing against ddjvu, with the MIT-licensed djvu-rs decoder
(`crates/djvu-iw44`) as a reference; no code is copied.
Decoded this way, every layer of the corpus and all 289 backgrounds of 8
books are identical to ddjvu's (*checked*).

### Chunk header

Each chunk starts with a header (10.3). The first chunk of a layer (serial 0)
has 9 bytes; later chunks have only the first 2:

| Bytes | Field |
|:---|:---|
| 1 | serial number: 0, 1, 2, … within the layer |
| 1 | number of slices in this chunk |
| 1 | bit 7: 1 = grayscale (Y only), 0 = colour; bits 0–6: major version, 1 |
| 1 | minor version, 2 |
| 2 | width, big-endian |
| 2 | height, big-endian |
| 1 | bits 0–6: chrominance delay; bit 7: 0 = chroma at half resolution |

The corpus `photo.djvu` starts `00 4A 01 02 03 52 04 4C 8A`: serial 0, 74
slices, colour, version 1.2, 850×1100, delay 10. The gray version has `81`
in the third byte and a delay of 0 (*checked*).

Several BG44 chunks continue one decoding: nothing is reset between them
except the ZP coder itself, which restarts on each chunk's data, keeping its
contexts (10.2). FG44 is a separate layer with its own state.
Past the end of a chunk's data the ZP coder reads 0xFF bytes; encoders
rely on it, so this is not an error.

Bit 7 of the delay byte is not ignored (*erratum*): when it is 0 (`c44
-crcbhalf`; minor version ≥ 2), the chroma planes are reconstructed only
down to scale 2 (below), and each value covers a 2×2 pixel block, rows
paired from the bottom.

### Structure

The image is cut into 32×32 **blocks**, numbered from the lower left, row by
row upwards (10.4.1); edge blocks may be partial. Each block has 1024
coefficients per colour component (Y, Cb, Cr), grouped in 64 **buckets** of
16, and in 10 **bands**:

![Which band each position of a block belongs to. Band 0 is a coarse 4×4
grid; bands 1–3, 4–6 and 7–9 add detail at steps 8, 4 and
2.](figures/djvu_iw44.svg)

Coefficient index i (bits b9…b0) lands at row b1b3b5b7b9 and column
b0b2b4b6b8 of the block, read MSB first (10.6.3). This interleaving puts the
coarse coefficients at the multiples of 8, so the inverse transform works in
place.

The data is a series of **slices**. A slice codes one band for every block,
for Y, then Cb, then Cr. The band number goes 0, 1, …, 9, then back to 0.
While the **chrominance delay** counter is above 0, slices code only Y and
the counter drops by 1; grayscale chunks code only Y (10.4.1.1).

### Coefficient decoding

Each coefficient has a value a and a step size S; S comes from one of 16
step-size table entries (Table 3), with fixed starting values (Table 4) for
each component. At the end of each slice, the steps used by its band are
halved (Table 5; a step of 1 becomes 0). A coefficient is *active* when a ≠ 0
and 0 < S < 0x8000, and *potential* when a = 0 and 0 < S < 0x8000. Buckets
and the whole block band take the OR of these flags.

A block band is decoded in four passes (10.5):

1. **Block**: if the band has 16 buckets, none active and some potential,
   decode "any new coefficients?" (1 context).
2. **Bucket**: for each potential bucket b, decode "any new ones here?" (8
   contexts per band: the block band's active flag, and how many of
   coefficients 4b to 4b+3, b's parents, are **non-zero**, at most 3; always
   0 in band 0). The spec says zero ones (*erratum*).
3. **New coefficients**: in a flagged bucket, for each potential coefficient,
   decode "active now?" (16 contexts: the **bucket's** active flag, not the
   block's (*erratum*), and min(7, n), where n counts the bucket's potential
   coefficients, drops by 1 after each one, and resets to 0 after a hit). If
   yes, the sign comes in IW44 pass-through mode (1 = negative), and
   a = ±(S + S/2 − S/8), 11S/8, a little below the interval's midpoint 3S/2
   (*erratum*).
4. **Refinement**: for each coefficient active before this band, decode one
   more bit. While |a| ≤ 3S (the first refinement) the bit uses 1 context
   and |a| first gains S/4, reaching the midpoint; later bits are IW44
   pass-through. Then |a| goes up by S/2, or down by S − S/2 (*erratum*:
   the spec has only ±S/2).

A component thus has 1 + 80 + 16 + 1 = 98 contexts, all starting at 0.
Coefficients are 16-bit fixed point with 6 fraction bits; like DjVuLibre,
dejapdf keeps them, and every transform sample, in 16 bits.

### Reconstruction

After the last chunk (or any chunk, for a preview) (10.6):

1. Put each block's coefficients in place, as in the figure.
2. **Inverse wavelet**: for s = 16, 8, 4, 2, 1, transform the columns, then
   the rows, using only samples at multiples of s; call them c[0..kmax].
   Each pass is a lifting step on the even samples, then a prediction step
   on the odd ones (`>>` rounds down):

   ```
   even k:  c[k] -= (9*(c[k-1] + c[k+1]) - (c[k-3] + c[k+3]) + 16) >> 5
   odd k:   c[k] += (9*(c[k-1] + c[k+1]) - (c[k-3] + c[k+3]) + 8) >> 4
   ```

   Missing neighbours count as 0 in the lifting step, with one quirk in
   the row pass (*erratum*): its neighbour registers keep rolling, so at
   k = 2 and k = 4 a missing c[k+3] repeats c[k+1], and the following
   step sees that value again. Near the ends, the prediction step falls
   back to c[k] += (c[k-1] + c[k+1] + 1) >> 1, or to c[k] += c[k-1] at the
   last sample. The spec prints the full prediction step like the lifting
   step, with − and /32 (*erratum*); the formula above is the (4,4) filter
   (*checked*). The transform covers the image, not the padded blocks.
3. **Precision**: round away the 6 fraction bits, clamp to −128…127; add 128
   to Y. For grayscale, the value is 127 − V.
4. **Colour** (10.6.7, with DjVuLibre's rounding): t2 = Cr + (Cr >> 1),
   t3 = Y + 128 − (Cb >> 2); R = Y + 128 + t2, G = t3 − (t2 >> 1),
   B = t3 + 2 Cb, each clamped to 0–255.

Pixels of partial edge blocks that fall outside the image are dropped. The
reference is `ddjvu -mode=background -aspect=no -size=W×H` at the layer's
own size; for FG44, rewrap the chunk as the BG44 of a page its size
(`djvuextract`, `djvumake`), since `-mode=foreground` paints through the
mask.

## 10. Foreground colour

With FG44, the foreground is an ordinary IW44 image, usually at 1/12 scale,
and gives the colour of each mask pixel under it. ddjvu upscales FG44 by
pixel replication (*checked*: identical on every mask pixel). How it is
painted in PDF: [PDF article §8](Portable_Document_Format.md#8-masks).

With FGbz, every *blit* of the mask (every shape placed on the page) gets one
colour from a palette (8.3.10). The layout (*checked*, `palette.djvu`):

| Bytes | Field |
|:---|:---|
| 1 | bit 7: indices present; bits 0–6: version, 0 |
| 2 | palette size p, big-endian |
| 3 × p | colours, **B, G, R** order |
| 3 | number of blits b, big-endian (only if bit 7) |
| rest | BZZ: b big-endian 2-byte palette indices (only if bit 7) |

The spec suggests that the blit count is inside the BZZ data; it is not.
`palette.djvu` has 4 colours, 2055 blits, and 4110 bytes of BZZ output.
Index k colours the k-th shape placed on the page, in record order, empty
shapes included (*checked*: the page renders within 1 level of ddjvu, and
FGbz pages of the books identical). Without indices (bit 7 clear) every
shape takes colour 0; scanned books have many such one-colour palettes.
In PDF, each palette colour becomes a stencil of its blits ([PDF article
§9](Portable_Document_Format.md#9-a-djvu-page-in-pdf)).

## 11. Alternative encodings

**BGjp, FGjp** hold a plain JPEG/JFIF file (8.3.13–14). It goes into the PDF
unchanged as a DCTDecode image (*checked*: `jpeg.djvu`, made with
`djvumake` from Ghostscript JPEGs).

**Smmr** holds a G4 (T.6) fax mask (8.3.15):

| Bytes | Field |
|:---|:---|
| 3 | `MMR` |
| 1 | bit 0: reverse video, like TIFF min-is-black; bit 1: striped |
| 2 | width, big-endian |
| 2 | height, big-endian |
| … | if striped: rows per stripe (2 bytes), then for each stripe a 4-byte length and its G4 data; else one G4 stream |

Stripes run from the top, the last one shorter, and each is an
independent G4 stream: the reference line starts white again (*checked*:
the strips of a multi-strip Ghostscript `tiffg4` file, behind this header,
decode identical to the original page both in dejapdf and in ddjvu). The
spec's "rows per stripe" field is 2 bytes and each stripe length 4, both
big-endian.

dejapdf decodes Smmr with the G4 decoder of `golang.org/x/image/ccitt`,
copied into `internal/ccitt` (BSD), into a bitmap that becomes a mask of one
blit. From there it is handled like a JB2 mask, with FGbz, FG44 and page
selection. Corpus files: `mmr.djvu` (one G4 stream), `mmr-striped.djvu`
(132 stripes of 25 rows) and `mmr-compound.djvu` (G4 mask, FG44 and BG44).
All three masks are identical to `ddjvu -mode=mask`.

djvulibre's own decoder fails ("Invalid G4/MMR Data") on striped halftone
pages when the stripes are under about 100 rows, though the data decodes
correctly here. The corpus uses a text page for its stripes.

## 12. Hidden text

TXTz holds the page's text and its layout, BZZ-compressed (8.3.5). TXTa is
the same data uncompressed. The decoded data:

| Bytes | Field |
|:---|:---|
| 3 | text length in bytes, big-endian |
| that many | the page text, UTF-8 |
| 1 | version, 1 |
| … | the zone tree: one page zone and its descendants, in preorder |

Each zone is a 17-byte record:

| Bytes | Field |
|:---|:---|
| 1 | type: 1 page, 2 column, 3 region, 4 paragraph, 5 line, 6 word, 7 character |
| 2 | x |
| 2 | y |
| 2 | width |
| 2 | height |
| 2 | text start |
| 3 | text length, in bytes |
| 3 | number of children |

The five 2-byte fields hold value + 0x8000, so they may be negative. The
text of a zone is the slice [start, start + length) of the page text. Its
start is coded as a gap after the end of the previous sibling's text, or
after the parent's start for a first child. The spec calls this field
unused, but it carries the same 0x8000 bias. In every file we have, the gap
is 0. The text between zones may hold separators: space, newline, and the
control characters 0x0B (VT), 0x1D, 0x1E, 0x1F. They mark lines, paragraphs,
regions and columns.

Coordinates are pixels, with the origin at the bottom left of the
**unrotated** image. Each zone is stored relative to an earlier one:

![The three placement rules. A first child is placed from its parent's
upper left. A later page, paragraph or line is placed from the previous
sibling's lower left. A later column, region, word or character is placed
from the previous sibling's lower right.](figures/djvu_text.svg)

- **first child**: (x, y) goes from the parent's upper-left corner to the
  zone's upper-left corner, x to the right, y **down**;
- **later page, paragraph, line**: from the previous sibling's lower-left
  corner to this zone's upper-left corner, y down;
- **later column, region, word, character**: from the previous sibling's
  lower-right corner to this zone's lower-left corner, y **up**.

The spec leaves region out of the last rule. A test file with all seven zone
types, made by `djvused set-txt`, decodes only with region there. Decoded
this way, every zone of the corpus and of several books (Latin and Cyrillic)
matches `djvused print-txt` exactly (*checked*, 3,500 zones).

In the PDF, each word becomes invisible text sized to its box
([PDF article §12](Portable_Document_Format.md#12-invisible-text)).

## 13. Annotations

ANTz holds annotations as BZZ-compressed text (8.3.4); ANTa is the same
uncompressed. A page's annotations are all its ANTa/ANTz chunks, its own and
included ones, concatenated. The text is a list of S-expressions:

```
(background #FFFFFF)
(zoom page)
(mode bw)
(maparea "https://example.com/" "Example link" (rect 300 230 700 70) (none))
(maparea "#3" "Go to page 3" (rect 1250 230 420 70) (none))
(metadata (Title "dejapdf test corpus"))
```

Strings are in double quotes, UTF-8. Real files use C escapes such as `\"`
and `\n` (*checked*: an XMP packet in a book); the spec only mentions `\"`.
Unknown or broken expressions are ignored.

| Expression | Meaning | dejapdf |
|:---|:---|:---|
| `(background #RRGGBB)` | viewer background (8.3.4.1.1) | ignored |
| `(zoom page)`, `(zoom d300)` | initial zoom: stretch, one2one, width, page, or d1–d999 | ignored |
| `(mode bw)` | initial layer: color, bw, fore, black | ignored |
| `(align h v)` | page alignment | ignored |
| `(maparea url comment area options…)` | a link or highlight (8.3.4.2) | Link annotation |
| `(phead …)`, `(pfoot …)` | printed headers and footers (8.3.4.3) | ignored |
| `(metadata (Key "value") …)` | document info; DjVuLibre extension | Info dictionary |
| `(xmp "…")` | an XMP packet; DjVuLibre extension | /Metadata stream |

A **maparea** has:

- a **url**: a string, or `(url "href" "target")`. `#` followed by a page
  number or a component id is a link inside the document, as in `#3`,
  `#p0021.djvu`, or the relative `#+1` and `#-1`;
- a **comment**: a tooltip;
- an **area**: `(rect x y w h)`, `(oval x y w h)`, `(text x y w h)`, `(poly x0
  y0 x1 y1 …)` or `(line x0 y0 x1 y1)`. These are pixels with the origin at
  the bottom left; rect, oval and text give the lower-left corner and the
  size;
- **options**: border `(none)`, `(xor)`, `(border #RRGGBB)`,
  `(shadow_in n)`…; `(border_avis)`; `(hilite #RRGGBB)` and `(opacity n)`;
  for lines `(arrow)`, `(width n)`, `(lineclr c)`; for text boxes `(backclr
  c)`, `(textclr c)`, `(pushpin)`.

The spec says maparea coordinates are on the *rotated* page (8.3.4.2.3).
DjVuLibre does not rewrite them when the rotation changes (`djvused
set-rotation` leaves ANTz alone, *checked*), so dejapdf treats them like the
text: unrotated, with /Rotate applied to the whole page.

Annotations shared by all pages sit in a DJVI of type 3 (§7), reached by
INCL. In real books they usually hold `metadata` and `xmp`.

## 14. Outline

NAVM is the document outline (8.3.3), BZZ-compressed. It must come right
after DIRM. The decoded data:

| Bytes | Field |
|:---|:---|
| 2 | total number of bookmarks, big-endian |
| … | the top-level bookmarks, each followed by its children (preorder) |

Each bookmark:

| Bytes | Field |
|:---|:---|
| 1 | number of children |
| 3 | title length, big-endian |
| that many | title, UTF-8 |
| 3 | URL length |
| that many | URL, UTF-8, the same forms as a maparea url |

The corpus outline (*checked* against `djvused print-outline`) starts
`00 05` (5 bookmarks), then `00` (no children), `00 00 04 Text`, `00 00 02
#1`. The URL is usually `#` plus a page number (1-based) or a component id;
DIRM maps an id to its page. A bookmark with an empty URL is a heading
without a target.

## 15. Checking it

| Tool | Checks |
|:---|:---|
| `djvudump` | the chunk tree; `dejapdf -dump` prints the same text |
| `djvused -e 'print-txt'`, `print-ant`, `print-outline` | TXTz, ANTz, NAVM (selected page with `select N`) |
| `djvused -e 'set-txt FILE' -s` | makes test files with chosen zones |
| `ddjvu -mode=mask` | the JB2 mask, pixel for pixel |
| `ddjvu -mode=background`, `-mode=foreground` | the IW44 layers |
| `djvuextract` | raw chunk data |
| `bzz -e`, `bzz -d` | BZZ streams |
| `djvu-zp` (Rust, MIT) | ZP test vectors, in `internal/zp/testdata` |

The spec errata, collected in [README.md](README.md): the ZP
state table and decoder figures, the BZZ speed field, the DIRM type 3, the
FGbz blit count, the TXTz text start and region placement, the ANTz
escapes, three JB2 details (the phase-2 decision, the first shape's
column, the context bit order), and the IW44 coefficient values, contexts,
row-transform quirk, chroma resolution bit and colour rounding.
