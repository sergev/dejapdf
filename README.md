# dejapdf

A command-line DjVu → PDF converter written in Go, from scratch.

Most converters (`ddjvu -format=pdf`, `djvu2pdf`) render each page into one
full-resolution bitmap, so the PDF comes out 10–100× larger than the DjVu.
`dejapdf` instead decodes each DjVu layer and re-encodes it into a format PDF
supports natively, keeping the layers separate. The PDF comes out
**1.06–1.18× the DjVu size** on the books below.

## Usage

```
dejapdf [options] input.djvu output.pdf
dejapdf -dump input.djvu
```

| Option | Meaning |
|---|---|
| `-p 1-10,15` | pages to convert (default all) |
| `-q 1.2` | colour layer size as a multiple of its DjVu size (default 1.2) |
| `-v` | show progress and a size report by layer |
| `-dump` | print the chunk tree, like `djvudump` |
| `-version` | print the version |

`-q` sets the byte budget of the JPEG 2000 re-encoding of the colour layers;
the mask and text are lossless and do not depend on it.

```
$ dejapdf -v Алгол-Брудно.djvu Алгол-Брудно.pdf
page 84/84
layer            djvu          pdf  ratio
mask          1155792      1268745   1.10
colour          68172        81594   1.20
text           209424       189448   0.90
other            4866        28244   5.80
total         1438254      1568031   1.09
```

## Results

Scanned books, converted with the default options. Every PDF passes
`qpdf --check`.

| Book | Pages | DjVu, MB | PDF, MB | PDF / DjVu | Time, s |
|---|--:|--:|--:|--:|--:|
| algol-68-russian | 537 | 22.97 | 26.40 | 1.150 | 10.2 |
| Алгол-60, пересмотренное сообщение | 81 | 1.07 | 1.19 | 1.107 | 1.0 |
| Алгол (Брудно) | 84 | 1.44 | 1.57 | 1.090 | 0.5 |
| Криминалистический анализ файловых систем | 482 | 7.65 | 8.15 | 1.066 | 2.0 |
| Куліш, Чорна рада | 185 | 3.40 | 3.83 | 1.128 | 0.7 |
| Программирование ЕС ЭВМ | 266 | 6.44 | 7.05 | 1.096 | 2.3 |
| Гнеденко и др., Элементы программирования | 349 | 6.06 | 6.40 | 1.056 | 2.6 |
| Юный кибернетик | 451 | 23.38 | 26.78 | 1.146 | 12.2 |
| 20 уроков мудрости Дао (colour) | 14 | 3.64 | 4.31 | 1.184 | 5.5 |

For comparison, `ddjvu -format=pdf` makes 15.8 MB of Алгол (Брудно) (11×),
10.5 MB of Алгол-60 (9.8×) and 352 MB of Дао (97×).

## How a page maps to PDF

| DjVu | PDF |
|---|---|
| `Sjbz` text mask (JB2) | JBIG2 stencil mask (`/ImageMask true`) |
| `Djbz` shared shape dictionary | `/JBIG2Globals`, shared across pages |
| `Smmr` mask (G4) | JBIG2 generic region |
| `BG44` background (IW44) | JPEG 2000 image (`JPXDecode`), stretched to the page |
| `FG44` foreground colour (IW44) | the stencil filled with a pattern of the JPEG 2000 image |
| `FGbz` colour palette | one stencil mask per colour |
| `BGjp`, `FGjp` (JPEG) | passed through as `DCTDecode` |
| `TXTz` hidden text | invisible text: the PDF stays searchable |
| `NAVM` table of contents | PDF outline (bookmarks) |
| `ANTz` annotations | link annotations |
| `DIRM` page titles, metadata | page labels, Info dictionary and XMP |

JB2 shapes and their positions are carried over to JBIG2 as they are, so
no re-classification happens and no new character-substitution errors are
introduced. Shapes that JB2 refines from another shape of the same size stay
refinements; the rest are coded directly, because Adobe Reader fails on
size-changing refinements ([docs/JBIG2_Coding.md](docs/JBIG2_Coding.md) §8).

The PDFs are checked in MuPDF, Poppler, pdf.js, Ghostscript, Adobe
Reader 9 and Adobe Acrobat 26.

## Build

```
go install github.com/sergev/dejapdf/cmd/dejapdf@latest
```

or, in a clone, `make build`. Go 1.27 or later.

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

`go test ./...` runs the unit tests and checks the output against external
tools, which are run but never linked: `ddjvu`, `djvused`, `djvudump`
(djvulibre-bin), `jbig2dec`, `qpdf`, `mutool`, `pdftoppm`, `opj_decompress`,
`gs`, and `acroread` (Adobe Reader 9 for Linux) when installed. A test skips
when its tool is missing. Masks must be pixel-identical to
`ddjvu -mode=mask`, and the JBIG2 streams must decode bit-exact with
`jbig2dec`. `make fuzz` runs the fuzz targets of the decoders.

To check the hidden text, annotations and outline of your own books against
`djvused`, name them in `DJVU_BOOKS`:

```
DJVU_BOOKS='/path/to/books/*.djvu' go test ./internal/djvu -run TestBooks -v
```

## Documentation

Four articles in [docs/](docs/) describe the formats as dejapdf uses them:
[DjVu](docs/DjVu_Format.md) (what we decode),
[JBIG2](docs/JBIG2_Coding.md) (how we encode the mask),
[JPEG 2000](docs/JPEG_2000_Image_Coding.md) (how we encode the colour layers)
and [PDF](docs/Portable_Document_Format.md) (what we write).

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
