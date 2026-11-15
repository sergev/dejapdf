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
Todo.md as tasks T1–T15.

## Commands

Standard Go tooling (`make build`, `make test`, `make lint`, `make fuzz`, `make corpus`).
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
- `internal/ccitt`: G4 decoder for `Smmr` masks (copied from golang.org/x/image).
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
  budget; the only codec for re-encoded images.
- `internal/pdf`: our own minimal PDF writer. It embeds already-encoded streams
  with any filter and DecodeParms; other objects go into object streams, with
  an xref stream.
- `internal/convert`: lays out each page in PDF. Pages are built in
  parallel, largest first, within a budget of colour pixels
  (`schedule.go`), and written in order; shared dictionaries are coded
  in the background. A page has:
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
- Real scanned books in `~/Documents` (not redistributable, never committed)
  for size measurements and `DJVU_BOOKS='glob' go test ./internal/djvu -run TestBooks`.
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
