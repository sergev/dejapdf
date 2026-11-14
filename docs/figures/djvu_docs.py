# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""Bundled and indirect multipage documents; INCL points to a shared DJVI."""

from svg import Figure, LARGE, BLUE, GREEN, PURPLE, YELLOW, GRAY, GREY, INK


def main():
    f = Figure(940, 470, LARGE)
    # Bundled: one file.
    f.text(210, 26, "Bundled: one file", 15, weight="bold")
    f.box(20, 40, 380, 360, GRAY)
    f.text(32, 62, "FORM:DJVM", 12, anchor="start", mono=True, weight="bold")
    f.box(40, 74, 340, 50, YELLOW, "DIRM  bundled, 4 files\noffsets 0x54, 0xE02, …", 10, mono=True)
    f.box(40, 136, 340, 30, YELLOW, "NAVM  outline (optional)", 10, mono=True)
    ys = [180, 234, 288, 342]
    names = [("FORM:DJVI {dict.iff}", PURPLE, "Djbz, shared ANTz"),
             ("FORM:DJVU {p0001.djvu}", BLUE, "INCL dict.iff, Sjbz"),
             ("FORM:DJVU {p0002.djvu}", BLUE, "INCL dict.iff, Sjbz"),
             ("FORM:THUM {p0001.thumb}", GREEN, "TH44 per page")]
    for y, (name, colour, sub) in zip(ys, names):
        f.box(100, y, 290, 44, colour)
        f.text(245, y + 19, name, 10, mono=True)
        f.text(245, y + 37, sub, 9.5, fill=GREY)
        f.arrow(56, 124, 56, y + 22, stroke=GREY) if y == ys[0] else f.line(56, y - 32, 56, y + 22, GREY)
        f.arrow(56, y + 22, 98, y + 22, stroke=GREY)
    # INCL arrows in the bundle.
    for y in ys[1:3]:
        f.curve(392, y + 22, 412, y + 22, 412, 202, 392, 202, stroke=PURPLE[1], dash="4 3")
    f.text(416, 256, "INCL", 9.5, fill=PURPLE[1], anchor="start")
    # Indirect: several files.
    x = 500
    f.text(x + 210, 26, "Indirect: one file per component", 15, weight="bold")
    f.box(x, 40, 420, 96, GRAY)
    f.text(x + 12, 62, "index.djvu  FORM:DJVM", 12, anchor="start", mono=True, weight="bold")
    f.box(x + 20, 74, 380, 50, YELLOW, "DIRM  indirect, no offsets\nids = file names", 10, mono=True)
    files = [("dict.iff", PURPLE, "FORM:DJVI"), ("p0001.djvu", BLUE, "FORM:DJVU"),
             ("p0002.djvu", BLUE, "FORM:DJVU")]
    for k, (name, colour, form) in enumerate(files):
        y = 186 + k * 74
        f.box(x + 140, y, 250, 50, colour)
        f.text(x + 265, y + 21, name, 12, mono=True, weight="bold")
        f.text(x + 265, y + 40, form, 10, fill=GREY, mono=True)
        f.curve(x + 60, 124, x + 60, y + 25, x + 90, y + 25, x + 138, y + 25, stroke=GREY)
    f.text(x + 54, 300, "name", 9.5, fill=GREY, anchor="end")
    for k in (1, 2):
        y = 186 + k * 74
        f.curve(x + 392, y + 25, x + 420, y + 25, x + 420, 211, x + 392, 211, stroke=PURPLE[1], dash="4 3")
    f.lines(x + 210, 430, ["each component is a valid IFF file;",
                                "a lone FORM:DJVU is a one-page document"], 10.5, fill=INK)
    f.write()


main()
