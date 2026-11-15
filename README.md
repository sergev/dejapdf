# dejapdf

A command-line DjVu → PDF converter written in Go, from scratch. It also
re-encodes scanned PDFs the same way, making them several times smaller.

Most converters (`ddjvu -format=pdf`, `djvu2pdf`) render each page into one
full-resolution bitmap, so the PDF comes out 10–100× larger than the DjVu.
`dejapdf` instead decodes each DjVu layer and re-encodes it into a format PDF
supports natively, keeping the layers separate. The PDF comes out
**1.06–1.18× the DjVu size** on scanned books.

## Usage

```
dejapdf [options] input.djvu output.pdf
dejapdf [options] input.pdf output.pdf
dejapdf -dump input.djvu|input.pdf
```

| Option | Meaning |
|---|---|
| `-p 1-10,15` | pages to convert (default all) |
| `-q 1.2` | colour layer size as a multiple of its DjVu size (default 1.2); for a PDF, the cap on a page's background as a fraction of its scan image (default 0.15) |
| `-lossless` | PDF input: keep the shapes of the text exact (see below) |
| `-v` | show a size report by layer |
| `-quiet` | print nothing but warnings and errors |
| `-dump` | print the chunk tree, like `djvudump`; for a PDF, the scan images by page and what will be done with each |
| `-version` | print the version |
| `-cpuprofile file` | write a CPU profile, for `go tool pprof` |
| `-trace file` | write an execution trace, for `go tool trace` |

`-q` trades colour quality for size; the black-and-white text layer is
lossless and does not depend on it.

```
$ dejapdf -v book.djvu book.pdf
page 84/84
layer            djvu          pdf  ratio
mask          1155792      1268745   1.10
colour          68172        81594   1.20
text           209424       189448   0.90
other            4866        28244   5.80
total         1438254      1568031   1.09
reading 0.0s, pages 0.5s, writing 0.0s
book.pdf: 1531 kbytes, 1.09x in 0.5s
```

Without `-v`, only the last line is printed. While converting, when the
output is a terminal, a progress line shows the percentage done and the
pages (or, for a PDF, the scan images) done, or the phase: reading,
finding images, writing; the summary line replaces it.

## Results

Nine scanned books (14–537 pages, 1–23 MB, bilevel and colour), converted
with the default options, come out 1.06–1.18× the DjVu size, in 0.5–12 s
each. Every PDF passes `qpdf --check`. For comparison, `ddjvu -format=pdf`
makes 9.8–97× the DjVu size on the same books.

The PDF keeps what the DjVu has: hidden text (so it stays searchable),
the table of contents as bookmarks, links, page labels and metadata. The
text layer is carried over losslessly, so no new character substitution
errors appear.

The PDFs are checked in MuPDF, Poppler, pdf.js, Ghostscript, Adobe
Reader 9 and Adobe Acrobat 26.

## Scanned PDFs

Given a PDF, `dejapdf` re-encodes its scan images in place and copies
everything else (text layer, bookmarks, links, metadata) as it is:

- bilevel scans become JBIG2, with symbol dictionaries shared by groups of
  32 pages;
- gray and colour scans are split like a DjVu page: the text as JBIG2
  stencils in its own colours, the rest as a JPEG 2000 background at about
  100 dpi with the text filled out of it. Faded pages get their contrast
  raised.

By default, characters of the same shape are drawn from one bitmap, with a
guard against look-alikes such as 6 and 8; `-lossless` keeps every shape
exact instead (bilevel scans then decode to exactly the original bitmap). A file is
never made larger: if re-encoding saves nothing, it is copied as is. One
with nothing to re-encode is written unchanged (`-v` tells why). A
password-protected PDF is refused.

The 40 scanned PDFs of one collection, with the default options: 30
distinct books re-encoded below; 6 were refused (their scans already
JBIG2 or JPEG 2000, or text drawn over them); 4 were duplicates. Every
output passes `qpdf --check`.

