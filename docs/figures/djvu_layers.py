# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""A compound page: background, mask and foreground colour, then the composite."""

from svg import Figure, LARGE, GREY, INK

C = 9            # mask cell size
N = 18           # mask cells per side
GLYPH = ["..................",
         "..................",
         "..####.....####...",
         ".#....#...#....#..",
         ".#........#....#..",
         ".#........#....#..",
         ".#....#...#....#..",
         "..####.....####...",
         "..................",
         "..................",
         "..######..#....#..",
         "..#.......##...#..",
         "..#####...#.#..#..",
         "..#.......#..#.#..",
         "..#.......#...##..",
         "..######..#....#..",
         "..................",
         ".................."]
SIDE = N * C


def bg(r, c):
    """Background: a smooth paper-and-picture gradient, low resolution."""
    t = (r + c) / (2 * (N - 1))
    return "#%02x%02x%02x" % (int(250 - 40 * t), int(240 - 70 * t), int(215 - 120 * t))


def fg(r, c):
    """Foreground colour: dark red on top, dark blue below."""
    return "#b0302a" if r < 9 else "#2a4ab0"


def cells(f, x, y, step, colour):
    n = N * C // step
    for r in range(n):
        for c in range(n):
            f.rect(x + c * step, y + r * step, step, step, fill=colour(r * step // C, c * step // C),
                   stroke="none")
    f.rect(x, y, SIDE, SIDE, stroke=GREY, width=1)


def main():
    f = Figure(940, 330, LARGE)
    xs = [20, 230, 440, 720]
    y = 70
    heads = [("background", "BG44 at 1/3 resolution"), ("mask", "Sjbz at full resolution"),
             ("foreground colour", "FG44 at 1/12, or FGbz"), ("page", "")]
    for x, (h, sub) in zip(xs, heads):
        f.text(x + SIDE / 2, 28, h, 12.5, weight="bold")
        f.text(x + SIDE / 2, 52, sub, 10, fill=GREY)
    cells(f, xs[0], y, 3 * C, bg)
    cells(f, xs[1], y, C, lambda r, c: INK if GLYPH[r][c] == "#" else "white")
    cells(f, xs[2], y, 9 * C, fg)
    cells(f, xs[3], y, C, lambda r, c: fg(r, c) if GLYPH[r][c] == "#" else bg(r // 3 * 3, c // 3 * 3))
    # Operators between panels.
    for x, s in [(xs[1] - 24, "+"), (xs[2] - 24, "+"), (xs[3] - 60, "=")]:
        f.text(x + 4, y + SIDE / 2 + 8, s, 22, fill=GREY)
    f.lines(xs[3] + SIDE / 2, y + SIDE + 30, ["mask black: foreground", "mask white: background"], 11,
            fill=GREY)
    f.lines(xs[0] + SIDE / 2, y + SIDE + 30, ["paper, pictures,", "smooth colour"], 11, fill=GREY)
    f.lines(xs[1] + SIDE / 2, y + SIDE + 30, ["text and line art:", "shapes and blits"], 11, fill=GREY)
    f.lines(xs[2] + SIDE / 2, y + SIDE + 30, ["colour of the ink,", "one value per area"], 11, fill=GREY)
    f.write()


main()
