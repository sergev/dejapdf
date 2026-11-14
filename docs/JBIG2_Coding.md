# JBIG2 Coding

What dejapdf's JBIG2 encoder must produce: the segments, the MQ arithmetic
coder, the integer coders, generic regions, symbol dictionaries and text
regions, and how they go into a PDF. Numbers in parentheses are sections of
ITU-T T.88 (02/2000), [T-REC-T.88-200002.pdf](T-REC-T.88-200002.pdf).
T.88 defines only the decoder; an encoder is right when that decoder gets
back the same bits. A claim marked *checked* was tested with a throwaway Python
encoder: its output decoded pixel-exact with `jbig2dec`, and inside a PDF with
MuPDF and Poppler. The DjVu side is in [DjVu_Format.md](DjVu_Format.md), the
PDF side in [Portable_Document_Format.md](Portable_Document_Format.md).

JBIG2 can code a page in many ways. dejapdf uses one small subset, chosen to
carry DjVu's JB2 structure over unchanged:

| JBIG2 feature | dejapdf |
|:---|:---|
| arithmetic coding (MQ) | yes, everywhere |
| Huffman coding, MMR | no |
| generic region | yes: non-symbol bitmaps, and every symbol bitmap |
| symbol dictionary | yes: one shared (Djbz) and one per page |
| text region | yes: the blits of a page |
| refinement, aggregation | not at first (§8) |
| halftone, pattern dictionary | no |
| striping, end-of-stripe, unknown page height | no |

And the mapping from JB2:

| DjVu JB2 | JBIG2 |
|:---|:---|
| Djbz shape dictionary | symbol dictionary in the PDF's JBIG2Globals |
| shapes added to the library by the page (records 1, 2, 4, 5) | the page's own symbol dictionary |
| blit of a library shape (records 1, 4, 7) | a symbol instance in a text region |
| shape placed on the page only (records 3, 6) | a symbol, or a generic region |
| non-symbol bitmap (record 8) | a generic region |
| refined shape | its decoded bitmap, as a plain symbol |

## 1. The model

A JBIG2 stream describes **pages** (bilevel bitmaps). It is a sequence of
**segments**. Each segment has a header and a data part (7.1). Some segments
define things for later use, such as a symbol dictionary. Others are
**regions**: they decode a bitmap and draw it onto the page buffer at some
position with a **combination operator** (OR, AND, XOR, XNOR, REPLACE)
(8.2). The page buffer starts filled with the page's default pixel.

A pixel value of 1 is black (5.6). A decoder outputs 1 for black, which a PDF
JBIG2Decode filter turns into sample 0. A stencil mask paints where the
sample is 0, so a plain `/ImageMask true` paints the black pixels; no
`/Decode` is needed (*checked*).

T.88 defines three file organisations (Annex D). PDF uses the **embedded**
one (D.3): bare segments, no file header, in an order the container chooses.
For testing with `jbig2dec`, the same segments get a 13-byte file header in
front (D.4): `97 4A 42 32 0D 0A 1A 0A` (`\x97JB2\r\n\x1a\n`), then a flags
byte `01` (sequential, page count known), then the page count (4 bytes).

## 2. Segments

The segment header (7.2):

![The header of a text region segment that refers to two symbol
dictionaries, and the region info that starts every region's data
part.](figures/jbig2_segment.svg)

| Bytes | Field |
|:---|:---|
| 4 | segment number |
| 1 | flags: bits 0–5 type; bit 6: page field is 4 bytes; bit 7: deferred non-retain (0) |
| 1 | bits 5–7: number of referred-to segments (0–4); bit 0: this segment is referred to later; bits 1–4: the k-th referred segment is used again later |
| 1, 2 or 4 each | referred-to segment numbers: 1 byte while this segment's number ≤ 256, 2 while ≤ 65536, else 4 |
| 1 or 4 | page association: 0 = no page (globals) |
| 4 | data length |

