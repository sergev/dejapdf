# JPEG 2000 Image Coding

What dejapdf needs to know about JPEG 2000 to re-encode DjVu's colour layers:
the coding pipeline, the parameters that set size and quality, the codestream
and the JP2 file that PDF's `/JPXDecode` reads, and how to check the result.
Numbers in parentheses are sections of ITU-T T.800 (06/2019), JPEG 2000 Part
1, [T-REC-T.800-201906.pdf](T-REC-T.800-201906.pdf). dejapdf does not write
a general codec: `internal/jpx` is a small encoder for exactly the case
below, with pieces of the Apache-2.0 `tmp/go-jpeg2000` (§12). This article
says what it writes and why. A claim marked *checked* was tested with OpenJPEG
(`opj_compress`, `opj_decompress`, `opj_dump`), MuPDF, Poppler and `qpdf`,
used as outside tools only. The other articles: [DjVu_Format.md](DjVu_Format.md),
[JBIG2_Coding.md](JBIG2_Coding.md),
[Portable_Document_Format.md](Portable_Document_Format.md).

What goes through JPEG 2000:

| DjVu layer | Pixels | PDF |
|:---|:---|:---|
| BG44 background | colour or grey, at its own resolution (often 1/3 of the page) | JPX image stretched over the page |
| FG44 foreground colour | colour, usually 1/12 of the page | JPX image in a pattern that fills the JBIG2 stencil |
| photo page (BG44 only) | full resolution | JPX image |
| BGjp, FGjp | already JPEG | passed through as DCTDecode, not re-encoded |

JPEG 2000 suits this job: like IW44 it is a wavelet coder, so it handles the
same smooth, low-resolution layers well, and PDF 1.5 and later support it.
JPEG is excluded by the project rules.

## 1. The pipeline

![The encoder, stage by stage. The lossy steps are the colour transform, the
9/7 wavelet, quantization and truncation; the byte budget is spent in tier 1
and rate control.](figures/jpx_pipeline.svg)

1. **DC shift and colour transform** (Annex G): unsigned samples become
   signed, and RGB becomes Y, Cb, Cr.
2. **Wavelet transform** (Annex F): each component is split into sub-bands.
3. **Quantization** (Annex E): each sub-band's coefficients are divided by a
   step size.
4. **Tier 1** (Annex D): each code-block (a 64×64 piece of a sub-band) is
   coded bit-plane by bit-plane with the MQ coder, giving a stream that can
   be cut after any coding pass.
5. **Rate control** (J.13): choose where to cut each code-block's stream so
   the whole fits a byte budget with the least distortion.
6. **Tier 2** (Annex B): the kept bytes are grouped into packets, each with a
   header saying which code-blocks contribute and how much.
7. **Codestream and file** (Annexes A, I): marker segments around the
   packets, then the JP2 boxes around the codestream.

## 2. Image model

An image is up to 16384 **components** on a **reference grid** (B.2). Each
component may be sub-sampled (XRsiz, YRsiz) and the image cut into **tiles**
(B.3). dejapdf uses the simplest case:

- 1 component (grey) or 3 (RGB), 8 bits, unsigned;
- no sub-sampling: the colour transform needs all three components on the
  same grid (Table A.17);
- one tile, the whole image (XTsiz = width, YTsiz = height), no offsets.

A layer is coded at its own size, for example 850×1100 for a 2550×3300 page
with a 1/3 background. The PDF scales it.

## 3. Colour transform

**DC level shift** (G.1): subtract 2^(depth−1) = 128 from every 8-bit sample.

**ICT**, the irreversible component transform (G.3), is the usual YCbCr:

```
Y  =  0.299 R + 0.587 G + 0.114 B
Cb = −0.16875 R − 0.33126 G + 0.5 B
Cr =  0.5 R − 0.41869 G − 0.08131 B
```

It goes with the 9/7 wavelet; the reversible RCT goes with the 5/3 wavelet,
for lossless work (Table A.17). dejapdf is lossy throughout: 9/7 + ICT. COD's
MCT byte set to 1 tells the decoder to undo the transform. A grey layer has
one component and no transform.

## 4. Wavelet transform

Each level splits a component into four **sub-bands** at half size: LL
(low-pass in both directions), HL, LH and HH. The next level splits LL again
(F.2). With NL levels there are 3·NL + 1 sub-bands and NL + 1 **resolution
levels**: r = 0 is NL·LL, and each higher r adds the three detail bands of
one level.

![Three levels of decomposition, and the resolution levels they form. A
decoder can stop at any resolution level and get a smaller image.](figures/jpx_subbands.svg)

