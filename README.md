# dejapdf

**dejapdf turns DjVu books into PDF without making them huge, and makes
scanned PDFs smaller.**

DjVu is a compact format for scanned books, but many devices and programs
only read PDF. The usual converters turn every page into one big picture,
so a 10 MB DjVu book can become a 500 MB PDF. dejapdf keeps the text and
the pictures of each page apart, as DjVu does, so the PDF comes out about
the same size as the DjVu, usually within 20%.

It can also shrink a PDF made from a scanner: typically 2–15 times smaller,
sometimes much more, with the pages looking the same.

What you keep:

- the text stays sharp and searchable (the hidden text layer is kept);
- the table of contents becomes PDF bookmarks;
- links, page numbers and document information are carried over.

The PDFs are tested in Adobe Acrobat and Reader, the Firefox viewer
(pdf.js), MuPDF, Poppler (Okular, Evince) and Ghostscript.

## Install

With [Go](https://go.dev/dl/) 1.27 or later:

```
go install github.com/sergev/dejapdf/cmd/dejapdf@latest
```

This puts `dejapdf` in `~/go/bin`.

## Use

Convert a DjVu book:

```
dejapdf book.djvu book.pdf
```

Make a scanned PDF smaller:

```
dejapdf scan.pdf smaller.pdf
```

When it finishes, dejapdf prints the new size:

```
book.pdf: 1531 kbytes, 1.09x in 0.5s
```

A PDF is never made larger: if there is nothing to gain, it is copied as
it is. Password-protected PDFs are not supported.

### Options

| Option | What it does |
|---|---|
| `-p 1-10,15` | convert only these pages |
| `-q 1.2` | quality of the colour pictures: higher is better but larger |
| `-lossless` | for scanned PDFs: keep every letter exactly as scanned (see below) |
| `-v` | show more detail about sizes and time |
| `-quiet` | print only warnings and errors |
| `-dump` | show what is inside the file, and for a PDF what would be done |
| `-version` | print the version |

When making a scanned PDF smaller, dejapdf by default draws letters that
look the same from one picture, which saves a lot of space. It takes care
not to mix up similar characters such as 6 and 8. If you want every letter
kept exactly as scanned, add `-lossless`; the file will be somewhat larger.

## How well it works

- Nine scanned DjVu books became PDFs 1.06–1.18 times the DjVu size.
  The common converter `ddjvu` made PDFs 10–97 times the DjVu size.
- Thirty scanned PDFs, 1.1 GB in all, became 200 MB: 5.5 times smaller.
  Gray and colour scans shrink the most; black-and-white scans already
  compressed well shrink little.

Details, with a table per book, are in
[docs/PDF_Rewriting.md](docs/PDF_Rewriting.md).

## For developers

Building from a clone, running the tests and the tools they need are
described in [CONTRIBUTING.md](CONTRIBUTING.md). How dejapdf works is told
in five articles in [docs/](docs/).

## License

MIT, see [LICENSE](LICENSE). dejapdf is written in Go and needs nothing
but Go's standard library.