More than four references take a long form (7.2.4): the count as 4 bytes with
the top 3 bits set to 7, then retain bits, one per reference rounded up to
bytes. A segment may refer only to lower-numbered segments. The retain bits
only let a decoder free memory early; viewers ignore them (*checked*: the PDF
spec's own example sets them all to 0).

The segment types dejapdf writes (7.3):

| Type | Segment | Use |
|:---|:---|:---|
| 0 | symbol dictionary | Djbz shapes, page shapes |
| 6 | immediate text region | the blits of a page |
| 38 | immediate generic region | a non-symbol bitmap |
| 48 | page information | first segment of every page |

Types 7 and 39 are the "lossless" variants of 6 and 38. They code the same
data, and only claim that the region is exact. Type 49, end of page, is not
used in PDF.

Rules (7.3.1–7.3.2): a text region may refer to any number of symbol
dictionaries; a generic region refers to none. Every region and page
information segment belongs to a page. A segment of page 0 may refer only to
page-0 segments. The first segment of a page must be its page information.

**Region info.** Every region's data part starts with 17 bytes (7.4.1): width,
height, x and y of the top-left corner on the page (4 bytes each), and a flags
byte whose bits 0–2 are the combination operator (0 = OR).

## 3. Page information

Segment type 48, 19 bytes (7.4.8):

| Bytes | Field | dejapdf |
|:---|:---|:---|
| 4 | width | page width in pixels |
| 4 | height | page height |
| 4 | x resolution, pixels per metre | dpi × 10000 / 254, or 0 |
| 4 | y resolution | the same |
| 1 | flags: bit 0 lossless; bit 1 may have refinements; bit 2 default pixel; bits 3–4 default operator; bit 5 needs auxiliary buffers; bit 6 operator overridden | 0: white page, OR |
| 2 | striping: bit 15 striped, bits 0–14 max stripe height | 0 |

## 4. The MQ arithmetic coder

All the data dejapdf writes is MQ-coded (Annex E). MQ is the adaptive binary
arithmetic coder of JPEG and JBIG, a cousin of DjVu's ZP coder. It has 47
probability states instead of ZP's 251, and the bits it writes are different,
so DjVu data cannot be transcoded. It has to be decoded and coded again.

Each **context** CX holds a state index I(CX) (0–46) and a more-probable value
MPS(CX). Both start at 0 (E.2.8). Table E.1 gives, for each state, the LPS
probability Qe, the next state after an MPS (NMPS) and after an LPS (NLPS),
and SWITCH, which flips the MPS after an LPS.

![The MQ encoder. Each decision is coded as the MPS or the LPS; when the
interval drops below 0x8000 the coder renormalises, and every 8 shifts a byte
goes out.](figures/jbig2_mq.svg)

Points that matter for bit-exact output:

- **Conditional exchange**: when the MPS sub-interval would be smaller than
  Qe, the coder swaps the two sub-intervals (E.2.4).
- **Bit stuffing**: after an 0xFF byte, the next byte carries only 7 bits.
  This keeps a carry from rippling into bytes already written, and keeps
  0xFF followed by a byte above 0x8F free for markers (E.2.7).
- **INITENC**: A = 0x8000, C = 0, CT = 12. The byte before the output buffer
  counts as 0x00 (E.2.8).
- **FLUSH**: fills the low bits of C with 1s within the interval, writes two
  more bytes, then the marker `FF AC`. If the last byte is already 0xFF, it
  serves as the marker's first byte (E.2.9).
- Trailing `FF 7F` pairs before the marker may be dropped (E.2.10). Dropping
  the marker itself still decodes, but `jbig2dec` warns about it
  (*checked*); dejapdf keeps it, as T.88's own test output does.

*Checked*: `internal/mq` encodes the test sequence of Annex H.2 (32 bytes of
decisions, one context) into exactly the 30 bytes given there
(`TestAnnexH2`). The same encoder serves JPEG 2000, which differs only in
the flush: no marker, and a final 0xFF is dropped (T.800 C.2.9).

Contexts are reset (all I and MPS to 0) at the start of every generic
region, text region and symbol dictionary segment (E.3.7). The exception is
a symbol dictionary that sets "bitmap coding context used": it continues the
bitmap contexts of the last dictionary it refers to (7.4.2.2). dejapdf does
not use that.

## 5. Integer coding

Numbers in symbol dictionaries and text regions are coded as a string of MQ
decisions, one context set per kind of number (Annex A). There are 13 kinds:
IADH, IADW, IAEX, IAAI (dictionaries), IADT, IAFS, IADS, IAIT, IARI, IARDW,
IARDH, IARDX, IARDY (text regions), plus IAID for symbol IDs.

All but IAID use one code (Table A.1): a sign bit, a unary prefix choosing a
range, then the offset in the range, MSB first.

| Value | Bits after the sign | Value bits |
|:---|:---|:---|
| 0–3 | `0` | 2 |
| 4–19 | `10` | 4 |
| 20–83 | `110` | 6 |
| 84–339 | `1110` | 8 |
| 340–4435 | `11110` | 12 |
| 4436 and up | `11111` | 32 |
| OOB | sign 1, then `000` (that is, "−0") | — |

Negative values use the same ranges for −v, with sign 1; −0 is the
out-of-band value **OOB**, which ends a list. The context of each bit is
PREV: a register that starts at 1 and shifts in each bit. Once PREV reaches
256, only its low 8 bits plus a leading 1 are kept: PREV = ((PREV << 1 | bit)
& 511) | 256. So each kind needs 512 contexts (A.2).

**IAID** (A.3) codes a symbol number in exactly SBSYMCODELEN bits, MSB first,
with PREV = 1, then (PREV << 1 | bit), never truncated: 2^SBSYMCODELEN
contexts. In a text region, SBSYMCODELEN = ⌈log2 SBNUMSYMS⌉, where SBNUMSYMS
counts the symbols of all the dictionaries it uses. With one symbol, the ID
takes 0 bits (*checked*).

## 6. Generic region

A generic region codes a bitmap pixel by pixel, rows top to bottom, left to
right (6.2.5). Each pixel's context is formed from already-coded neighbours
under a **template**; pixels outside the bitmap count as 0:

![The four generic templates. A1–A4 are the adaptive (AT) pixels at their
nominal places; an encoder may move them within 128 pixels to the left, 127
to the right and 128 rows up.](figures/jbig2_gb.svg)

| GBTEMPLATE | Pixels | AT pixels, nominal (x, y) |
|:---|:---|:---|
| 0 | 16 | (3, −1), (−3, −1), (2, −2), (−2, −2) |
| 1 | 13 | (3, −1) |
| 2 | 10 | (2, −1) |
| 3 | 10, two rows | (2, −1) |

The order in which the pixels form the context number is not fixed (6.2.5.7):
any consistent order gives the same code, since a context is only a label
for its probability state. The prototype uses its own order and decodes
correctly (*checked*, all four templates).

**TPGDON** (typical prediction, 6.2.5.5): before each row, code one bit,
SLTP, that tells whether this row differs in "typicality" from the previous
one. A typical row is a copy of the row above and codes no pixels. SLTP uses
the context of a fixed pixel pattern (figures 8–11), the same pattern
whatever the AT positions. Cheap for blank rows; *checked* with all
templates.

The generic region segment (7.4.6): region info (17 bytes); a flags byte
(bit 0 MMR = 0, bits 1–2 GBTEMPLATE, bit 3 TPGDON); the AT positions as
signed bytes (8 for template 0, else 2); then the MQ data. dejapdf uses
template 0 with nominal AT pixels and TPGDON (`internal/jbig2`, from
`tmp/jbig2`). With the AT pixels in place, the context is three contiguous
windows: bits 0–3 the four pixels to the left, bits 4–10 the row above from
x+3 to x−3, bits 11–15 the row before that from x+2 to x−2; this is the spec's
order, so SLTP's context is 0x9B25 (*checked*: bitmaps from 1×1 to 2550×300
decode bit-exact in `jbig2dec`).

