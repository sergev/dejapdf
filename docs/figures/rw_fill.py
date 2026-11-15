# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""How much a pixel near the text counts when the background is filled."""

from svg import Figure, LARGE, BLUE, ORANGE, GREY, INK


def main():
    f = Figure(940, 330, LARGE)
    ox, oy, w, h = 80, 260, 520, 200
    f.arrow(ox, oy, ox + w + 30, oy)
    f.arrow(ox, oy, ox, oy - h - 30)
    f.text(ox + w + 30, oy + 60, "distance from the mask", 10, anchor="end")
    f.text(ox - 10, oy - h - 34, "weight", 10, anchor="end")
    r0, r1 = ox + 100, ox + 360
    f.path(f"M{ox},{oy} L{r0},{oy} L{r1},{oy - h} L{ox + w},{oy - h}", stroke=BLUE[1], width=2.5)
    for x, name, note in ((r0, "r0", "dpi/100"), (r1, "r1", "dpi/20")):
        f.line(x, oy, x, oy + 6)
        f.text(x, oy + 22, name, 10, mono=True)
        f.text(x, oy + 38, note, 9.5, fill=GREY)
    f.text(ox - 8, oy - h + 4, "1", 10, anchor="end")
    f.text(ox - 8, oy + 4, "0", 10, anchor="end")
    f.lines(ox + 50, oy - 100, ["blurred", "edge of", "the ink"], 9.5, fill=GREY)
    f.lines(ox + 150, oy - h + 2, ["ringing: counts less,", "and not at all if", "darker than the paper", "beyond r1 by 6"], 9.5, fill=ORANGE[1])
    f.lines(ox + w + 40, oy - h + 30, ["clean", "paper"], 9.5, fill=GREY, anchor="start")
    f.lines(ox + w + 40, oy - 80, ["cells with no", "weight: push-pull", "from coarser levels"], 9.5, fill=INK, anchor="start")
    f.write()


main()
