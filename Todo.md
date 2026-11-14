# dejapdf — Todo

Sequential plan. Each phase builds on the previous one and ends with a testable
"Done when" criterion.

## Ground rules

- **No GPL and no commercial code or dependencies.** Implement from the DjVu v3
  spec and ITU-T T.88; never read or copy DjVuLibre source.
- GPL/AGPL *binaries* (`ddjvu`, `djvudump`, `djvused`, `jbig2dec`) are used only
  as external test oracles, never linked or copied.
- **Prefer borrowing code or writing our own over linking dependencies.**
  Target: Go stdlib only in `go.mod`.
- Borrowed Apache-2.0 code goes under `internal/`, keeps its copyright header,
  and is credited in a top-level `NOTICE` file (Apache-2.0 §4). Upstream clones
  for reference live in `tmp/` (git-ignored):
  - `tmp/go-jpeg2000` (mrjoshuak, Apache-2.0) → `internal/jpx`, encoder path only
  - `tmp/jbig2` (xiaoqidun, Apache-2.0) → `internal/mq`, `internal/jbig2`:
    MQ arithmetic encoder (`jbig2_arith_encoder.go`), generic region encoder
    (`jbig2_grd_encoder.go`), segment/bitmap helpers
  - `tmp/gobig2` (dkrisman, Apache-2.0) → reference for symbol-dictionary and
    text-region decoding logic, mirrored when writing our encoder; its
    `testdata/` may serve as fixtures
- JBIG2 verification uses external decoders (`jbig2dec`, `mutool`), not Go deps.

## Library survey (summary)

| Need | Candidate | Decision |
|---|---|---|
| DjVu decoding | din14970/djvu-go | Rejected: GPL-2.0+, layers not exposed (renders RGBA only) |
| DjVu decoding | — | Write own (ZP, BZZ, JB2, IW44) |
| JBIG2 encoding | xiaoqidun/jbig2 (Apache-2.0) | Borrow MQ + generic-region encoder; write symbol dict / text region ourselves |
| JBIG2 decoding | dkrisman/gobig2 (Apache-2.0) | Reference for SD/TR semantics; not linked |
| JBIG2 encoding | UniPDF | Rejected: commercial |
| JPEG 2000 | mrjoshuak/go-jpeg2000 (Apache-2.0) | Borrow encoder path into `internal/jpx` — the only image codec |
| PDF writer | boxesandglue/baseline-pdf (BSD-3) | Can't embed pre-encoded JBIG2/JPX streams; write own |

## 0. Setup

- [x] `go mod init github.com/sergev/dejapdf`
- [x] Layout: `cmd/dejapdf`, `internal/{iff,zp,bzz,jb2,iw44,djvu,mq,jbig2,jpx,pdf,convert}`
- [x] `NOTICE` file crediting borrowed Apache-2.0 code
- [x] Makefile: `build`, `test`, `lint`
- [x] Install oracles: `djvulibre-bin` (ddjvu, djvudump, djvused, cjb2, c44, cpaldjvu, djvm), `pdf2djvu`, `jbig2dec`, `qpdf`
- [x] Corpus source `testdata/src/pages.ps` (own content: text, gradient, colour text, outline, links)
      and generator `testdata/mkcorpus.sh` (`make corpus`)
- [x] Run `make corpus`; commit `testdata/corpus` (bilevel, lossy, photo, gray, palette, compound,
      bundled + text/outline/annotations, indirect, pdf2djvu) with djvudump/djvused goldens
- [ ] Shared `Djbz` sample: Debian's pdf2djvu lacks `--pages-per-dict` (no minidjvu);
      need a real multi-page scan with Djbz, or build minidjvu as an external tool
- [ ] A few public-domain scanned books (kept outside git, for size measurements)

Done when: `go build ./...` and `go test ./...` pass on an empty skeleton.

## 1. DjVu container

- [ ] IFF85 chunk parser (`AT&T` magic, FORM nesting, padding)
- [ ] FORM:DJVU (single page), FORM:DJVM (multi-page), FORM:DJVI (shared include)
- [ ] DIRM: bundled and indirect documents (BZZ-compressed part finished in phase 3)
- [ ] INCL resolution (shared dictionaries, shared annotations)
- [ ] INFO: width, height, dpi, gamma, rotation, version
- [ ] `dejapdf -dump file.djvu` prints the chunk tree

Done when: `-dump` output matches `djvudump` on the corpus.

## 2. ZP-coder decoder

- [ ] Probability/threshold tables from the spec, adaptive context bits
- [ ] Decoder API: `Decode(ctx *byte) int`, `DecodePassthrough() int`

Done when: unit tests pass on known streams; fuzz test runs clean.

## 3. BZZ decompressor

- [ ] ZP-coded block decoding, MTF, inverse Burrows–Wheeler
- [ ] Finish DIRM (component names, sizes, flags)
- [ ] Raw decode of TXTz, ANTz, NAVM

Done when: decoded text and outlines match `djvused -e print-txt` / `print-outline`.

## 4. JB2 decoder

Produce the *structure*, not just a bitmap.