## 7. Symbol dictionary

A symbol dictionary defines a numbered list of bitmaps, the **symbols**
(6.5). Symbols of equal height form a **height class**. Each class codes its
height as a difference from the previous class, then each symbol's width as a
difference from the previous symbol in the class, followed by that symbol's
bitmap. An OOB ends the class:

![A dictionary of 8 symbols in two height classes. Each class gives its
height as a delta; each symbol gives its width as a delta, then its bitmap.
The export flags end the dictionary.](figures/jbig2_symdict.svg)

Steps (6.5.5), arithmetic variant, SDREFAGG = 0:

1. For each height class: IADH delta height; then for each symbol: IADW
   delta width and the bitmap as a generic region of that width and height
   (template SDTEMPLATE, TPGDON 0); then IADW OOB.
2. **Export flags** (6.5.10): IAEX run lengths, alternating between "not
   exported" and "exported", starting with "not exported", over the imported
   symbols followed by the new ones. A dictionary that imports nothing and
   exports all n symbols codes the two runs 0, n.

The bitmap contexts are shared by all symbols of a dictionary and are not
reset between symbols. Heights and widths may go up or down, but coding the
classes by increasing height and symbols by increasing width keeps the
deltas small. Symbol numbers follow the coding order, so the encoder must
renumber the JB2 shapes to match (*checked*).

The segment (7.4.2): flags (2 bytes: bit 0 SDHUFF = 0, bit 1 SDREFAGG = 0,
bits 10–11 SDTEMPLATE, rest 0); the AT positions (8 bytes for template 0);
SDNUMEXSYMS (4 bytes); SDNUMNEWSYMS (4 bytes); then the MQ data.

A dictionary may refer to other dictionaries. Their exported symbols become
its input symbols, numbered first. dejapdf's dictionaries import nothing.