| Scans | dpi | Pages | Before, MB | After, MB | Smaller by | Time, s |
|---|---:|---:|---:|---:|---:|---:|
| bilevel, CCITT G4 | 600 | 369 | 39.0 | 4.6 | 8.5× | 3.3 |
| bilevel, CCITT G4 | 400 | 524 | 34.2 | 14.4 | 2.4× | 15.4 |
| bilevel, CCITT G4 | 300 | 742 | 33.6 | 12.8 | 2.6× | 19.0 |
| bilevel, CCITT G4 | 300 | 527 | 22.5 | 4.4 | 5.1× | 5.4 |
| bilevel, CCITT G4 | 529 | 288 | 21.3 | 3.0 | 7.0× | 9.2 |
| bilevel, CCITT G4 | 600 | 276 | 19.6 | 16.5 | 1.2× | 3.8 |
| bilevel, CCITT G4 | 600 | 84 | 15.8 | 1.4 | 11.6× | 2.8 |
| bilevel, CCITT G4 | 600 | 226 | 15.8 | 9.0 | 1.7× | 8.6 |
| bilevel, CCITT G4 | 300 | 202 | 13.4 | 4.4 | 3.0× | 20.1 |
| bilevel, CCITT G4 | 600 | 81 | 10.5 | 1.0 | 10.8× | 1.1 |
| bilevel, CCITT G4 | 301 | 136 | 7.4 | 5.1 | 1.4× | 2.0 |
| bilevel, CCITT G4 | 232 | 283 | 6.6 | 1.4 | 4.6× | 0.8 |
| bilevel, CCITT G4 | 339 | 6 | 0.6 | 0.2 | 2.9× | 0.2 |
| bilevel, Flate | 400 | 274 | 36.2 | 4.7 | 7.7× | 14.5 |
| bilevel, Flate | 300 | 1 | 0.1 | 0.0 | 3.7× | 0.0 |
| bilevel, JBIG2 | 600 | 546 | 37.0 | 35.9 | 1.0× | 1.2 |
| bilevel, JBIG2 | 600 | 263 | 7.2 | 6.7 | 1.1× | 0.3 |
| colour, Flate | 400 | 1 | 3.8 | 0.4 | 9.5× | 14.8 |
| colour, JPEG | 400 | 228 | 130.8 | 2.3 | 56.6× | 28.2 |
| colour, JPEG | 342 | 137 | 123.6 | 15.4 | 8.0× | 41.5 |
| colour, JPEG | 342 | 95 | 81.6 | 10.2 | 8.0× | 35.0 |
| colour, JPEG | 300 | 37 | 6.0 | 1.4 | 4.2× | 3.1 |
| gray, JPEG | 241 | 458 | 166.3 | 12.5 | 13.3× | 40.5 |
| gray, JPEG | 241 | 208 | 66.1 | 4.5 | 14.8× | 13.3 |
| gray, JPEG | 241 | 258 | 64.3 | 4.4 | 14.6× | 15.4 |
| gray, JPEG | 241 | 290 | 59.8 | 7.9 | 7.6× | 19.7 |
| gray, JPEG | 241 | 130 | 23.6 | 3.3 | 7.1× | 7.9 |
| gray, JPEG | 96 | 53 | 22.3 | 4.3 | 5.2× | 9.8 |
| gray, JPEG | 96 | 52 | 18.6 | 4.2 | 4.4× | 11.3 |
| gray, JPEG | 96 | 37 | 14.9 | 3.3 | 4.5× | 7.0 |
| **all 30** | | | **1102** | **200** | **5.5×** | |

Bilevel scans already in JBIG2 gain little; gray and colour scans in JPEG
gain the most.

## Build

```
go install github.com/sergev/dejapdf/cmd/dejapdf@latest
```

or, in a clone, `make build`, and `make install` to copy the binary to
`~/.local/bin` (`make install BINDIR=...` for another directory).
Go 1.27 or later.

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

## Documentation

Five articles in [docs/](docs/) describe the formats as dejapdf uses them:
[DjVu](docs/DjVu_Format.md) (what we decode),
[JBIG2](docs/JBIG2_Coding.md) (how we encode the mask),
[JPEG 2000](docs/JPEG_2000_Image_Coding.md) (how we encode the colour layers),
[PDF](docs/Portable_Document_Format.md) (what we write) and
[PDF rewriting](docs/PDF_Rewriting.md) (how scanned PDFs are re-encoded).

## Dependencies and licensing

Pure Go, standard library only. Some Apache-2.0 code is borrowed into `internal/`
(see `NOTICE`): the JPEG 2000 encoder from
[go-jpeg2000](https://github.com/mrjoshuak/go-jpeg2000) and the MQ / generic-region
JBIG2 encoder from [xiaoqidun/jbig2](https://github.com/xiaoqidun/jbig2).
No GPL or commercial code: DjVu decoding follows the DjVu v3 specification, JBIG2
encoding follows ITU-T T.88.

Released under the MIT License, see [LICENSE](LICENSE).

## References

- DjVu v3 specification (LizardTech)
- ITU-T T.88 / ISO/IEC 14492 (JBIG2)
- ITU-T T.800 / ISO/IEC 15444-1 (JPEG 2000)
- PDF 1.7 / ISO 32000-1
