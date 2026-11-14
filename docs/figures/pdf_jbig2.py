"""DjVu's shared Djbz dictionary and per-page Sjbz become JBIG2 segments:
global segments in one JBIG2Globals stream, page segments in each image."""

from svg import Figure, BLUE, GREEN, YELLOW, PURPLE, GREY, INK

ROW = 92  # vertical step of the pages


def seg(f, x, y, w, label, sub):
    f.box(x, y, w, 44, YELLOW)
    f.text(x + w / 2, y + 19, label, 11.5)
    f.text(x + w / 2, y + 36, sub, 10, fill=GREY)


def main():
    f = Figure(910, 450)
    # DjVu side.
    f.text(135, 26, "DjVu (JB2)", 15, weight="bold")
    f.box(20, 44, 230, 60, PURPLE, "FORM:DJVI  Djbz\nshared shapes", 12)
    for k in range(3):
        y = 136 + k * ROW
        f.box(20, y, 230, 60, BLUE, f"FORM:DJVU page {k + 1}\nINCL → Djbz,  Sjbz", 12)
        f.arrow(36, y, 36, 104, stroke=GREY, dash="3 2")
    # PDF side.
    f.text(610, 26, "PDF (JBIG2Decode)", 15, weight="bold")
    f.box(340, 44, 560, 60, GREEN)
    f.text(352, 80, "JBIG2Globals", 12, anchor="start", weight="bold")
    seg(f, 520, 52, 370, "symbol dictionary", "page association 0")
    for k in range(3):
        y = 136 + k * ROW
        f.box(340, y, 560, 60, GREEN)
        f.text(352, y + 24, f"image {k + 1}", 12, anchor="start", weight="bold")
        f.text(352, y + 44, "/JBIG2Globals", 10, anchor="start", mono=True, fill=GREY)
        seg(f, 520, y + 8, 116, "page info", "type 48")
        seg(f, 642, y + 8, 120, "local symbols", "optional")
        seg(f, 768, y + 8, 122, "text region", "type 6: blits")
        f.path(f"M342,{y + 40} C312,{y + 40} 312,74 336,74", stroke=INK, dash="3 2")
        f.head(338, 74, 0, INK)
    f.arrow(260, 74, 330, 74, width=1.5)
    for k in range(3):
        f.arrow(260, 160 + k * ROW, 338, 160 + k * ROW, width=1.5)
    f.lines(620, 418, ["no file header, no end-of-page or end-of-file segments;",
                       "page segments carry page association 1"], 11.5, fill=GREY)
    f.write()


main()
