# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`dejapdf` is a Go command-line tool that converts DjVu to PDF. It keeps the DjVu
layers separate instead of rasterizing each page, so the PDF stays 1–1.5× the
DjVu size (other tools produce ~30×). Module path: `github.com/sergev/dejapdf`.

**Status:** DjVu to PDF is complete (the old roadmap is in git history).
Checked in MuPDF, Poppler, pdf.js, Ghostscript, Adobe Reader 9
(TestAcrobat) and, by hand, Acrobat 26 on macOS. The original design brief is
`tmp/create-djvu-to-pdf.md`. Next: re-encoding scanned PDFs, planned in
Todo.md as tasks T15–T16.

## Commands

Standard Go tooling (`make build`, `make test`, `make lint`, `make fuzz`, `make corpus`, `make corpus-pdf`).
Fuzz with few workers (`-parallel 4`): one per CPU can exhaust memory.

```
go build ./cmd/dejapdf
go test ./...
go test ./internal/jb2 -run TestName      # single test
go test ./internal/zp -fuzz FuzzDecode -parallel 4   # fuzz one target
make fuzz FUZZTIME=60s                    # all targets, capped memory
go vet ./...
dejapdf -cpuprofile cpu.prof -trace trace.out in.djvu out.pdf   # profile
```

Speed changes must keep the output byte-identical: convert the corpus
and some books before and after, and `cmp` the PDFs. The coders' fast
paths (`zp.TryDecode`, `mq.TryEncode`) inline only at call sites in hot
loops; the jbig2 tests check the coders against pixel-by-pixel references.

## Architecture

Data flows from DjVu decoding through re-encoding to the PDF writer:

- `internal/iff` → `internal/djvu`: IFF85 chunks; bundled (DJVM) and indirect
  documents; DIRM; INCL; INFO (size, dpi, rotation).
- `internal/ccitt`: G4 and G3 decoder for `Smmr` masks and PDF CCITT
  images (copied from golang.org/x/image; accepts EOL fill bits, keeps
  a partly decoded row).
- `internal/zp`: the ZP arithmetic decoder, used by BZZ, JB2 and IW44.
- `internal/bzz`: decompresses DIRM, TXTz, ANTz and NAVM.
- `internal/jb2`: decodes to **structure** (shape dictionary + blits of shape
  index and x,y, and which shape each refined shape refines), not just a
  bitmap. That structure is what lets the JBIG2 output stay as small as JB2.
  Shared `Djbz` dictionaries map to PDF `/JBIG2Globals`.
- `internal/iw44`: decodes BG44/FG44 to pixels at their native low resolution.
- `internal/mq` + `internal/jbig2`: JBIG2 encoder (MQ coder, generic region,
  symbol dictionary, refinement dictionary, text region). Produces the
  PDF-embedded format with no file header.
- `internal/jpx`: JPEG 2000 encoder with PCRD-opt rate control to a byte
  budget; the only codec for re-encoded images. `EncodeSlope` also
  drops passes removing less than a given squared error per byte
  (grain goes, a faint feature stays).
- `internal/pdfread`: PDF reader for re-encoding scanned PDFs: objects
  (as `internal/pdf` values plus `*Stream`), xref tables and streams,
  object streams, `/Prev` chains, repair by scanning, decryption, page
  tree; the generic filters (Flate, LZW, ASCII85, ASCIIHex, RunLength,
  PNG and TIFF predictors). `Decode` stops at an image filter with
  `UnsupportedFilterError` and the data decoded so far. A sign inside
  a number ends it (`0.5-5` is 0.5), as in Reader, MuPDF and
  Ghostscript; Poppler skips the sign and warns "Badly formatted number".
- `internal/rewrite`: copies a document read by `pdfread` through the
  writer: objects reachable from the catalog and Info, renumbered (the
  root of the page tree at `Writer.PageTree`, ending with `Finish`),
  streams with their encoded data, decrypted; reals written exactly.
  `Replace` writes new streams in place of some objects. Dictionary
  keys are visited sorted, so the numbering (and output) is the same
  every run.
- `internal/reencode`: re-encodes a scanned PDF (`dejapdf in.pdf
  out.pdf`): each bilevel scan image becomes a JBIG2 image with the
  same object number (`shapes.Split`, then `stencil.Code`), with a
  `/Decode` from `raster.Bitmap.Decode`; groups of 32 images share a
  `/JBIG2Globals` of the shapes found in more than one; an image, or a
  group, that would not shrink is kept. Lossless: alike shapes of the
  same size are coded as refinements of their representative. `-lossy`
  (`Options.Lossy`): they are replaced by it (PDP11 Handbook 5×
  smaller, against 1.66× lossless). MuPDF renders an image whose
  ink was raw sample 1 one gray level off when subsampling (it averages
  before `/Decode`); identical from half the image's resolution up.
  Tests: jbig2dec on every replaced image, and the object graphs of
  input and output compared (equal but for numbering and the images'
  filters; streams byte for byte).
  Gray and colour scans (T14, `separate.go`): each becomes a form
  XObject, `/BBox [0 0 1 1]`, with the same object number: the
  background (`background.go`, or a fill), then one JBIG2 stencil per
  palette colour cropped to its marks and inset by 1e-5 against edge
  snapping; stencils of 32 images share globals. Tests: forms decode
  (opj_decompress, jbig2dec); pages render in MuPDF, and print in
  Reader, alike blurred (26 dB at 50 dpi). Etudes 9.1× (the text
  layer, copied, is 2.1 MB of it), 15× with `-lossy`.
