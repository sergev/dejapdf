# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""The JBIG2 segments of a two-page dejapdf document, split into PDF streams."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, PURPLE, ORANGE, GRAY, GREY, INK


def seg(f, x, y, w, colour, num, name, sub):
    f.box(x, y, w, 50, colour)
    f.text(x + 10, y + 21, num, 10, anchor="start", mono=True, weight="bold")
    f.text(x + 40, y + 21, name, 10.5, anchor="start")
    f.text(x + 40, y + 41, sub, 9.5, anchor="start", fill=GREY)


def main():
    f = Figure(940, 470, LARGE)
    # Globals.
    f.text(20, 26, "JBIG2Globals stream (shared)", 12, weight="bold", anchor="start")
    f.box(20, 38, 420, 70, GRAY)
    seg(f, 30, 48, 400, PURPLE, "0", "symbol dictionary: Djbz", "page 0, all symbols exported")
    # Two page streams.
    for k, x in enumerate((20, 480)):
        p = k + 1
        f.text(x, 150, f"image stream for PDF page {p}", 12, weight="bold", anchor="start")
        f.box(x, 162, 440, 236, GRAY)
        seg(f, x + 10, 172, 420, YELLOW, "1", "page information", "page 1: size, default pixel 0, OR")
        seg(f, x + 10, 228, 420, BLUE, "2", "symbol dictionary: this page's own shapes", "page 1")
        seg(f, x + 10, 284, 420, GREEN, "3", "immediate text region: the blits", "page 1, refers to 0 and 2")
        seg(f, x + 10, 340, 420, ORANGE, "4", "immediate generic region", "page 1, a non-symbol bitmap")
    # References.
    f.curve(440, 309, 462, 309, 470, 73, 434, 73, stroke=PURPLE[1], dash="4 3")
    f.curve(900, 160, 900, 70, 560, 56, 442, 62, stroke=PURPLE[1], dash="4 3")
    f.lines(470, 424, ["Each image is decoded alone, with the globals: every page stream is page 1, and numbers",
                       "need only be unique within globals + one stream. No file header, no end segments."], 10, fill=INK)
    f.write()


main()
