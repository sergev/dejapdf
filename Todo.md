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

## T8. Connected components

- [ ] Split a bitmap into 8-connected components; each one becomes a
  shape plus a blit, giving a `jb2.Image`
- [ ] Merge exact duplicates (hash, then compare) into one shape
- [ ] Keep very large components (rules, figures) as their own shapes;
  cap shape size so the generic coding stays efficient

Done when: rendering the `jb2.Image` reproduces the input bitmap exactly
on every page of the G4 books; tests on synthetic bitmaps.

## T9. Re-encode bilevel scans (lossless)

- [ ] Replace the image stream in place with a JBIG2 stencil (`internal/stencil`, T8):
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

## T10. McKusick: visible text over a bilevel scan

`McKusick_-_Design_and_Implementation_of_the_4.4_BSD_Operating_System_-_Part_2__UNIX__(AW,_1996).pdf`
(276 pages, 19.6 MB) draws visible vector text (`0 Tr`, black,
non-embedded base-14 Times) over a page-size 600 dpi CCITT G4 image of
about 7000×5000 pixels (two-page spreads, 868×606 pt). The text is the
page's text; the image holds only what the converter did not turn into
text: rules, figures, stray marks. `-dump` plans JBIG2 for every page.
Re-encoding is safe only if nothing else changes.

- [ ] Re-encode with T9; check that only the image streams change: every
  other object keeps its values (renumbered), and the content streams
  keep their data byte for byte
- [ ] The images decode bit-exact (`jbig2dec` against `pdfimages -png`
  of the original) at this width; check the JBIG2 encoder on 7000-pixel
  rows and near-empty pages
- [ ] Every page renders pixel-identical to the original in `mutool
  draw` and `pdftoppm`; the text still selects and searches
  (`pdftotext` output unchanged)
- [ ] The content streams make Poppler warn ("Badly formatted number",
  8368 times): find why, and make sure our content parsing (T5) reads
  them as the viewers do, so the image's matrix and coverage are right
- [ ] Check in TestAcrobat and in Acrobat 26 by hand: text over the
  JBIG2 image, at these sizes

Done when: the file is smaller, renders identically page by page in
MuPDF and Poppler, `pdftotext` and `qpdf --check` agree with the
original, and Acrobat shows it without errors.

## T11. Shape matching

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

## T12. Separate text from background

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

## T13. Background

- [ ] Fill the pixels under the mask from their unmasked neighbours, so
  text leaves no ghosts and the wavelets spend no bits on it
- [ ] Downsample by 3 (or to ~100 dpi), encode as JPX with the existing
  rate control; `-q` sets the budget relative to the original image
- [ ] A background that is a single uniform colour becomes a fill, as on
  the DjVu path

Done when: backgrounds of the Etudes pages show no text ghosts at 400%,
and the whole page comes to within 20% of the `cjb2`+`c44` measurement
above.

## T14. Re-encode gray and colour scans

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

## T15. Command line

- [ ] `dejapdf in.pdf out.pdf`: pick the path by the input's magic bytes;
  `-p`, `-q`, `-v`, `-quiet` and the progress line work as for DjVu;
  add `-lossy`
- [ ] Refuse with a clear message: password-protected files, PDFs with
  nothing to re-encode (and say so)
- [ ] Run all scanned books (the 40 files that `-dump` calls scanned):
  no errors, no file grows

Done when: the size table for the scanned books is in the README; CI
passes.

## T16. Docs

- [ ] An article `docs/PDF_Rewriting.md`: what we read, how scan images
  are found, segmentation, and what stays untouched; regenerate the PDFs
- [ ] Update CLAUDE.md (architecture, commands) and the README

Done when: `make docs` is clean and the article PDFs are committed.
