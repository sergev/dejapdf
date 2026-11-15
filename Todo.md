# dejapdf — Todo: PDF to PDF

Teach dejapdf to re-encode **scanned** PDFs: replace each page's scan image
with the same layers the DjVu path produces (JBIG2 text mask, JPX
background), and leave everything else in the file untouched.

Sequential plan: each task builds on the previous ones and ends with a
testable "Done when".

## Why

Measured on 20 pages of each of two scanned books in `~/Documents`, with
`cjb2`/`c44` standing in for the segmentation this plan adds and dejapdf
doing the PDF:

| Source | Original | Re-encoded | Gain |
|---|---|---|---|
| PDP11 Handbook, 300 dpi CCITT G4 | 757 KB | 458 KB lossless JBIG2 | 1.65× |
| | | 174 KB lossy JBIG2 | 4.3× |
| Etudes for Programmers, 241 dpi gray JPEG | 6221 KB | 473 KB mask + 1/3 background | 13× |

Of the 131 PDFs in `~/Documents`, 34 are scanned (~15 JPEG at
150–880 KB/page, ~14 G4, ~5 already JBIG2). Born-digital PDFs gain nothing
and are left alone.

## Ground rules

- The rules in CLAUDE.md hold: no GPL or commercial code, standard library
  only, JPEG 2000 rather than JPEG for anything re-encoded.
- **Change only what we re-encode.** Every other object (text layer, fonts,
  outline, links, page labels, metadata) is copied as is. A scan image is
  replaced *in place*, by object number, so content streams stay untouched.
- **Never grow a file.** A replacement that comes out larger than the
  original image is dropped, and the original is kept.
- **Lossless by default.** Lossy shape substitution is an option, guarded
  against the "6 becomes 8" class of errors.
- JBIG2 and JPX images are copied through: we have no decoders for them,
  and they are already compact.
- Oracles as before: `qpdf --check`, `mutool draw`, `pdftoppm`, `pdfimages`,
  `jbig2dec`, `acroread`, and Acrobat 26 by hand.
- Acrobat constraints already learned apply to new code: refinements only
  between same-size shapes at zero offset (docs/JBIG2_Coding.md §8).

## T4. Copy a PDF

- [ ] Copy the object graph from the parser to `internal/pdf.Writer`:
  renumber, put non-stream objects into object streams, carry over
  `Info`, the catalog (outline, page labels, names, structure tree),
  annotations
- [ ] Write the output unencrypted
- [ ] Drop unreferenced objects and old revisions

Done when: a straight copy of every PDF in the collection passes
`qpdf --check` and renders pixel-identical to the original
(`mutool draw` on every page); no copy is larger than its original by
more than 1%.

## T5. Find the scan images

- [ ] Parse content streams enough to track `q`/`Q`/`cm` and `Do`,
  following form XObjects
- [ ] A scan image is an image XObject that covers most of its page;
  record its effective dpi and the pages using it
- [ ] Classify: bilevel (1 bit, or `/ImageMask`), gray, colour; note
  `/Decode`, `/SMask`, `/Mask`, colour space (DeviceGray/RGB/CMYK, Indexed,
  ICCBased, CalGray/CalRGB)
- [ ] Skip what we cannot replace safely: inline images, images used as
  soft masks, images with `/SMask` or colour-key `/Mask`
- [ ] `-dump` for PDF input prints one line per page: what we found and
  what we would do

Done when: the dump agrees with `pdfimages -list` on every file in the
collection, and lists each of the 34 scanned books as scanned.

## T6. Decode the scan images

- [ ] CCITT through `internal/ccitt`: G4 and G3 1-D, `EncodedByteAlign`,
  `BlackIs1`, `/Decode`; G3 2-D (K>0) is not supported there, so pass
  it through
- [ ] Flate, LZW and RunLength images at 1 and 8 bits per component
- [ ] DCT through `image/jpeg`, including Adobe CMYK (inverted, APP14)
- [ ] Convert to the internal forms: 1-bit bitmap, gray or RGB pixels

Done when: decoded pixels equal `pdfimages -png` (lossless sources) or are
within ±1 of it (JPEG) on every scanned book.

## T7. Factor out mask coding

- [ ] Move `maskImage`/`codeShapes` and friends from `internal/convert`
  to a package that codes any `jb2.Image` as a JBIG2 stencil, with or
  without globals; `internal/convert` calls it

Done when: the DjVu path's output is byte-identical to before on the
corpus and the books; all tests pass.

## T8. Connected components