- `internal/scan`: finds the scan images of a PDF by walking each page's
  content (`pdfread.Content`) through forms, soft-mask groups and
  annotation appearances: every image drawn with its matrix, dpi and
  page coverage, as `pdfimages -list` reports them. A page is scanned
  when its images cover 70% of it; a document when half its pages are.
  `Plan` decides per image: JBIG2 (bilevel), separate (gray, colour) or
  keep, with the reason (CMYK is kept: viewers convert it each their
  own way, and we would fix one conversion). `dejapdf -dump file.pdf` prints it by page.
- `internal/raster`: decodes scan images to a bitmap (1 = painted or
  darker) or 8-bit gray, RGB or CMYK pixels, applying the colour space,
  /Decode, palettes and stencil polarity: CCITT (G4, G3 1-D), Flate,
  LZW, RunLength, and DCT through `internal/jpeg` with libjpeg's
  upsampling and YCbCr rounding. Equal to `pdfimages -png` (`-tiff` for
  CMYK) on every scanned book.
- `internal/segment`: separates the text of gray and colour scans
  (T12): `Mask` works on the luminance, darkened where the darkest
  channel falls further below the paper's (bright coloured ink stands
  out; nothing changes for ink of a coloured paper's hue). A faded page
  (the darkest 0.5% within 160 levels of the paper: Malevich) is
  stretched to full range for the threshold only. It thresholds with Sauvola (window 1/6 inch, plus anything
  darker than half the paper), fills dark holes (strokes wider than the
  window), and drops specks, marks less than 30 levels darker than
  their tile's paper, and pictures: tiles with dark paper or mid-tones
  away from the ink (photos), thick blobs over a square inch (shadows,
  page edges), and Bloomberg's halftone mask (Leptonica's method,
  written anew; textures too). Small marks at the edge of a picture's
  box, on clean paper, stay: captions. Tile paper levels leave out the
  ink and a halo of dpi/150. `Colors` gives each mark its colour:
  lightness from its 25th–50th percentile pixels, hue from the mark and
  its halo summed against the paper (JPEG chroma subsampling spreads a
  stroke's colour but keeps its sum); k-means palette of up to 8,
  merging close centres (luma at half weight), each mark the nearest.
  `Enhance` darkens faded near-neutral ink to a third of the paper's
  luminance (saturated colours and the paper stay as printed).
  Light text on dark or coloured ground stays in the background.
  `Background` (T13) reduces the scan and fills what the mask hides:
  pixels within r0 of the mask are left out, those within r1 count
  less the nearer, and not at all if darker than the paper beyond r1
  (no ghost of the text); empty cells are filled by push-pull.
  `reencode.background` codes it at about 100 dpi as JPX
  (`EncodeSlope` 100, capped at `-q` × the original, default 0.15),
  or as a fill when uniform. Etudes: 12.9× lossless, 32× lossy
  (`TestSeparate` with PDF_BOOKS reports this).
- `internal/jpeg`: Go's image/jpeg decoder (BSD) with libjpeg's integer
  IDCT, returning component planes at their own resolution.
- `internal/shapes`: splits a bilevel bitmap into 8-connected
  components (runs joined by union-find) as a `jb2.Image`: exact
  duplicates merged, components over `MaxSide` (1024) tiled. `Classify`
  groups alike shapes (`match.go`): sizes within 2 pixels (1 up to 24,
  none for marks up to 10 and for narrow shapes like l, I, 1), the same
  holes, each within a pixel of the other with centroids aligned, a
  weighted XOR under the perimeter, and no thick XOR patch (a serif, a
  flag, a tail). Each shape joins its best earlier representative,
  in parallel by chunks, independent of the CPU count. Tuned on
  `testdata/pdf/digits.pdf` (look-alike characters, three scan
  qualities): TestLookalikes reads every glyph before and after
  substitution.
- `internal/stencil`: codes a `jb2.Image` (or a subset of its blits) as
  a JBIG2 stencil: symbol and refinement dictionaries plus a text region,
  or a generic region for one bitmap; shared dictionaries as
  `/JBIG2Globals` coded once in the background (`Cache`), or inline.
- `internal/pdf`: our own minimal PDF writer. It embeds already-encoded streams
  with any filter and DecodeParms; other objects go into object streams, with
  an xref stream.
- `internal/convert`: lays out each page in PDF. Pages are built in
  parallel, largest first, within a budget of colour pixels
  (`schedule.go`), and written in order; masks via `internal/stencil`. A page
  has:
  - background JPX stretched to the page;
  - mask as a stencil (`/ImageMask true`; JBIG2 decodes black as 0, so no
    `/Decode`; a raw 1 = black mask needs `/Decode [1 0]`);
  - FG44 (and FGjp) as the stencil filled with a pattern whose cell is the
    JPX image (`/Mask` renders badly in Poppler);
  - one stencil per FGbz palette colour;
  - TXTz as invisible text, NAVM as outlines, ANTz as links.
- BGjp/FGjp layers (already JPEG) pass through unchanged as DCTDecode.

## Docs

`docs/` holds the specs (DjVu v3 as `DjVu3Spec.pdf`, converted by dejapdf and
committed; T.88, T.800, PDF 1.7 downloaded and git-ignored; see
`docs/README.md`, which also lists the DjVu spec errata) and four articles:
`docs/DjVu_Format.md` (what we decode), `docs/JBIG2_Coding.md` (how we encode
the mask), `docs/JPEG_2000_Image_Coding.md` (how we encode the colour layers)
and `docs/Portable_Document_Format.md` (what we write). Figures are `docs/figures/NAME.py` scripts
printing `NAME.svg` via `docs/figures/svg.py`; `make docs` regenerates them
and formats the articles to PDF (pandoc + typst; `make docs-pdf` for the PDFs only).
The article PDFs are committed: regenerate them after editing an article.

## Test data

- `testdata/corpus`: made by `make corpus` from `testdata/src/pages.ps`, with
  djvudump/djvused goldens; committed, so regenerating is rarely needed. Needs
  `pdf2djvu` (not in Homebrew) and `minidjvu` (for `shared-dict.djvu`).
- `testdata/pdf`: scanned PDFs made by `make corpus-pdf` (`testdata/mkpdf.sh`)
  from the `pdf2djvu.djvu` pages with img2pdf, tiff2pdf, qpdf, gs and mutool:
  CCITT G4/G3, Flate, LZW, RunLength, ASCII85/ASCIIHex chains, JPEG (gray,
  RGB, CMYK), Indexed, ImageMask, inline
  image, OCR text layer, rotation, encryption, an incremental update;
  `digits.pdf`, look-alike characters for shape matching. Goldens:
  `*.images` (`pdfimages -list`) and `npages.txt`.
- Real scanned books in `~/Documents` (not redistributable, never committed)
  for size measurements and `DJVU_BOOKS='glob' go test ./internal/djvu -run TestBooks`.
  Scanned PDFs too: `PDF_BOOKS='glob' go test ./internal/pdfread -run 'TestBooks|TestStreamsQpdf'`.
- On macOS, Acrobat can be driven by AppleScript `do script` (Acrobat
  JavaScript) to read the outline, links and hidden text; it cannot export
  PostScript or images, so TestAcrobat stays Linux-only.

## Rules

- **Never use GPL or commercial code or dependencies.** Implement DjVu decoding
  from the DjVu v3 spec, never from DjVuLibre source. Implement JBIG2 from
  ITU-T T.88.
- **Copy code in or write our own rather than adding Go module dependencies.**
  The aim is a `go.mod` with the standard library only.
- `tmp/` is git-ignored and holds upstream clones to borrow from:
  - `tmp/go-jpeg2000` (Apache-2.0) → `internal/jpx` packet writer and tag tree
  - `tmp/jbig2` (Apache-2.0) → `internal/mq` MQ encoder; `internal/jbig2` generic region
  - `tmp/gobig2` (Apache-2.0) → reference for how symbol dictionaries and text
    regions decode
  - `tmp/openjpeg` (BSD-2) → reference for JPEG 2000 encoder details
  - `tmp/djvu-rs` (MIT) → reference where the DjVu spec is wrong
  - `tmp/leptonica` (BSD-2) → page segmentation (`pageseg.c`: halftone
    mask), Sauvola, morphology; `tmp/noteshrink` (MIT) → colour palette
    of ink; `tmp/jbig2enc` (Apache-2.0) → its text/halftone split
  - `tmp/how-to-separate-text.md` → notes on DjVu's segmentation and
    the open implementations
  - `tmp/mupdf` (AGPL), `tmp/poppler` (GPL) → read only, to understand how
    the viewers render (e.g. how they snap image edges); never copy from them
- Borrowed code keeps its upstream copyright header and is credited in `NOTICE`.
- Do not use JPEG for re-encoded images; use JPEG 2000.
- Test oracles are external command-line tools, run but never linked:
  - `ddjvu`, `djvudump`, `djvused` (from djvulibre-bin)
  - `jbig2dec`, `qpdf`, `mutool`, `pdftoppm`
  - `acroread` (Adobe Reader 9.5.5 for Linux, i386): `-toPostScript` prints
    without a window; its JBIG2 decoder is stricter than the others

  Typical checks:
  - masks are pixel-identical to `ddjvu -mode=mask`;
  - JBIG2 output decodes bit-exact with `jbig2dec`;
  - `qpdf --check` passes.
- Keep comments in source code short.
- Commit messages are a single short subject line.
