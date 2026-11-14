"""JB2 pixel contexts: the direct template and the refinement template."""

from svg import Figure, LARGE, BLUE, GREEN, ORANGE, GREY, INK

C = 34


def grid(f, x, y, cells, colour, bold="P"):
    """cells: {(row, col): label}; row 0 is the row of P."""
    for (r, c), s in cells.items():
        cx, cy = x + c * C, y + r * C
        if s == "P":
            f.rect(cx, cy, C, C, fill=ORANGE[0], stroke=ORANGE[1], width=1.5)
        else:
            f.rect(cx, cy, C, C, fill=colour[0], stroke=colour[1])
        f.text(cx + C / 2, cy + C / 2 + 6, s, 10, weight="bold" if s in bold else "normal")


def main():
    f = Figure(940, 300, LARGE)
    # Direct: rows -2, -1, 0; columns -2..+2.
    x, y = 40, 60
    f.text(x + 2.5 * C, 30, "direct: 10 pixels", 12, weight="bold")
    direct = {(0, 1): "1", (0, 2): "2", (0, 3): "3",
              (1, 0): "4", (1, 1): "5", (1, 2): "6", (1, 3): "7", (1, 4): "8",
              (2, 0): "9", (2, 1): "10", (2, 2): "P"}
    grid(f, x, y, direct, BLUE)
    f.lines(x + 2.5 * C, y + 3 * C + 26, ["2 rows above, 2 pixels", "left; outside = white"],
            10, fill=GREY)
    # Refinement: current symbol and matching symbol.
    x = 290
    f.text(x + 1.5 * C, 30, "refinement: new", 12, weight="bold")
    grid(f, x, y, {(0, 0): "1", (0, 1): "2", (0, 2): "3", (1, 0): "4", (1, 1): "P"}, BLUE)
    x2 = 480
    f.text(x2 + 1.5 * C, 30, "+ library shape", 12, weight="bold")
    grid(f, x2, y, {(0, 1): "5", (1, 0): "6", (1, 1): "7", (1, 2): "8",
                    (2, 0): "9", (2, 1): "10", (2, 2): "11"}, GREEN, bold=("7",))
    f.lines(x2 - 30, y + 3 * C + 26, ["7 is where P is, the shapes aligned by",
                                     "their centres (even size: the left or",
                                     "lower of the two middle pixels)"], 10, fill=GREY)
    f.box(680, 60, 250, 102, BLUE)
    f.lines(805, 84, ["10 bits → 1024 contexts", "11 bits → 2048 contexts", "bit order: confirm in P4",
                      "against ddjvu"], 10, fill=INK)
    f.text(470, 284, "rows are coded top to bottom, pixels left to right; 1 = black", 10, fill=INK)
    f.write()


main()
