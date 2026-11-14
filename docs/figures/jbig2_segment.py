# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""A JBIG2 segment header, byte by byte, for a text region that uses two dictionaries."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, PURPLE, ORANGE, GRAY, GREY, INK

CELL = 46


def main():
    f = Figure(940, 420, LARGE)
    f.text(20, 26, "Header of segment 3: an immediate text region using segments 0 and 2", 12, weight="bold",
           anchor="start")
    fields = [
        (["00", "00", "00", "03"], BLUE, "segment number", "4 bytes"),
        (["06"], YELLOW, "flags", "bits 0–5 type (6); bit 6: 4-byte page field"),
        (["42"], ORANGE, "count and retain bits", "bits 5–7: 2 refs; bit 1: segment 0 is used again"),
        (["00", "02"], ORANGE, "referred-to segments", "1 byte each while number ≤ 256"),
        (["01"], PURPLE, "page association", "1 byte (4 if flag bit 6)"),
        (["00", "00", "00", "60"], GREEN, "data length", "4 bytes"),
    ]
    x = 20
    for cells, colour, _, _ in fields:
        for k, c in enumerate(cells):
            f.box(x + k * CELL, 44, CELL, 40, colour, c, 11, mono=True)
        x += len(cells) * CELL
    f.text(x + 10, 70, "data…", 10, fill=GREY, anchor="start")
    for k, (_, colour, name, sub) in enumerate(fields):
        y = 112 + k * 24
        f.rect(20, y - 13, 16, 16, fill=colour[0], stroke=colour[1])
        f.text(46, y, name, 10.5, anchor="start", weight="bold")
        f.text(250, y, sub, 10.5, anchor="start", fill=GREY)
    # Data part of a region segment.
    f.text(20, 280, "A region segment's data starts with the region info (7.4.1)", 12, weight="bold",
           anchor="start")
    parts = [(4, "width"), (4, "height"), (4, "x"), (4, "y"), (1, "op")]
    x = 20
    for n, name in parts:
        f.box(x, 296, n * 30, 36, GRAY, name, 10.5)
        x += n * 30
    f.lines(x + 16, 310, ["then the segment's own flags", "and the coded data"], 10, fill=GREY, anchor="start")
    f.text(470, 380, "All multi-byte fields are big-endian; x, y are the top-left corner on the page.", 10,
           fill=INK)
    f.write()


main()
