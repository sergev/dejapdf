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

- [ ] `dejapdf in.pdf out.pdf` works since T9 (magic bytes, `-v`,
  `-quiet`, progress by image; `-lossy` since T11); make `-p` and `-q`
  work as for DjVu
- [ ] Decide whether `-lossy` becomes the default, after running it on
  all scanned books
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
