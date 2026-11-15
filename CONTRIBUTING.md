# Building and testing dejapdf

## Build

In a clone, `make build`, and `make install` to copy the binary to
`~/.local/bin` (`make install BINDIR=...` for another directory).
Go 1.27 or later.

The code is pure Go, standard library only. Some Apache-2.0 code is
borrowed into `internal/` (see `NOTICE`): the JPEG 2000 encoder from
[go-jpeg2000](https://github.com/mrjoshuak/go-jpeg2000) and the MQ /
generic-region JBIG2 encoder from
[xiaoqidun/jbig2](https://github.com/xiaoqidun/jbig2). No GPL or commercial
code: DjVu decoding follows the DjVu v3 specification, JBIG2 encoding
follows ITU-T T.88.

## Prerequisites

Building needs only Go. The tests, `make corpus` and `make docs` also run
external tools; a test skips when its tool is missing.

| Tool | Used by | Debian/Ubuntu | macOS (Homebrew) |
|---|---|---|---|
| `go` 1.27+ | build | `golang` or [go.dev/dl](https://go.dev/dl/) | `go` |
| `ddjvu`, `djvused`, `djvudump`, `c44`, `cjb2`, `djvm` | tests, corpus | `djvulibre-bin` | `djvulibre` |
| `jbig2dec` | tests | `jbig2dec` | `jbig2dec` |
| `qpdf` | tests | `qpdf` | `qpdf` |
| `mutool` | tests | `mupdf-tools` | `mupdf-tools` |
| `pdftoppm` | tests | `poppler-utils` | `poppler` |
| `opj_decompress` | tests | `libopenjp2-tools` | `openjpeg` |
| `gs` | tests, corpus | `ghostscript` | `ghostscript` |
| `pdf2djvu`, `minidjvu` | corpus | `pdf2djvu`, `minidjvu` | `minidjvu` (no `pdf2djvu`) |
| `python3`, `pandoc`, `typst` | `make docs` | `python3`, `pandoc`, `typst` | `python`, `pandoc`, `typst` |

Debian/Ubuntu:

```
sudo apt install golang djvulibre-bin jbig2dec qpdf mupdf-tools \
    poppler-utils libopenjp2-tools ghostscript
sudo apt install python3 pandoc typst      # for make docs
```

The distribution's `golang` may be older than 1.27; if so, install Go from
[go.dev/dl](https://go.dev/dl/). On older releases without a `typst` package,
use the binary from [typst releases](https://github.com/typst/typst/releases).

macOS:

```
brew install go djvulibre jbig2dec qpdf mupdf-tools poppler openjpeg ghostscript
brew install pandoc typst                  # for make docs
```

`acroread` (Adobe Reader 9.5.5) exists only for Linux (i386); on macOS that
check is skipped.

The corpus in `testdata/corpus` is committed; `make corpus` regenerates it and
needs `pdf2djvu`, which Homebrew lacks, so regenerate it on Linux.

## Testing

`go test ./...` runs the unit tests and checks the output against the
external tools listed above; a test skips when its tool is missing.
`make fuzz` runs the fuzz targets of the decoders.

To check the hidden text, annotations and outline of your own books against
`djvused`, name them in `DJVU_BOOKS`:

```
DJVU_BOOKS='/path/to/books/*.djvu' go test ./internal/djvu -run TestBooks -v
```

And to re-encode your own scanned PDFs under the same checks (every JBIG2
image through `jbig2dec`, the object graph compared, `qpdf --check`, pages
rendered), name them in `PDF_BOOKS`:

```
PDF_BOOKS='/path/to/scans/*.pdf' go test ./internal/reencode -run 'TestJbig2dec|TestSeparate' -v
```

## Profiling

`-cpuprofile file` writes a CPU profile for `go tool pprof`, and
`-trace file` an execution trace for `go tool trace`:

```
dejapdf -cpuprofile cpu.prof -trace trace.out in.djvu out.pdf
```

## Documentation

Five articles in [docs/](docs/) describe the formats as dejapdf uses them:
[DjVu](docs/DjVu_Format.md) (what we decode),
[JBIG2](docs/JBIG2_Coding.md) (how we encode the mask),
[JPEG 2000](docs/JPEG_2000_Image_Coding.md) (how we encode the colour layers),
[PDF](docs/Portable_Document_Format.md) (what we write) and
[PDF rewriting](docs/PDF_Rewriting.md) (how scanned PDFs are re-encoded).
`make docs` regenerates their figures and PDFs.

References:

- DjVu v3 specification (LizardTech)
- ITU-T T.88 / ISO/IEC 14492 (JBIG2)
- ITU-T T.800 / ISO/IEC 15444-1 (JPEG 2000)
- PDF 1.7 / ISO 32000-1