The **9/7 irreversible** filter is four lifting steps and a scaling (F.3.8.2,
Table F.4). Forward, on a row or column x with even samples as low-pass and
odd samples as high-pass:

```
x[2n+1] += α (x[2n]   + x[2n+2])     α = −1.586134342059924
x[2n]   += β (x[2n−1] + x[2n+1])     β = −0.052980118572961
x[2n+1] += γ (x[2n]   + x[2n+2])     γ =  0.882911075530934
x[2n]   += δ (x[2n−1] + x[2n+1])     δ =  0.443506852043971
low = x[2n] / K,  high = x[2n+1] · K, K =  1.230174104914001
```

(The forward scaling is the inverse of F.3's synthesis scaling; F.4.8.2.)
At the edges, the signal is extended by mirroring without repeating the edge
sample (F.3.7). Vertical, then horizontal, level by level (F.4). The default
is NL = 5, which leaves the LL band 1/32 of the size. For small layers, NL
must stay low enough for the LL band to have at least a pixel (OpenJPEG
refuses a 16×8 image with 5 levels, *checked*).

IW44 uses a different wavelet, the (4,4) interpolating filter, with a
different coefficient layout, so DjVu's coefficients cannot be carried over.
dejapdf decodes IW44 to pixels and codes them again.

## 5. Quantization

Each sub-band b has a step size Δb, sent as a 5-bit exponent εb and an 11-bit
mantissa μb (E.1.1.1):

```
Δb = 2^(Rb − εb) · (1 + μb / 2048),   Rb = depth + log2(gain b)
```

The gains are 1 for LL, 2 for HL and LH, and 4 for HH (Table E.1). The
encoder divides each coefficient by Δb and drops the fraction (E.2). QCD sends
either all the (ε, μ) pairs ("expounded", style 2) or only the LL one
("derived", style 1), from which the others follow (E-5). It also sends the
number of **guard bits**, usually 2, which keep the coefficients from
overflowing.

With rate control, the step size only sets the finest detail an encoder could
keep. The actual size comes from truncation (§7). A common choice is steps
near the 9/7 filter's synthesis norm of each band (J.2), with all detail
available and truncation doing the work.

## 6. Tier 1: coding a code-block

Each sub-band is cut into **code-blocks**, 64×64 by default (COD: width and
height exponents minus 2; xcb + ycb ≤ 12, B.7). Each code-block is coded on
its own, with its own MQ coder whose contexts start fresh (D.4). The MQ coder
is the same as JBIG2's, with the same Qe table ([JBIG2 article,
§4](JBIG2_Coding.md#4-the-mq-arithmetic-coder)). Two things differ: the
initial states of some contexts (Table D.7: UNIFORM state 46, run-length 3,
all-zero-neighbour 4), and the end of the data. There is no `FF AC` marker;
the packet header gives the length, and the decoder pads with 0xFF (D.4.1).

The coefficients are coded as magnitude bit-planes, most significant first.
The all-zero top planes are skipped and their count is sent in the packet
header. Each plane is scanned in **stripes** of four rows, column by column,
and coded in up to three **passes**:

![The stripe scan of a code-block, and the passes of each bit-plane. The first
plane with a 1 has only a cleanup pass.](figures/jpx_codeblock.svg)

- **Significance propagation** (D.3.1): coefficients still zero that have a
  significant neighbour among their 8. The context (0–8) depends on how many
  horizontal, vertical and diagonal neighbours are significant, and on the
  sub-band type (Table D.1). A newly significant coefficient's sign follows,
  coded with one of 5 contexts from its neighbours' signs (D.3.2).
- **Magnitude refinement** (D.3.3): one more bit of each coefficient that was
  already significant (contexts 14–16).
- **Cleanup** (D.3.4): all the rest. Four coefficients of a column with no
  significant neighbours are coded as one run-length bit; if it is 1, two
  UNIFORM bits give the position of the first 1.

Every pass end is a possible **truncation point**. The encoder notes the
bytes and the distortion reduction at each one, for rate control. The bytes
are those output so far plus 3, which lets the decoder finish the pass
(as in OpenJPEG; *checked* in `internal/mq` tests), capped at the final
length and never ending on 0xFF. The distortion is exact: each coefficient's
magnitude keeps 6 fraction bits below the quantization index, and the
decoder reconstructs at the middle of the known interval. The code-block
style byte in COD stays 0: no bypass, no per-pass reset or termination, no
segmentation symbols (Table A.19).

## 7. Rate control, layers and packets

Rate control picks a truncation point for every code-block (J.13). The goal
is the least total distortion for a total size R ≤ Rmax. The classic method
is PCRD-opt: keep the passes whose distortion drop per byte, ΔD/ΔR, is at
least λ, and search for the λ that just fits the budget (J-13).

![Six code-blocks, their passes drawn by size, and one cut each. A layer is
one set of cuts; one packet carries a precinct's share of one
layer.](figures/jpx_layers.svg)

A **layer** is one set of cuts. Several layers make a progressive stream, each
improving on the last; a PDF image is read whole, so dejapdf uses **one
layer**.

The kept bytes travel in **packets** (B.9): one per layer, resolution level,
component and precinct. A precinct is a region of a resolution level; one
precinct per resolution level is the default (PPx = PPy = 15, B.6). A packet
has a header and a body. The header is bit-packed, with a 0 bit stuffed after
any 0xFF byte (B.10.1). It holds:

- a bit: is the packet empty?
- per code-block: is it included? For a first inclusion this is a **tag
  tree** (B.10.2) over the precinct's code-blocks, which also codes the
  number of all-zero top bit-planes (B.10.5);
- the number of new passes (Table B.4: `0` = 1, `10` = 2, `11xx` = 3–5, …);
- the byte length, in Lblock + ⌊log2 passes⌋ bits, where Lblock starts at 3
  and grows by a unary prefix (B.10.7).

The body is the code-block bytes, in the same order. With one layer and
LRCP progression (layer, resolution, component, position; Table A.16), the
packets come lowest resolution first.

## 8. Codestream

A codestream is marker segments (Annex A). A marker is two bytes, FF xx; a
marker segment adds a 2-byte length that counts itself but not the marker.
What dejapdf writes, in order:

| Marker | Code | Contents |
|:---|:---|:---|
| SOC | FF4F | start of codestream (A.4.1) |
| SIZ | FF51 | Rsiz 0; grid size; offsets 0; tile = grid; Csiz; per component: Ssiz (depth − 1 = 7), XRsiz = YRsiz = 1 (A.5.1); length 38 + 3·Csiz |
| COD | FF52 | Scod 0; progression 0 (LRCP), layers 1, MCT 1 (or 0 for grey); levels NL, code-block 4, 4 (64×64), style 0, transform 0 = 9/7 (A.6.1); length 12 |
| QCD | FF5C | Sqcd: guard bits × 32 + style (0x42: 2 guard bits, expounded); then 2 bytes per sub-band, ε << 11 \| μ (A.6.4) |
| SOT | FF90 | tile 0, tile-part length Psot (from SOT to the end of the data), part 0 of 1 (A.4.2) |
| SOD | FF93 | then the packets (A.4.3) |
| EOC | FFD9 | end of codestream (A.4.4) |

*Checked* byte for byte against an `opj_compress` file: COD `FF52 000C 00 00
0001 01 02 04 04 00 00`, QCD `FF5C 0011 42` and 7 step sizes for 2 levels.

## 9. The JP2 file

PDF wants a JP2 file, not a bare codestream. A raw codestream displays in
MuPDF, but Poppler complains ("first box must be JPEG 2000 signature box").
JP2 is a sequence of **boxes**, each a 4-byte length (counting the whole box),
a 4-byte type, and contents (I.4):

![A JP2 file around its codestream: four boxes, 85 bytes before the codestream
starts. The values shown are from a 64×32 RGB file made by
OpenJPEG.](figures/jpx_file.svg)

| Box | Bytes | Contents |
|:---|:---|:---|
| `jP  ` signature | 12 | `0D 0A 87 0A` (I.5.1) |
| `ftyp` file type | 20 | brand `jp2 `, minor version 0, compatibility list `jp2 ` (I.5.2) |
| `jp2h` header | 8 + 22 + 15 | a superbox holding the next two (I.5.3) |
| `ihdr` image header | 22 | height, width (4 bytes each), components (2), BPC = depth − 1 (1), compression 7, colourspace unknown 0, IPR 0 (I.5.3.1) |
| `colr` colour | 15 | method 1 (enumerated), precedence 0, approximation 0, EnumCS 16 = sRGB or 17 = greyscale (I.5.3.3) |
| `jp2c` codestream | 8 + n | the codestream (I.5.4) |

The order is fixed: signature, ftyp, jp2h before jp2c (I.2.2). A greyscale
JP2 is the same with one component and EnumCS 17.

## 10. Parameters for DjVu layers

The size study took the IW44 layers of the corpus and of three books, decoded
them at their own size with `ddjvu -mode=background` (or `foreground`), and
coded each with `opj_compress -r` at 0.5 to 2 times the size of its IW44
chunks. Then it measured the PSNR of the JP2 decode against the IW44 decode:

![PSNR of the JPEG 2000 copy against the IW44 decode, for JP2 sizes from half
to twice the IW44 size. At 1×, every layer is above 38 dB, and most above
41 dB.](figures/jpx_size.svg)

| Layer | Size | IW44 bytes | 0.5× | 0.75× | 1× | 1.5× | 2× |
|:---|:---|---:|---:|---:|---:|---:|---:|
| corpus BG | 850×1100 | 16 560 | 41.1 | 44.1 | 46.5 | 49.7 | 51.6 |
| corpus FG | 213×275 | 5 372 | 33.7 | 36.4 | 38.0 | 41.8 | 44.4 |
| corpus grey BG | 850×1100 | 9 662 | 47.2 | 51.4 | 54.3 | 58.0 | 60.6 |
| Kaihlavirta cover BG | 850×1100 | 67 041 | 38.0 | 41.8 | 44.8 | 48.9 | 51.5 |
| Yunyi cover BG | 2325×2850 | 313 392 | 35.6 | 38.7 | 41.1 | 45.6 | 48.7 |
| Klabnik p1 BG | 700×925 | 2 404 | 49.4 | 53.8 | 55.8 | 57.2 | 57.9 |

What follows for dejapdf:

- **Budget = the IW44 size.** At 1× the copy is 38–56 dB from the IW44
  decode, which is visually the same: the IW44 picture is already the
  lossy original, and JPEG 2000 at the same size reproduces it closely. This
  keeps the layer at 1× its DjVu size, as the project's 1–1.5× target needs.
  A `-q` option can scale the budget.
- **Generation loss, and the default 1.2×.** PSNR against the IW44 decode
  says how closely the copy follows IW44, not what re-encoding costs against
  the scan. A second study started from originals (photos and document
  scans, 600–1550 pixels wide): c44 made the IW44 layer, internal/jpx coded
  its decode at q × the IW44 size, and both were measured against the
  original.

  | IW44 rate | IW44 | q = 0.8 | 1 | 1.2 | 1.5 | JPX of the original, 1× |
  |:---|---:|---:|---:|---:|---:|---:|
  | c44 default (8 images) | 31.7–37.0 | −0.4 to −2.1 | −0.3 to −1.0 | −0.1 to −0.6 | 0 to −0.3 | +1.1 to +2.9 |
  | c44 `-slice 74,84` (4) | 26.8–28.2 | −0.4 to −2.3 | −0.2 to −1.5 | −0.1 to −1.0 | 0 to −0.5 | +0.7 to +1.9 |

  The IW44 column is in dB; the others are dB relative to it. JPEG 2000 is
  the better codec: coding the original at the same size beats IW44 by
  1–3 dB. But the copy also has to reproduce IW44's own errors, and that
  costs up to 1.5 dB at 1×. On photos the difference is not visible at 1×.
  On scanned text in a background layer, a faint halo around the letters is
  visible at 1× and nearly gone at 1.2×; 1.5× looks the same as IW44. The
  encoder settings do not change this: 6 or 7 levels instead of 5, and base
  steps of half or twice the size, all give the same PSNR to 0.01 dB.

  dejapdf therefore defaults to `-q 1.2`. Of the colour layers' bytes in
  the books, this adds 20%. Whole books become 1.06–1.17× the DjVu instead
  of 0.99–1.11×; one book with many palette stencils is 1.29× (Portable_Document_Format.md §9). `-q 1` gives the smallest
  files, and `-q 1.5` the closest copy.
- **Rate control is required.** go-jpeg2000 has no rate control: its
  `CompressionRatio` option is declared but unused, and `Quality` only sets
  the step sizes from an error bound. At its lowest Quality, on the corpus
  background, it writes 107 KB (6.5× IW44) at 52 dB. At the same PSNR,
  OpenJPEG needs 26 KB. On the Kaihlavirta cover the figures are 214 KB and
  108 KB. internal/jpx adds PCRD-opt truncation to a byte budget (§7, §12).
- **Uniform layers.** Many book pages have a background of one flat colour
  (white paper). IW44 codes it in 79–1117 bytes; the smallest JP2 is about
  260–330 bytes. dejapdf should test for a single colour and paint it, or
  nothing for white, instead of embedding an image.
- **Small layers**: 85 bytes of JP2 boxes plus about 120 bytes of marker
  segments (SIZ, COD, a QCD with 16 step sizes, SOT) per image. They matter
  only for tiny layers.
- **Settings**: 9/7 + ICT; one tile; one layer; 64×64 code-blocks; LRCP;
  NL = 5, lowered for small images until the LL band is at least a few
  pixels; 2 guard bits; expounded quantization.

## 11. In the PDF

```
<< /Type /XObject /Subtype /Image /Width 850 /Height 1100
   /Filter /JPXDecode /Length … >>
```

- `/ColorSpace` and `/BitsPerComponent` may be left out: the reader takes
  them from the JP2 (`colr`, `ihdr`). Giving `/ColorSpace /DeviceRGB` or
  `/DeviceGray` also works (*checked*: RGB and grey JP2, with and without
  `/ColorSpace`, render in MuPDF and Poppler; `qpdf --check` passes).
- JPXDecode needs PDF 1.5 or later in the header.
- FG44: the JPX image is the cell of a tiling pattern that fills the JBIG2
  stencil, which has the page's full resolution (see [PDF article
  §8](Portable_Document_Format.md#8-masks); `/Mask` renders badly in
  Poppler).
