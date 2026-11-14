# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""BZZ decoding: per block, ZP bits to MTF ranks to the BWT buffer to bytes."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, PURPLE, ORANGE, GREY, INK


def main():
    f = Figure(940, 400, LARGE)
    stages = [
        (BLUE, "ZP decoder", ["one stream for", "the whole chunk"]),
        (YELLOW, "block header", ["size: 24 raw bits", "speed: 1 or 2 raw bits"]),
        (ORANGE, "rank coder", ["262 contexts,", "kept across blocks"]),
        (GREEN, "move-to-front", ["rank → byte;", "frequency-ordered"]),
        (PURPLE, "inverse BWT", ["one EOB marker", "per block"]),
    ]
    x, y, w, h = 20, 50, 160, 60
    for k, (colour, name, notes) in enumerate(stages):
        bx = x + k * 184
        f.box(bx, y, w, h, colour, name, 12, weight="bold")
        f.lines(bx + w / 2, y + h + 22, notes, 10.5, fill=GREY)
        if k:
            f.arrow(bx - 24, y + h / 2, bx - 2, y + h / 2)
    f.text(20, 28, "per block, until a block size of 0", 11, anchor="start", fill=INK)
    # Rank coding detail.
    f.text(20, 200, "Rank of the next byte in the MTF list", 12, anchor="start", weight="bold")
    rows = [("0", "ctx[c]", "c = previous rank if < 2, else 2"),
            ("1", "ctx[c+3]", ""),
            ("2–3", "ctx[6], then 1 bit", "ranges grow by powers of 2;"),
            ("4–7", "ctx[8], then 2 bits", "inner bits use a binary tree"),
            ("…", "…", "of contexts"),
            ("128–255", "ctx[132], then 7 bits", ""),
            ("256", "all tests fail", "end-of-block marker (EOB)")]
    for k, (r, how, note) in enumerate(rows):
        yy = 228 + k * 22
        f.text(30, yy, r, 9.5, anchor="start", mono=True)
        f.text(120, yy, how, 9.5, anchor="start", mono=True)
        f.text(370, yy, note, 10, anchor="start", fill=GREY)
    f.box(640, 290, 290, 100, YELLOW)
    f.lines(785, 312, ["inverse BWT: count each byte,", "number its occurrences, walk",
                       "back from the EOB position;", "a block yields size − 1 bytes"], 10, fill=INK)
    f.write()


main()
