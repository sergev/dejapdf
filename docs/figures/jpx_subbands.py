# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""Three DWT levels: sub-bands and resolution levels of one tile-component."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, ORANGE, GREY, INK

S = 320  # side of the tile


def main():
    f = Figure(940, 400, LARGE)
    x0, y0 = 30, 40
    colours = {3: ORANGE, 2: YELLOW, 1: GREEN}
    side = S
    # From the finest level (1) inwards to the coarsest (3).
    for lev in (1, 2, 3):
        h = side / 2
        c = colours[lev]
        for name, dx, dy in (("HL", h, 0), ("LH", 0, h), ("HH", h, h)):
            f.box(x0 + dx, y0 + dy, h, h, c, f"{lev}{name}", 13 - lev)
        side = h
    f.box(x0, y0, side, side, BLUE, "3LL", 9)
    f.text(x0 + S / 2, y0 + S + 28, "one tile-component after NL = 3 levels", 10.5, fill=GREY)
    # Resolution levels.
    lx = 400
    f.text(lx, 52, "resolution levels (B.5)", 12, weight="bold", anchor="start")
    rows = [(BLUE, "r = 0", "3LL", "1/8 size"),
            (ORANGE, "r = 1", "3HL 3LH 3HH", "1/4 size"),
            (YELLOW, "r = 2", "2HL 2LH 2HH", "1/2 size"),
            (GREEN, "r = 3", "1HL 1LH 1HH", "full size")]
    for k, (c, r, bands, size) in enumerate(rows):
        y = 70 + k * 46
        f.box(lx, y, 70, 34, c, r, 10.5)
        f.text(lx + 86, y + 22, bands, 10.5, anchor="start", mono=True)
        f.text(lx + 300, y + 22, f"decode up to here: {size}", 10, anchor="start", fill=GREY)
    f.lines(lx + 250, 286, ["First letter: filter along x, second: along y; HL is high-pass in x.",
                            "Sub-band gains (E.1): LL 1, HL and LH 2, HH 4 → the step",
                            "sizes and bit depths differ per band. QCD lists them in this",
                            "order: 3LL, 3HL, 3LH, 3HH, 2HL, …, 1HH."], 10, fill=INK)
    f.write()


main()
