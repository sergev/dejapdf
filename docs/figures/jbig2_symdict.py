"""A symbol dictionary: symbols grouped into height classes, coded as deltas."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, GREY, INK

C = 9  # pixel size

GLYPHS = {
    "a": [".###.", "....#", ".####", "#...#", ".####"],
    "e": [".###.", "#...#", "#####", "#....", ".###."],
    "o": [".###.", "#...#", "#...#", "#...#", ".###."],
    "m": ["##.#.", "#.#.#", "#.#.#", "#.#.#", "#.#.#"],
    "b": ["#....", "#....", "####.", "#...#", "#...#", "#...#", "####."],
    "d": ["....#", "....#", ".####", "#...#", "#...#", "#...#", ".####"],
    "l": ["#", "#", "#", "#", "#", "#", "#"],
    "g": [".####", "#...#", "#...#", ".####", "....#", "....#", ".###."],
}


def glyph(f, x, y, g):
    rows = GLYPHS[g]
    f.rect(x, y, len(rows[0]) * C, len(rows) * C, fill="white", stroke=GREY, width=0.6)
    for r, row in enumerate(rows):
        for c, ch in enumerate(row):
            if ch == "#":
                f.rect(x + c * C, y + r * C, C, C, fill=INK, stroke="none")


def main():
    f = Figure(940, 400, LARGE)
    classes = [(5, ["e", "a", "o", "m"]), (7, ["l", "b", "d", "g"])]
    y = 40
    prev_h = 0
    for h, names in classes:
        f.box(20, y, 900, 130, BLUE if h == 5 else GREEN)
        f.text(36, y + 26, f"height class {h}", 11, anchor="start", weight="bold")
        f.text(36, y + 50, f"delta height {h - prev_h:+d}", 10, anchor="start", mono=True)
        f.text(36, y + 72, "IADH", 9.5, anchor="start", fill=GREY)
        prev_h = h
        x = 250
        prev_w = 0
        for g in names:
            w = len(GLYPHS[g][0])
            gy = y + 16 + (7 - h) * C
            glyph(f, x, gy, g)
            f.text(x + w * C / 2, y + 102, f"{w - prev_w:+d}", 10, mono=True)
            prev_w = w
            x += max(w * C, 30) + 70
        f.text(x, y + 60, "OOB", 10, mono=True, anchor="start", weight="bold")
        f.text(x, y + 82, "ends the class", 9.5, fill=GREY, anchor="start")
        f.text(250, y + 122, "delta widths (IADW), each followed by the symbol's bitmap (generic coding)", 9.5,
               fill=GREY, anchor="start")
        y += 150
    f.box(20, 340, 900, 46, YELLOW)
    f.text(470, 368, "then the export flags: run lengths (IAEX) of not-exported / exported symbols, "
           "e.g. 0, 8", 10.5)
    f.write()


main()