- No `/SMaskInData`: there is no alpha channel. Encoders that take an RGBA
  image may add a fourth component; go-jpeg2000 does for `image.RGBA`
  (*checked*), so `internal/jpx` should take planar component data instead.

## 12. What `internal/jpx` is

The upstream go-jpeg2000 encoder is general (tiles, precincts, layers,
HTJ2K, float images, assembly) and its tier 1 records only per-bit-plane
byte counts, with no distortion, so PCRD-opt cannot sit on top of it.
`internal/jpx` is therefore written for the one configuration of §10, about
1300 lines:

| File | Contents | Source |
|:---|:---|:---|
| `dwt.go` | forward (and test-only inverse) 9/7 lifting; 1-D synthesis norms measured by inverting an impulse | T.800 F.4; norms checked against OpenJPEG's table, which scales high bands by 2 |
| `t1.go` | three passes, contexts, run mode; bytes and distortion per pass | T.800 Annex D |
| `t2.go` | packet-header bit writer, tag tree encoder, pass-count codes; one-layer packets | go-jpeg2000 `t2_encode.go` (Apache-2.0, header kept) |
| `jpx.go` | DC shift, ICT, step sizes, guard bits, PCRD-opt, SIZ COD QCD SOT, JP2 boxes | T.800 Annexes A, E, G, I, J.13 |
| `internal/mq` | MQ encoder with JPEG 2000 and JBIG2 flushes | `tmp/jbig2` (Apache-2.0, header kept) |

