# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`dejapdf` is a Go command-line tool that converts DjVu to PDF. It keeps the DjVu
layers separate instead of rasterizing each page, so the PDF stays 1–1.5× the
DjVu size (other tools produce ~30×). Module path: `github.com/sergev/dejapdf`.

**Status:** [Todo.md](Todo.md) is the roadmap: phases P2–P12, each ending with a
"Done when" check. Work through them in order and tick the checkboxes as you go;
finished phases are removed from it. The original design brief is
`tmp/create-djvu-to-pdf.md`.

## Commands

Standard Go tooling (`make build`, `make test`, `make lint`, `make corpus`):

```
go build ./cmd/dejapdf
go test ./...
go test ./internal/jb2 -run TestName      # single test
go test ./internal/zp -fuzz FuzzDecode    # fuzz one package
go vet ./...
```

## Architecture (planned, see Todo.md)

Data flows from DjVu decoding through re-encoding to the PDF writer:

- `internal/iff` → `internal/djvu`: IFF85 chunks; bundled (DJVM) and indirect
  documents; DIRM; INCL; INFO (size, dpi, rotation).
- `internal/zp`: the ZP arithmetic decoder, used by BZZ, JB2 and IW44.
- `internal/bzz`: decompresses DIRM, TXTz, ANTz and NAVM.
- `internal/jb2`: decodes to **structure** (shape dictionary + blits of shape
  index and x,y), not just a bitmap. That structure is what lets the JBIG2 output
  stay as small as JB2. Shared `Djbz` dictionaries map to PDF `/JBIG2Globals`.
- `internal/iw44`: decodes BG44/FG44 to pixels at their native low resolution.
- `internal/mq` + `internal/jbig2`: JBIG2 encoder (MQ coder, generic region,
  symbol dictionary, text region). Produces the PDF-embedded format with no file
  header.
- `internal/jpx`: JPEG 2000 encoder; the only codec for re-encoded images.
- `internal/pdf`: our own minimal PDF writer. It embeds already-encoded streams
  with any filter and DecodeParms.
- `internal/convert`: lays out each page in PDF:
  - background JPX stretched to the page;
  - mask as a stencil (`/ImageMask true`, `/Decode [1 0]`);
  - FG44 as a JPX with the stencil as its `/Mask`;
  - one stencil per FGbz palette colour;
  - TXTz as invisible text, NAVM as outlines, ANTz as links.
- BGjp/FGjp layers (already JPEG) pass through unchanged as DCTDecode.

## Rules

- **Never use GPL or commercial code or dependencies.** Implement DjVu decoding
  from the DjVu v3 spec, never from DjVuLibre source. Implement JBIG2 from
  ITU-T T.88.
- **Copy code in or write our own rather than adding Go module dependencies.**
  The aim is a `go.mod` with the standard library only.
- `tmp/` is git-ignored and holds Apache-2.0 upstream clones to borrow from:
  - `tmp/go-jpeg2000` → `internal/jpx` (encoder path only)
  - `tmp/jbig2` → MQ encoder and generic-region encoder
  - `tmp/gobig2` → reference for how symbol dictionaries and text regions decode
- Borrowed code keeps its upstream copyright header and is credited in `NOTICE`.
- Do not use JPEG for re-encoded images; use JPEG 2000.
- Test oracles are external command-line tools, run but never linked:
  - `ddjvu`, `djvudump`, `djvused` (from djvulibre-bin)
  - `jbig2dec`, `qpdf`, `mutool`, `pdftoppm`

  Typical checks:
  - masks are pixel-identical to `ddjvu -mode=mask`;
  - JBIG2 output decodes bit-exact with `jbig2dec`;
  - `qpdf --check` passes.
- Keep comments in source code short.
- Commit messages are a single short subject line.
