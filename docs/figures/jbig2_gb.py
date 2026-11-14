# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""The four generic-region templates (T.88 figures 3-6), AT pixels at their nominal places."""

from svg import Figure, LARGE, BLUE, ORANGE, GREEN, GREY, INK

C = 26
# (dx, dy) of the fixed pixels; AT pixels separately (nominal locations, table 5).
FIXED = {
    0: [(-1, -2), (0, -2), (1, -2), (-2, -1), (-1, -1), (0, -1), (1, -1), (2, -1),
        (-4, 0), (-3, 0), (-2, 0), (-1, 0)],
    1: [(-1, -2), (0, -2), (1, -2), (2, -2), (-2, -1), (-1, -1), (0, -1), (1, -1), (2, -1),
        (-3, 0), (-2, 0), (-1, 0)],
    2: [(-1, -2), (0, -2), (1, -2), (-2, -1), (-1, -1), (0, -1), (1, -1), (-2, 0), (-1, 0)],
    3: [(-3, -1), (-2, -1), (-1, -1), (0, -1), (1, -1), (-4, 0), (-3, 0), (-2, 0), (-1, 0)],
}
AT = {0: [(3, -1), (-3, -1), (2, -2), (-2, -2)], 1: [(3, -1)], 2: [(2, -1)], 3: [(2, -1)]}
TITLES = {0: "GBTEMPLATE 0: 16 pixels", 1: "1: 13 pixels", 2: "2: 10 pixels", 3: "3: 10 pixels, 2 rows"}


def template(f, ox, oy, t):
    """Origin (ox, oy) is the cell of the coded pixel."""
    for dx, dy in FIXED[t]:
        f.rect(ox + dx * C, oy + dy * C, C, C, fill=BLUE[0], stroke=BLUE[1])
    for k, (dx, dy) in enumerate(AT[t]):
        x, y = ox + dx * C, oy + dy * C
        f.rect(x, y, C, C, fill=GREEN[0], stroke=GREEN[1])
        f.text(x + C / 2, y + C / 2 + 4, f"A{k + 1}", 8.5)
    f.rect(ox, oy, C, C, fill=ORANGE[0], stroke=ORANGE[1], width=1.5)
    f.text(ox + C / 2, oy + C / 2 + 4, "X", 9, weight="bold")


def main():
    f = Figure(940, 320, LARGE)
    spots = [(130, 100, 0), (470, 100, 1), (790, 100, 2), (300, 240, 3)]
    for ox, oy, t in spots:
        rows = 1 if t == 3 else 2
        f.text(ox + C / 2, oy - rows * C - 16, TITLES[t], 11.5, weight="bold")
        template(f, ox, oy, t)
    f.lines(700, 216, ["X: the pixel being coded", "blue: fixed neighbours", "green: AT pixels, movable",
                       "outside the bitmap = 0"], 10, fill=INK)
    f.write()


main()