Rate control: each code-block's passes are reduced to their convex hull
(decreasing ΔD/ΔR); a binary search over all hull slopes finds the lowest
threshold whose file, packet headers included, fits the budget. Distortion
is weighted by Δb², the band's synthesis norm², and for colour the inverse
ICT column norm² (√3, 1.805, 1.573), so it is squared error in RGB. Step
sizes are Δb = 1/‖band synthesis‖ (in sample units), fine enough that
truncation, not quantization, sets the size; with every pass kept the error
is about 1/12 per sample.

Results with the same layers and the same file sizes as `opj_compress -r`
(PSNR against the IW44 decode, internal/jpx / OpenJPEG):

| Layer | 0.5× | 1× | 2× |
|:---|---:|---:|---:|
| corpus BG | 41.16 / 41.13 | 46.48 / 46.44 | 51.60 / 51.57 |
| corpus FG | 19.92 / 19.87 | 27.40 / 27.04 | 34.03 / 34.01 |
| corpus grey BG | 47.26 / 47.18 | 54.37 / 54.29 | 60.63 / 60.61 |
| Kaihlavirta cover BG | 37.99 / 37.96 | 44.80 / 44.78 | 51.50 / 51.47 |
| Yunyi cover BG | 35.61 / 35.58 | 41.14 / 41.10 | 48.72 / 48.72 |
| Klabnik p1 BG | 50.18 / 49.37 | 55.84 / 55.85 | 57.86 / 57.83 |

(The FG reference here is the IW44 decode itself; §10's FG row used
`ddjvu -mode=foreground`, which paints through the mask.) An 850×1100 RGB
layer encodes in about 80 ms, the 2325×2850 Yunyi cover in 0.4 s.

Borrowed files keep their copyright header and are credited in `NOTICE`.

## 13. Checking it

| Tool | Checks |
|:---|:---|
| `opj_dump -i file.jp2` | boxes, SIZ, COD, QCD as parsed by OpenJPEG |
| `opj_decompress -i file.jp2 -o file.ppm` | the file decodes; compare with the source for PSNR |
| `ddjvu -mode=background -size=WxH` | the IW44 layer at its own size, the reference |
| `mutool draw`, `pdftoppm` | the image inside a PDF |
| `qpdf --check` | PDF structure |

Size and PSNR per page against the IW44 sizes, as in §10, make a good
regression test for the rate control.