- [ ] Record types: start of image, new symbol (add to library / to image only / both),
      matched refinement, matched copy, non-symbol data, shared dictionary import,
      comments, required dictionary or reset, end of data
- [ ] Djbz dictionary decoding and reuse across pages via INCL
- [ ] Output: `Dict{Shapes []Bitmap}` (shared + page-local),
      `Page{W, H, Blits []{Shape, X, Y}}`; refinement shapes materialised
      as standalone bitmaps; non-library bitmaps tracked separately
- [ ] Render helper for tests

Done when: rendered masks are pixel-identical to `ddjvu -mode=mask` on the corpus.

## 5. Minimal PDF writer

- [ ] Objects, xref table, trailer, Flate-compressed streams
- [ ] Pages: MediaBox from INFO size and dpi, `/Rotate` from INFO
- [ ] Image XObjects with arbitrary pre-encoded filter and DecodeParms
- [ ] Document info (Producer, Title)

**Milestone A**: bilevel DjVu → PDF, mask as Flate-compressed 1-bit stencil.

Done when: `qpdf --check` passes; `pdftoppm` render matches `ddjvu` render.

## 6. IW44 decoder

- [ ] Wavelet coefficient decoding (buckets, bands, slices), ZP contexts
- [ ] Inverse wavelet transform
- [ ] Grayscale and colour (YCbCr→RGB, chroma delay, half-resolution chroma)
- [ ] Multiple BG44/FG44 chunks (progressive refinement); ignore TH44
- [ ] Output layer at its native (low) resolution

Done when: decoded background matches `ddjvu -mode=background` (PSNR near exact).

## 7. Layered page assembly

- [ ] `internal/jpx`: port the encoder path of `tmp/go-jpeg2000` (codestream, dwt 9/7,
      entropy/EBCOT encoder, mct, tcd, t2 packets, jp2 boxes); drop decoder, float/half,
      HTJ2K and progressive APIs; keep upstream unit tests for ported parts
- [ ] Background → JPEG 2000 via `internal/jpx` (`/JPXDecode`), `-q` quality, stretched to the page
- [ ] Mask → stencil (`/ImageMask true`) painted black
- [ ] FG44 → JPEG 2000 colour image with explicit `/Mask` pointing to the full-resolution stencil
- [ ] FGbz palette → split blits by colour index, one stencil per colour
- [ ] BGjp/FGjp (JPEG-coded layers, rare) → passthrough as `/DCTDecode`, no re-encode
- [ ] Smmr (MMR mask) → `/CCITTFaxDecode` if feasible
- [ ] Pages with no mask, no background, or photo-only pages

**Milestone B**: renders visually match `ddjvu` on the whole corpus; record size ratio per file.

## 8. MQ encoder + JBIG2 generic region

- [ ] MQ arithmetic encoder (T.88 Annex E), borrowed from `tmp/jbig2/jbig2_arith_encoder.go`
- [ ] Generic region encoding, template 0, TPGDON, from `tmp/jbig2/jbig2_grd_encoder.go`
- [ ] Segment headers; PDF-embedded format (no file header, no end-of-file segment)
- [ ] Replace Flate masks with JBIG2 generic regions (`/JBIG2Decode`)

Done when: output decodes bit-exact with `jbig2dec` and renders correctly in `mutool`.

## 9. JBIG2 symbol dictionary + text region

- [ ] Symbol dictionary segment: arithmetic coding, height classes, generic-coded symbols, no refinement
- [ ] Text region segment: arithmetic IA* contexts, SBSYMCODELEN, REFCORNER, no transposition
- [ ] Djbz shapes → `/JBIG2Globals` stream shared by all pages using it
- [ ] Page-local shapes → per-page symbol dictionary segment
- [ ] Non-symbol bitmaps → generic region or single-use symbols
- [ ] Verify polarity: `/Decode [1 0]` for stencils, correct sense for FG44 `/Mask`

**Milestone C**: mask bytes ≈ JB2 bytes; whole PDF 1–1.5× the DjVu size.

## 10. Extras

- [ ] TXTz → invisible text (`3 Tr`, glyph-less font, word positioning from zones)
- [ ] NAVM → outlines (bookmarks)
- [ ] ANTz → link annotations (`/URI` and internal page links), metadata → document info

Done when: PDF is searchable and copyable in viewers; bookmarks and links work.

## 11. Background rate tuning

- [ ] Choose JPX `CompressionRatio`/quality from the IW44 chunk size so the image ≈ IW44 bytes
- [ ] Check JPX rendering in pdf.js, MuPDF, Poppler, Acrobat

Done when: background bytes within ~1.2× of IW44 bytes at comparable visual quality.

## 12. Hardening and UX

- [ ] Page-parallel decode/encode
- [ ] Options: page range, quality, progress, verbose size report
- [ ] Graceful handling of broken or unknown chunks
- [ ] Fuzz tests for iff, zp, bzz, jb2, iw44
- [ ] Size-report tool comparing DjVu vs PDF per layer
- [ ] CI (build, test, vet)
- [ ] Update README with real usage and results
