# dejapdf

A command-line DjVu → PDF converter written in Go, from scratch.

Most converters (`ddjvu -format=pdf`, `djvu2pdf`) render each page into one
full-resolution bitmap, so the PDF comes out ~30× larger than the DjVu.
`dejapdf` instead decodes each DjVu layer and re-encodes it into a format PDF
supports natively, keeping the layers separate. Target size: **1–1.5× the DjVu file**.

## How a page maps to PDF

| DjVu | PDF |
|---|---|
| `Sjbz` text mask (JB2) | JBIG2 stencil mask (`/ImageMask true`, `/Decode [1 0]`) |
| `Djbz` shared shape dictionary | `/JBIG2Globals`, shared across pages |
| `BG44` background (IW44) | JPEG 2000 image (`JPXDecode`), stretched to the page |
| `FG44` foreground colour (IW44) | JPEG 2000 image with the JBIG2 mask attached via `/Mask` |
| `FGbz` colour palette | one stencil mask per colour |
| `TXTz` hidden text | invisible text — the PDF stays searchable |
| `NAVM` table of contents | PDF bookmarks (outlines) |
| `ANTz` annotations | link annotations |

JB2 symbols are carried over to JBIG2 as-is (decoded shapes + positions, re-encoded
with the MQ coder), so no re-classification happens and no new character-substitution
errors are introduced.

## Usage (planned)

```
dejapdf [-q N] input.djvu output.pdf
```

## Build

```
go install github.com/sergev/dejapdf/cmd/dejapdf@latest
```

## Status

Early development. See [Todo.md](Todo.md) for the roadmap.

## Dependencies and licensing

Pure Go. Runtime dependency: [go-jpeg2000](https://github.com/mrjoshuak/go-jpeg2000) (Apache-2.0).
No GPL or commercial code: DjVu decoding follows the DjVu v3 specification, JBIG2
encoding follows ITU-T T.88.

Released under the MIT License, see [LICENSE](LICENSE).

## References

- DjVu v3 specification (LizardTech)
- ITU-T T.88 / ISO/IEC 14492 (JBIG2)
- PDF 1.7 / ISO 32000
