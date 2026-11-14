# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""JB2 records fill one shape library and blit shapes onto the page."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, PURPLE, ORANGE, GRAY, GREY, INK

GLYPHS = {
    "a": [".###.", "....#", ".####", "#...#", ".####"],
    "e": [".###.", "#...#", "#####", "#....", ".###."],
    "n": ["####.", "#...#", "#...#", "#...#", "#...#"],
    "t": [".#...", "####.", ".#...", ".#...", "..##."],
}


def glyph(f, x, y, g, cell=5, colour=INK):
    for r, row in enumerate(GLYPHS[g]):
        for c, ch in enumerate(row):
            if ch == "#":
                f.rect(x + c * cell, y + r * cell, cell, cell, fill=colour, stroke="none")


def records(f, x, y, title, items):
    f.text(x + 145, y, title, 12, weight="bold")
    for k, (code, text, colour) in enumerate(items):
        yy = y + 14 + k * 30
        f.box(x, yy, 290, 26, colour)
        f.text(x + 8, yy + 18, code, 9.5, anchor="start", mono=True, weight="bold")
        f.text(x + 38, yy + 18, text, 10, anchor="start")
    return y + 14 + len(items) * 30


def main():
    f = Figure(940, 470, LARGE)
    # Djbz.
    records(f, 20, 26, "Djbz (shared, in a DJVI)", [
        ("0", "start", GRAY),
        ("2", "new shape → library only", PURPLE),
        ("2", "new shape → library only", PURPLE),
        ("11", "end of data", GRAY)])
    # Sjbz.
    records(f, 20, 220, "Sjbz (the page)", [
        ("9", "dictionary size 2 (from INCL)", GRAY),
        ("0", "start: page width, height", GRAY),
        ("1", "new shape → library and page", BLUE),
        ("7", "copy shape #1 to page", GREEN),
        ("7", "copy shape #0 to page", GREEN),
        ("3", "new shape → page only", ORANGE),
        ("11", "end of data", GRAY)])
    # Library.
    lx = 360
    f.text(lx + 60, 52, "library", 12, weight="bold")
    lib = [("a", "#0", "Djbz"), ("e", "#1", "Djbz"), ("n", "#2", "Sjbz")]
    for k, (g, idx, src) in enumerate(lib):
        yy = 70 + k * 56
        f.rect(lx, yy, 44, 44, fill="white", stroke=GREY)
        glyph(f, lx + 10, yy + 10, g)
        f.text(lx + 54, yy + 20, idx, 10, anchor="start", mono=True, weight="bold")
        f.text(lx + 54, yy + 38, src, 9.5, anchor="start", fill=GREY)
    f.arrow(312, 83, lx - 4, 92, stroke=PURPLE[1])
    f.arrow(312, 113, lx - 4, 148, stroke=PURPLE[1])
    f.arrow(312, 307, lx - 4, 204, stroke=BLUE[1])
    f.lines(lx + 50, 260, ["#3, #4, …", "would follow"], 9.5, fill=GREY)
    # Page.
    px = 480
    f.text(px + 225, 52, "page (blits)", 12, weight="bold")
    f.rect(px, 66, 450, 170, fill="white", stroke=GREY)
    blits = [("n", 30, 150), ("e", 72, 150), ("a", 114, 150), ("t", 156, 150)]
    for g, bx, by in blits:
        glyph(f, px + bx + 120, 66 + by - 60, g, colour=ORANGE[1] if g == "t" else INK)
    f.text(px + 225, 222, "blit: shape index + position", 10, fill=GREY)
    f.lines(px + 225, 270, ["A line's first shape is placed from the previous",
                            "line's first shape; the next ones from the previous",
                            "shape's right edge and the baseline. Pixels are OR'ed."], 10, fill=INK)
    f.box(px, 340, 450, 62, YELLOW)
    f.lines(px + 225, 364, ["In PDF: library → JBIG2 symbol dictionary,",
                            "blits → text region; Djbz → JBIG2Globals"], 10, fill=INK)
    f.write()


main()
