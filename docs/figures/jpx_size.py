"""Size study: PSNR of OpenJPEG's JP2 against the IW44 decode, at multiples of the IW44 size."""

from svg import Figure, LARGE, GREY, INK

MULT = [0.5, 0.75, 1.0, 1.5, 2.0]
# PSNR in dB, measured with opj_compress -r / opj_decompress (see the article, §10).
SERIES = [
    ("Klabnik p1 BG", "#2a78d6", [49.4, 53.8, 55.8, 57.2, 57.9]),
    ("corpus BG", "#eb6834", [41.1, 44.1, 46.5, 49.7, 51.6]),
    ("Kaihlavirta cover", "#1baf7a", [38.0, 41.8, 44.8, 48.9, 51.5]),
    ("Yunyi cover 2325×2850", "#eda100", [35.6, 38.7, 41.1, 45.6, 48.7]),
    ("corpus FG 213×275", "#e87ba4", [33.7, 36.4, 38.0, 41.8, 44.4]),
]
X0, X1, Y0, Y1 = 90, 640, 40, 380   # plot box
LO, HI = 30, 60                      # dB


def px(m):
    return X0 + (m - 0.5) / 1.5 * (X1 - X0)


def py(db):
    return Y1 - (db - LO) / (HI - LO) * (Y1 - Y0)


def main():
    f = Figure(940, 450, LARGE)
    # Grid and axes.
    for db in range(LO, HI + 1, 5):
        f.line(X0, py(db), X1, py(db), stroke="#e6e6e6")
        f.text(X0 - 10, py(db) + 5, str(db), 10, fill=GREY, anchor="end")
    for m in MULT:
        f.line(px(m), Y1, px(m), Y1 + 5, stroke=GREY)
        f.text(px(m), Y1 + 24, f"{m:g}×", 10, fill=GREY)
    f.line(X0, Y1, X1, Y1, stroke=GREY)
    f.text(X0 - 60, Y0 - 14, "PSNR vs the IW44 decode, dB", 10, fill=INK, anchor="start")
    f.text((X0 + X1) / 2, Y1 + 50, "JP2 size as a multiple of the IW44 chunks' size", 10, fill=INK)
    # 1x reference.
    f.line(px(1.0), Y0, px(1.0), Y1, stroke=GREY, dash="4 3")
    # Series: 2px lines, markers, direct labels at the right end.
    label_y = []
    for name, colour, vals in SERIES:
        d = "M" + " L".join(f"{px(m):.1f},{py(v):.1f}" for m, v in zip(MULT, vals))
        f.path(d, stroke=colour, width=2)
        for m, v in zip(MULT, vals):
            f._add(f'<circle cx="{px(m):.1f}" cy="{py(v):.1f}" r="4.5" fill="{colour}" stroke="white" '
                   f'stroke-width="2"/>')
        y = py(vals[-1]) + 5
        while any(abs(y - u) < 18 for u in label_y):
            y += 18
        label_y.append(y)
        f.text(X1 + 14, y, name, 10, fill=INK, anchor="start")
        f.line(X1 + 2, py(vals[-1]), X1 + 10, y - 5, stroke=colour, width=1.5)
    f.write()


main()