## 8. Refinement

Refinement codes a bitmap from a similar reference bitmap (6.3). JB2 does the
same for its "matched with refinement" records, but with its own template:

![JBIG2's two refinement templates, and DjVu JB2's. DjVu's is GRTEMPLATE 1
plus one more reference pixel, so its contexts cannot be
reused.](figures/jbig2_gr.svg)

JBIG2 can refine in a text region (SBREFINE: a symbol instance is a refined
copy of a symbol) or in a dictionary (SDREFAGG: each new symbol is a
refinement or an aggregate of earlier ones). Either way the refinement data
must be coded afresh.

JB2 refines a lot in scanned books: of the shapes coded anew, 83% are
refinements in one book and 79% in another (*checked*, counting record
types). dejapdf keeps them as refinements, in a dictionary with SDREFAGG = 1
where every symbol is REFAGGNINST = 1 (6.5.8.2.2): IAAI 1, then IAID the
reference (SBSYMCODELEN = ⌈log2(inputs + new symbols)⌉ bits, the same for
the whole dictionary), IARDX and IARDY its offset, then the bitmap by generic
refinement (6.3) with GRTEMPLATE 0, AT pixels nominal at (−1, −1), no TPGRON.
The reference is an input symbol or an earlier new one, so these symbols are
coded in JB2's order, not sorted, and a height class changes whenever the
height does (IADH and IADW may be negative). Directly coded shapes stay in a
dictionary of their own (SDREFAGG = 0), which the refinement dictionary
imports and does not export.

Offsets: JB2 lays the reference over the new bitmap by their centres; the new
pixel (r, c) (rows from the top) sees the reference's (r + ⌊Hr/2⌋ − ⌊h/2⌋,
c + ⌊(Wr−1)/2⌋ − ⌊(w−1)/2⌋). JBIG2's reference pixel is (x − DX, y − DY), so
DX and DY are those two shifts negated, less the columns and rows JB2 then
trims from the new bitmap's left and top.

GRTEMPLATE 0's reference pixels include all 7 of JB2's, and it codes 1–2%
smaller than template 1 on the scanned books. A refinement dictionary
starts with cold contexts: with fewer than 100 refined shapes (a typeset
book has ~10 a page), coding them directly is smaller, and dejapdf does.

## 9. Text region

A text region draws symbol instances: (symbol ID, position) pairs (6.4). The
positions are coded relative to each other, in **strips**:

