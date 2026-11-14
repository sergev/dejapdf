# Docs

Articles:

- [Portable_Document_Format.md](Portable_Document_Format.md): the parts of
  PDF that dejapdf writes, with figures from `figures/*.py`. `make docs`
  regenerates the figures and the PDF; `make docs-pdf` only the PDF.

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