- [ ] Split a bitmap into 8-connected components; each one becomes a
  shape plus a blit, giving a `jb2.Image`
- [ ] Merge exact duplicates (hash, then compare) into one shape
- [ ] Keep very large components (rules, figures) as their own shapes;
  cap shape size so the generic coding stays efficient

Done when: rendering the `jb2.Image` reproduces the input bitmap exactly
on every page of the G4 books; tests on synthetic bitmaps.

## T9. Re-encode bilevel scans (lossless)

- [ ] Replace the image stream in place with a JBIG2 stencil from T7/T8:
  same object number, `Width`/`Height` kept, `/Filter /JBIG2Decode`
- [ ] Get the polarity right: JBIG2 decodes 1 as black, so an image mask
  needs no `/Decode`, while a 1-bit DeviceGray image needs `/Decode [1 0]`;
  combine with the original `/Decode`
- [ ] Share one `/JBIG2Globals` across pages for shapes repeated between
  pages, as with Djbz
- [ ] Keep the original when the replacement is not smaller

Done when: every page of the G4 books renders pixel-identical to the
original in `mutool draw` and `pdftoppm`; every stream decodes bit-exact
with `jbig2dec`; PDP11 Handbook is at least 1.5× smaller; `qpdf --check`
passes; TestAcrobat and Acrobat 26 by hand show no errors.

## T10. Shape matching

- [ ] Cluster similar components: size within a pixel or two, weighted XOR
  distance, same number of holes (Euler number) as a guard against
  confusing digits
- [ ] Lossless mode: code cluster members as refinements of the cluster
  representative, only where Acrobat accepts it (same size, zero offset);
  measure whether this beats T9
- [ ] Lossy mode (`-lossy`): replace members by the representative
- [ ] A test page of digits and look-alike letters (6/8, 3/8, 1/l/I, 0/O)
  at several scan qualities

Done when: lossy PDP11 Handbook is about 4× smaller than the original, the
digits page shows no substitution between different characters, and a
by-eye check of 10 random pages finds no errors.

## T11. Separate text from background

- [ ] Adaptive threshold (Sauvola or similar) on gray and colour scans;
  remove specks
- [ ] Leave photos and halftones in the background: detect them from
  component statistics (size, density, count per area)
- [ ] Foreground colour per component, quantised to a small palette (as
  FGbz), or plain black when the text is near black

Done when: on the Etudes book and the colour scans, a by-eye comparison
at 100% finds no lost or broken characters and no photo turned to a
stencil; the mask of a corpus page rendered to gray matches `ddjvu
-mode=mask` closely (a small XOR count).

## T12. Background

- [ ] Fill the pixels under the mask from their unmasked neighbours, so
  text leaves no ghosts and the wavelets spend no bits on it
- [ ] Downsample by 3 (or to ~100 dpi), encode as JPX with the existing
  rate control; `-q` sets the budget relative to the original image
- [ ] A background that is a single uniform colour becomes a fill, as on
  the DjVu path

Done when: backgrounds of the Etudes pages show no text ghosts at 400%,
and the whole page comes to within 20% of the `cjb2`+`c44` measurement
above.

## T13. Re-encode gray and colour scans

- [ ] Replace the image XObject in place with a form XObject,
  `/BBox [0 0 1 1]`, whose content draws the background JPX and the
  coloured stencils exactly as `internal/convert` lays out a DjVu page
  (palette stencils, pattern fill for a foreground image)
- [ ] Interpolation, cropping and the inset against viewer edge snapping
  carry over from the DjVu path
- [ ] Keep the original when the replacement is not smaller

Done when: the Etudes book is at least 10× smaller; pages render correctly
in MuPDF, Poppler, pdf.js, Ghostscript, TestAcrobat and Acrobat 26 by
hand; the OCR text layer still selects over the right words;
`qpdf --check` passes.

## T14. Command line

- [ ] `dejapdf in.pdf out.pdf`: pick the path by the input's magic bytes;
  `-p`, `-q`, `-v`, `-quiet` and the progress line work as for DjVu;
  add `-lossy`
- [ ] Refuse with a clear message: password-protected files, PDFs with
  nothing to re-encode (and say so)
- [ ] Run all 34 scanned books: no errors, no file grows

Done when: the size table for the scanned books is in the README; CI
passes.

## T15. Docs

- [ ] An article `docs/PDF_Rewriting.md`: what we read, how scan images
  are found, segmentation, and what stays untouched; regenerate the PDFs
- [ ] Update CLAUDE.md (architecture, commands) and the README

Done when: `make docs` is clean and the article PDFs are committed.
