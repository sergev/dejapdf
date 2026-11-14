"""IW44: where each band's coefficients sit in a 32x32 block (spec 10.6.3)."""

from svg import Figure, LARGE, GREY, INK

# Band of coefficient index i (spec table 2).
BANDS = [(0, 16), (16, 32), (32, 48), (48, 64), (64, 128), (128, 192), (192, 256),
         (256, 512), (512, 768), (768, 1024)]
COLOURS = ["#d62728", "#ff7f0e", "#e8b80c", "#bcbd22", "#2ca02c", "#17becf", "#1f77b4",
           "#9467bd", "#c5b0d5", "#e7e1ef"]
C = 20
N = 16  # draw the lower-left 16x16 quarter


def band(i):
    for k, (lo, hi) in enumerate(BANDS):
        if lo <= i < hi:
            return k


def position(i):
    """Row and column of index i: row bits b1 b3 b5 b7 b9, column bits b0 b2 b4 b6 b8 (MSB first)."""
    bit = lambda n: (i >> n) & 1
    row = (bit(1) << 4) | (bit(3) << 3) | (bit(5) << 2) | (bit(7) << 1) | bit(9)
    col = (bit(0) << 4) | (bit(2) << 3) | (bit(4) << 2) | (bit(6) << 1) | bit(8)
    return row, col


def main():
    f = Figure(940, 420, LARGE)
    x0, y0 = 30, 50
    f.text(x0 + N * C / 2, 30, "lower-left quarter of a 32×32 block", 12, weight="bold")
    for i in range(1024):
        r, c = position(i)
        if r >= N or c >= N:
            continue
        # Row 0 is the bottom of the block.
        x, y = x0 + c * C, y0 + (N - 1 - r) * C
        k = band(i)
        f.rect(x, y, C, C, fill=COLOURS[k], stroke="white", width=0.6)
        f.text(x + C / 2, y + C / 2 + 4, str(k), 8, fill="white" if k < 8 else INK)
    f.text(x0, y0 + N * C + 20, "origin (row 0, column 0): lower left", 9.5, fill=GREY, anchor="start")
    # Legend.
    lx = 400
    f.text(lx, 64, "band", 10, anchor="start", weight="bold")
    f.text(lx + 70, 64, "indices", 10, anchor="start", weight="bold")
    f.text(lx + 190, 64, "count", 10, anchor="start", weight="bold")
    f.text(lx + 260, 64, "row, column", 10, anchor="start", weight="bold")
    steps = ["0, 0 (mod 8)", "0, 4 (mod 8)", "4, 0 (mod 8)", "4, 4 (mod 8)", "0, 2 (mod 4)",
             "2, 0 (mod 4)", "2, 2 (mod 4)", "even, odd", "odd, even", "odd, odd"]
    for k, (lo, hi) in enumerate(BANDS):
        y = 88 + k * 22
        f.rect(lx, y - 12, 14, 14, fill=COLOURS[k], stroke="none")
        f.text(lx + 24, y, str(k), 10, anchor="start", mono=True)
        f.text(lx + 70, y, f"{lo}–{hi - 1}", 10, anchor="start", mono=True)
        f.text(lx + 190, y, str(hi - lo), 10, anchor="start", mono=True)
        f.text(lx + 260, y, steps[k], 10, anchor="start", fill=GREY)
    f.lines(lx + 255, 340, ["Band 0 holds the coarsest samples; each later",
                            "band adds detail at a finer step. A slice codes",
                            "one band for every block, Y then Cb, Cr."], 10, fill=INK)
    f.write()


main()
