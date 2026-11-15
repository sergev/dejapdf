# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""How dejapdf re-encodes a scanned PDF: read, find the scans, plan, code, write."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, PURPLE, ORANGE, GRAY, GREY


def main():
    f = Figure(940, 420, LARGE)
    f.box(20, 170, 110, 60, GRAY, "input PDF", 11, weight="bold")
    f.box(160, 170, 120, 60, BLUE, "pdfread", 11, weight="bold")
    f.lines(220, 250, ["objects, xref,", "decryption,", "filters"], 9.5, fill=GREY)
    f.box(310, 170, 120, 60, BLUE, "scan", 11, weight="bold")
    f.lines(370, 250, ["images drawn,", "dpi, coverage;", "Plan"], 9.5, fill=GREY)
    f.arrow(130, 200, 158, 200)
    f.arrow(280, 200, 308, 200)
    # Three ways.
    rows = [
        (30, YELLOW, "bilevel", "components, alike shapes,", "→ a JBIG2 image"),
        (150, ORANGE, "gray, colour", "text mask, colours, background", "→ a form: JPX + stencils"),
        (270, GRAY, "keep", "inline, masked, CMYK, already", "JBIG2 or JPX → as it was"),
    ]
    for y, colour, name, how, what in rows:
        f.box(470, y, 120, 60, colour, name, 11, weight="bold")
        f.text(610, y + 25, how, 9.5, anchor="start")
        f.text(610, y + 45, what, 9.5, anchor="start", fill=GREY)
        f.curve(430, 200, 450, 200, 450, y + 30, 468, y + 30)
    f.box(470, 360, 450, 44, GREEN, "rewrite: all copied; replacements in place", 10.5)
    f.write()


main()
