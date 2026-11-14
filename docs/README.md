# Docs

Articles:

- [DjVu_Format.md](DjVu_Format.md): the parts of DjVu that dejapdf
  decodes, from the container to JB2, IW44, text and annotations.
- [Portable_Document_Format.md](Portable_Document_Format.md): the parts of
  PDF that dejapdf writes.

Figures come from `figures/*.py`. `make docs` regenerates the figures and
the article PDFs; `make docs-pdf` only the PDFs.

# Specifications

| File | What | Used for | Source |
|---|---|---|---|
| `DjVu3Spec.djvu` | Lizardtech DjVu v3 spec (2005) | IFF, INFO, DIRM, ZP coder (App. 3), BZZ (App. 4), JB2 (App. 2), IW44 (App. 1) | https://www.sndjvu.org/DjVu3Spec.djvu |
| `T-REC-T.88-200002.pdf` | ITU-T T.88 (02/2000), JBIG2 | MQ coder (Annex E), generic region, symbol dictionary, text region | https://www.itu.int/rec/T-REC-T.88-200002-S |
| `T-REC-T.800-201906.pdf` | ITU-T T.800 (06/2019), JPEG 2000 Part 1 | `internal/jpx` encoder | https://www.itu.int/rec/T-REC-T.800-201906-S |
| `PDF32000_2008.pdf` | ISO 32000-1:2008 (PDF 1.7), Adobe's free copy | PDF writer, JBIG2Decode, JPXDecode, image masks | https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf |

Text of the DjVu spec: `djvutxt docs/DjVu3Spec.djvu`. Its figures (decoder
pseudo-code) are images; render with `ddjvu -page=N`.

## DjVu spec errata

Found while decoding streams written by DjVuLibre (`bzz -e`) and checked
against raw vectors from the `djvu-zp` encoder. `internal/zp` follows these.

Appendix 3, ZP coder:

- Table 9 is an OCR'd scan. Unreadable entries, reconstructed from mirror
  states with equal successors (later confirmed against the djvu-zp table): Δ97 = 0x0B5D, Δ99 = 0x078A, Δ123 = 0x08AD,
  λ140 = 170, Δ151 = 0x017A, Δ163 = 0x0117, Δ195 = 0x0DC8,
  θ49 = θ50 = 0x7D0F. The text layer also splits numbers (rows 90, 100, 188).
- Figure 1: when Z = A + Δ < 0x8000 and Z ≤ C, the result is the MPS with
  A := Z and no state change; adaptation (and the D clamp) happens only on
  the other path. The MPS test is C ≥ Z, not C > Z.
- Figure 2 (Z = 0x8000 + 3A/8) is the IW44 pass-through. BZZ
  uses Z = 0x8000 + A/2. Ties go to bit 0 (data does not decide this).

Appendix 4, BZZ: the estimation speed is one pass-through bit, plus a second
one only if the first is 1.

Found by comparing our parsing with `djvudump` and `djvused` on the corpus
and on real books:

- 8.3.2 DIRM: component type 3 (bits 0–5 of the flags) is a DJVI of shared
  annotations; djvudump shows it as `[S]`.
- 8.3.10 FGbz: the 3-byte blit count is raw, before the BZZ data; the BZZ
  data holds only the 2-byte indices.
- 8.3.5 TXTz: the 2-byte text start is not unused. Like x, y, w and h it is
  biased by 0x8000 and is a gap after the previous sibling's text (0 in all
  files seen). The text length is in bytes.
- 8.3.5 TXTz: a second or later *region* is placed from the previous
  sibling's lower-right corner, y up, like columns, words and characters.
  The spec lists only column, word and character. Checked with a
  `djvused set-txt` file holding all seven zone types.
- 8.3.4 ANTz: strings use C escapes such as `\n`, not only `\"`.
- 10.6.4 IW44: the full prediction step is printed like the lifting step,
  `c[k] -= (… + 16) >> 5`; an interpolating (4,4) filter needs
  `c[k] += (9*(c[k-1] + c[k+1]) - (c[k-3] + c[k+3]) + 8) >> 4`.
  Unconfirmed until the IW44 decoder (P6).
