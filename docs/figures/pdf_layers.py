# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""A DjVu compound page as PDF painting operations, bottom to top."""

from svg import Figure, BLUE, GREEN, YELLOW, ORANGE, GREY, INK


def slab(f, x, y, w, colour):
    """A page seen in perspective: a parallelogram."""
    d = 60
    f.polygon([(x + d, y), (x + d + w, y), (x + w, y + 46), (x, y + 46)], fill=colour[0], stroke=colour[1], width=1.2)


def main():
    f = Figure(860, 420)
    rows = [  # bottom first: number, slab name, colour, lines of (text, mono)
        ("1", "background", GREEN, [("BG44 → JPX image, low resolution", False),
                                   ("q W 0 0 H 0 0 cm /Bg Do Q", True)]),
        ("2", "foreground", BLUE, [("one of:", False),
                                  ("FG44 → JPX image whose /Mask is the stencil", False),
                                  ("no FG44: the stencil painted in black (0 g)", False),
                                  ("FGbz palette: one stencil per colour, r g b rg", False)]),
        ("3", "hidden text", YELLOW, [("TXTz words in render mode 3", False),
                                      ("BT 3 Tr /F1 … Tf … Tj ET", True)]),
        ("4", "links", ORANGE, [("ANTz maparea → Link annotations", False),
                                ("in the page's /Annots, not the content", False)]),
    ]
    ys = [340, 225, 130, 40]
    for (n, name, col, text), y in zip(rows, ys):
        slab(f, 30, y, 220, col)
        f.text(170, y + 28, name, 13, weight="bold")
        f.text(340, y + 14, f"{n}.", 13, anchor="end", weight="bold")
        for i, (t, mono) in enumerate(text):
            f.text(350, y + 14 + i * 21, t, 11.5 if mono else 12.5, anchor="start", mono=mono,
                   fill="#335" if mono else INK)
    f.arrow(20, 385, 20, 50, stroke=GREY, width=1.3)
    f.lines(14, 20, ["painting", "order"], 11, fill=GREY, anchor="start")
    f.text(350, 410, "W × H: the page size in points", 11.5, fill=GREY, anchor="start")
    f.write()


main()
