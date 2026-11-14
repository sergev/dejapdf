"""Refinement templates: JBIG2 GRTEMPLATE 0 and 1 (T.88 figures 12-13) next to DjVu's JB2."""

from svg import Figure, LARGE, BLUE, GREEN, ORANGE, YELLOW, GREY, INK

C = 26

# (dx, dy) cells; "X" marks the coded pixel (current) or its aligned twin (reference).
GR = {
    "GRTEMPLATE 0: 13 pixels": (
        [(0, -1), (1, -1), (-1, 0)], [(-1, -1)],
        [(0, -1), (1, -1), (-1, 0), (0, 0), (1, 0), (-1, 1), (0, 1), (1, 1)], [(-1, -1)]),
    "GRTEMPLATE 1: 10 pixels": (
        [(-1, -1), (0, -1), (1, -1), (-1, 0)], [],
        [(0, -1), (-1, 0), (0, 0), (1, 0), (0, 1), (1, 1)], []),
    "DjVu JB2 refinement: 11 pixels": (
        [(-1, -1), (0, -1), (1, -1), (-1, 0)], [],
        [(0, -1), (-1, 0), (0, 0), (1, 0), (-1, 1), (0, 1), (1, 1)], []),
}


def group(f, ox, oy, cells, at, colour, centre):
    for dx, dy in cells:
        f.rect(ox + dx * C, oy + dy * C, C, C, fill=colour[0], stroke=colour[1])
    for dx, dy in at:
        f.rect(ox + dx * C, oy + dy * C, C, C, fill=YELLOW[0], stroke=YELLOW[1])
        f.text(ox + dx * C + C / 2, oy + dy * C + C / 2 + 4, "RA", 8)
    if centre:
        f.rect(ox, oy, C, C, fill=ORANGE[0], stroke=ORANGE[1], width=1.5)
    f.text(ox + C / 2, oy + C / 2 + 4, "X", 9, weight="bold")


def main():
    f = Figure(940, 300, LARGE)
    for k, (title, (cur, cat, ref, rat)) in enumerate(GR.items()):
        x = 40 + k * 310
        f.text(x + 120, 30, title, 11.5, weight="bold")
        group(f, x + 40, 100, cur, cat, BLUE, True)
        group(f, x + 170, 100, ref, rat, GREEN, False)
        f.text(x + 53, 176, "new bitmap", 10, fill=GREY)
        f.text(x + 183, 176, "reference", 10, fill=GREY)
    f.lines(470, 220, ["Blue: already-coded pixels of the new bitmap; green: the reference bitmap, aligned",
                       "on the coded pixel X; RA: movable AT pixels. DjVu's template is GRTEMPLATE 1",
                       "plus the reference pixel at (−1, +1): close, but contexts do not carry over."], 10,
            fill=INK)
    f.write()


main()
