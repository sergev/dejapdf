# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""The form XObject that takes the place of a gray or colour scan image."""

from svg import Figure, LARGE, BLUE, YELLOW, ORANGE, GRAY, GREY, INK, RED


def main():
    f = Figure(940, 430, LARGE)
    # The unit square: the image as the page draws it.
    x0, y0, s = 70, 40, 300
    f.rect(x0, y0, s, s, fill="#fbf6ea", stroke=INK, width=1.5)
    # Background, reduced by k and rounded up: overhangs right and bottom.
    f.rect(x0, y0, s + 14, s + 14, fill="none", stroke=BLUE[1], dash="5 3")
    f.text(x0, y0 + s + 40, "dashed: the background, overhanging; the BBox clips it", 9.5, anchor="start", fill=BLUE[1])
    # Stencils, each cropped to its colour's marks.
    f.rect(x0 + 30, y0 + 40, 240, 180, fill="none", stroke=INK, width=1.2)
    f.text(x0 + 150, y0 + 130, "black stencil", 10)
    f.rect(x0 + 60, y0 + 240, 150, 40, fill="none", stroke=RED, width=1.2)
    f.text(x0 + 135, y0 + 265, "red stencil", 10, fill=RED)
    f.text(x0, y0 - 12, "(0, 1)", 9.5, anchor="start", fill=GREY)
    f.text(x0 - 4, y0 + s + 4, "(0, 0)", 9.5, anchor="end", fill=GREY)
    f.text(x0 + s - 20, y0 + s + 18, "(1, 0)", 9.5, anchor="end", fill=GREY)
    # The form's content stream.
    f.box(430, 40, 490, 250, GRAY)
    f.lines(445, 70, [
        "<< /Type /XObject /Subtype /Form",
        "   /BBox [0 0 1 1]",
        "   /Resources << /XObject <<",
        "      /Bg … /S0 … /S1 … >> >> >>",
        "q 1.0035 0 0 1.0027 0 -0.0027 cm /Bg Do Q",
        "q 0 g 0.79 0 0 0.6 0.1 0.3 cm /S0 Do Q",
        "q .8 .1 .1 rg .5 0 0 .13 .2 .07 cm /S1 Do Q",
    ], 9, anchor="start", mono=True, spacing=1.8)
    f.lines(445, 320, [
        "Same object number as the image it replaces: the page's",
        "content (/Im0 Do, in the image's unit square) is unchanged.",
        "Stencil edges are inset by 1e-5 against pixel snapping.",
    ], 10, anchor="start")
    f.write()


main()