![Two strips of a text region with REFCORNER = BOTTOMLEFT. The red dots are
the reference corners. Within a strip, each instance after the first codes
its gap from the previous one's right edge.](figures/jbig2_text.svg)

S runs along a line, T across lines (TRANSPOSED = 0: S = x, T = y). Each
instance is placed by its reference corner; with BOTTOMLEFT, (S, T) is its
bottom-left pixel. Instances are grouped into strips of SBSTRIPS rows (1, 2,
4 or 8) by T, and sorted by S within a strip.

Decoding, and so what the encoder writes, in order (6.4.5):

1. IADT: the initial STRIPT, negated and multiplied by SBSTRIPS. dejapdf
   codes 0.
2. For each strip: IADT, the strip's T step in units of SBSTRIPS; STRIPT +=
   that × SBSTRIPS.
3. The first instance of a strip: IAFS, its S minus the first S of the
   previous strip (FIRSTS); CURS = FIRSTS = that S.
4. Each later instance: IADS, the gap. The decoder computes CURS = CURS +
   IADS + SBDSOFFSET. Here CURS is the previous instance's S plus its width
   minus 1, so abutting symbols code 1 − SBDSOFFSET.
5. If SBSTRIPS > 1: IAIT, the instance's T minus STRIPT.
6. IAID, the symbol number.
7. After drawing, CURS += width − 1 (for the LEFT corners).
8. IADS OOB ends the strip. The region ends after SBNUMINSTANCES instances.

*Checked* with strip sizes 1, 2, 4, 8 and SBDSOFFSET 0, 3 and −2: all decode
exactly.

The segment (7.4.3): region info (17 bytes); flags (2 bytes): bit 0 SBHUFF = 0,
bit 1 SBREFINE = 0, bits 2–3 log2 SBSTRIPS, bits 4–5 REFCORNER (0
BOTTOMLEFT, 1 TOPLEFT, 2 BOTTOMRIGHT, 3 TOPRIGHT), bit 6 TRANSPOSED, bits 7–8
SBCOMBOP (0 = OR), bit 9 SBDEFPIXEL, bits 10–14 SBDSOFFSET (signed, 5 bits),
bit 15 SBRTEMPLATE. With SBHUFF = 0 and SBREFINE = 0, no Huffman or
refinement fields follow. Then SBNUMINSTANCES (4 bytes) and the MQ data.

The symbols a text region can use are the exported symbols of the
dictionaries it refers to, concatenated **in the order of the references**
(Table 31). A region that refers to the shared dictionary (n symbols) and
then the page's dictionary uses IDs 0 to n − 1 for the shared shapes and n
onwards for the page's (*checked*).

**From JB2 blits.** JB2 counts rows from the bottom, starting at 1; JBIG2
counts from the top, starting at 0. For a page of height H, a blit whose
bottom row is b (JB2) has T = H − b (JBIG2) and S = its left column − 1. Sort
the blits by T and then S: JB2 coding goes line by line, so the strips follow
its text lines. SBDSOFFSET can be set to the most common gap. Measured on four
books: strips of 2 with SBDSOFFSET 3 are best, but every choice of 1, 2, 4,
8 and 0–3 is within 0.2% of the total, so it hardly matters.

## 10. Putting a DjVu mask into PDF

![The streams of a two-page document. The Djbz dictionary is a page-0
segment in the shared JBIG2Globals stream; each page's image stream holds its
page information, its own dictionary, a text region and any generic
regions.](figures/jbig2_pdf.svg)

- **JBIG2Globals**: the segments of page association 0, here the Djbz
  dictionary as one symbol dictionary. One globals stream can serve every
  page that includes that Djbz.
- **Image stream**: the segments of one page, each with page association 1,
  even for the 50th page (PDF 7.4.7). In order: page information, the page's
  own symbol dictionaries, the text region. Generic regions are not needed:
  JB2's non-symbol bitmaps become single-use symbols.
- **Numbers**: each image is decoded alone, with its globals. Segment numbers
  only need to be unique and increasing within globals + one stream.
- **No** file header, end-of-page, end-of-file, or the optional 0xFF 0xAA/0xAB
  markers (D.3; PDF 7.4.7).
- The image dictionary: `/ImageMask true /Width W /Height H /Filter
  /JBIG2Decode /DecodeParms << /JBIG2Globals n 0 R >>`. BitsPerComponent is
  implied by /ImageMask.

*Checked*: a PDF built this way, with a two-dictionary text region, renders
pixel-exact in `mutool draw` and `pdftoppm`, and passes `qpdf --check`, whether
the globals have page association 0 or 1.

For FGbz pages, each palette colour is a separate stencil image, so each
colour gets its own image stream (sharing one stencil among the colours by
clipping fails in MuPDF and Poppler; see Portable_Document_Format.md §9).
An Smmr mask, decoded, is one generic region (§5) the size of the stencil. Each stream holds the page information, a
text region with that colour's blits, and a dictionary of only the shapes
those blits use.

What `internal/convert` writes, for the globals and for each image stream
alike: a dictionary of the directly coded shapes, sorted by height then
width; a refinement dictionary of the refined ones (§8), when there are 100
or more; then, in an image stream, the text region referring to all of
them, the globals first. Empty shapes are left out; their blits draw nothing.
A shape is refined only from a shape the stream defines anyway; otherwise it
is coded directly, so no undrawn reference is pulled in. Symbol numbers
follow the dictionaries' export order, which is why the shapes are renumbered.

*Checked*: every corpus mask and sample pages of the books render
pixel-identical to `ddjvu -mode=mask` in MuPDF and Poppler; 14 books pass
`qpdf --check`. Masks are 1.04–1.13× the size of their JB2 data (Sjbz plus
Djbz): one generic region per page was 1.0–4.6× the whole DjVu; symbols
without refinement gave 1.04–1.23×.

## 11. Checking it

| Tool | Checks |
|:---|:---|
| `jbig2dec -t pbm -o out.pbm file.jb2` | a file with the D.4 header: pixel-exact decode |
| `mutool draw`, `pdftoppm` | the same segments inside a PDF, with JBIG2Globals |
| `qpdf --check` | PDF structure |
| `ddjvu -mode=mask` | the reference mask, to compare pixel for pixel |
| Annex H.2 test sequence | the MQ encoder, byte for byte |

The prototype that checked this article was a few hundred lines of Python,
not kept in the repository; `internal/mq` and `internal/jbig2` carry the
checks as Go tests, with `jbig2dec` as the oracle.

Generic regions alone (P8), one per mask: the books' PDFs went from 1.9–13.6×
to 1.04–4.6× the DjVu size. Symbol dictionaries, text regions and
refinement (P9) bring them to 0.79–1.08×, masks still pixel-exact.
