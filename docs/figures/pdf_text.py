"""Fitting an invisible word to its DjVu box with Tf, Td and Tz."""

from svg import Figure, BLUE, YELLOW, GREY, INK, RED


def main():
    f = Figure(840, 300)
    x0, base, h = 130, 150, 90  # box left, baseline, box height (pixels of the figure)
    word = "Привет"
    # Natural advance: 6 glyphs × 0.5 em × size; size = h.
    nat = len(word) * 0.5 * h
    box = 470
    # The DjVu word box.
    f.rect(x0, base - h, box, h, fill=YELLOW[0], stroke=YELLOW[1], width=1.4)
    f.text(x0 + box / 2, base - h - 10, "word box from TXTz: x0 y0 x1 y1", 12)
    # Natural glyph cells.
    for i in range(len(word)):
        f.rect(x0 + i * nat / len(word), base - h + 6, nat / len(word), h - 6, stroke=GREY, dash="3 2")
        f.text(x0 + (i + 0.5) * nat / len(word), base - 22, word[i], 30, fill="#bbbbbb")
    # Stretched cells.
    for i in range(len(word) + 1):
        x = x0 + i * box / len(word)
        f.line(x, base, x, base + 10, stroke=BLUE[1], width=1.2)
    f.line(x0, base, x0 + box, base, stroke=RED, width=1.4)
    f.text(x0 + box + 8, base + 4, "baseline", 11, fill=RED, anchor="start")
    f.rect(x0 - 3, base - 3, 6, 6, fill=RED, stroke="none")
    f.text(x0 - 6, base + 16, "Td x0 y0", 11, fill=RED, anchor="end", mono=True)
    # Dimensions.
    f.arrow(x0, base + 30, x0 + nat, base + 30, both=True, stroke=GREY)
    f.text(x0 + nat / 2, base + 46, "natural: n × DW/1000 × size", 11.5, fill=GREY)
    f.arrow(x0, base + 62, x0 + box, base + 62, both=True, stroke=BLUE[1])
    f.text(x0 + box / 2, base + 78, "box width = k/100 × natural", 11.5, fill=BLUE[1])
    f.arrow(x0 + box + 20, base - h, x0 + box + 20, base, both=True, stroke=GREY)
    f.text(x0 + box + 26, base - h / 2, "size = y1 − y0", 11.5, fill=GREY, anchor="start")
    f.lines(x0 + box + 26, base + 34, ["BT 3 Tr", "/F1 size Tf", "k Tz", "x0 y0 Td", "<0420…> Tj", "ET"],
            11.5, anchor="start", mono=True)
    f.write()


main()
