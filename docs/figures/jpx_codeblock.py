# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""Tier 1: the stripe scan of a code-block, and the three passes per bit-plane."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, ORANGE, GREY, INK, RED

C = 22


def main():
    f = Figure(940, 380, LARGE)
    x0, y0, cols, rows = 30, 50, 8, 8
    f.text(x0 + cols * C / 2, 32, "scan order (D.1)", 12, weight="bold")
    for r in range(rows):
        for c in range(cols):
            f.rect(x0 + c * C, y0 + r * C, C, C, fill=BLUE[0] if r < 4 else GREEN[0],
                   stroke=BLUE[1] if r < 4 else GREEN[1], width=0.6)
    # Path through the first stripe, column by column.
    pts = []
    for stripe in (0, 1):
        for c in range(cols):
            for r in range(4):
                pts.append((x0 + c * C + C / 2, y0 + (stripe * 4 + r) * C + C / 2))
    d = "M" + " L".join(f"{x:.0f},{y:.0f}" for x, y in pts[:32])
    f.path(d, stroke=RED, width=1.4)
    f.head(pts[31][0], pts[31][1] + 4, 1.5708, RED)
    d = "M" + " L".join(f"{x:.0f},{y:.0f}" for x, y in pts[32:])
    f.path(d, stroke=RED, width=1.4, dash="3 2")
    f.lines(x0 + cols * C / 2, y0 + rows * C + 26, ["stripes of 4 rows; in each,", "column by column, top down"],
            10, fill=GREY)
    # Passes per bit-plane.
    px = 300
    f.text(px + 300, 32, "coding passes, most significant bit-plane first (D.3)", 12, weight="bold")
    passes = [
        ("first plane with a 1", [("cleanup", ORANGE)]),
        ("each later plane", [("significance\npropagation", BLUE), ("magnitude\nrefinement", YELLOW),
                              ("cleanup", ORANGE)]),
    ]
    y = 56
    for label, ps in passes:
        f.text(px, y + 26, label, 10.5, anchor="start", fill=INK)
        for k, (name, c) in enumerate(ps):
            f.box(px + 170 + k * 140, y, 130, 46, c, name, 9.5)
        y += 64
    notes = [
        (BLUE, "significance propagation", "zero coefficients with a significant neighbour; a new 1 is followed by its sign"),
        (YELLOW, "magnitude refinement", "one more bit of each coefficient that is already significant"),
        (ORANGE, "cleanup", "the rest; a column of 4 all-zero-context coefficients is one run-length bit"),
    ]
    y = 204
    for c, name, text in notes:
        f.rect(px, y - 12, 14, 14, fill=c[0], stroke=c[1])
        f.text(px + 22, y, name, 10, anchor="start", weight="bold")
        f.text(px + 22, y + 18, text, 9.5, anchor="start")
        y += 42
    f.text(px + 300, 342, "19 contexts: 0–8 significance (by sub-band type), 9–13 sign, 14–16 refinement,", 9.5,
           fill=GREY)
    f.text(px + 300, 362, "17 run-length, 18 UNIFORM; one MQ coder per code-block, reset at its start", 9.5,
           fill=GREY)
    f.write()


main()
