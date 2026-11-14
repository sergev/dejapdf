"""The object graph of a two-page dejapdf output."""

from svg import Figure, BLUE, GREEN, YELLOW, PURPLE, ORANGE, GRAY, GREY

W, H = 112, 34


def main():
    f = Figure(880, 500)
    B = {}

    def node(name, x, y, colour, label):
        f.box(x, y, W, H, colour, label, 12)
        B[name] = (x, y)

    def edge(a, b, label=None, dash=None, how="down", lx=4, ly=-3):
        (xa, ya), (xb, yb) = B[a], B[b]
        if how == "down":
            x1, y1, x2, y2 = xa + W / 2, ya + H, xb + W / 2, yb
        elif how == "right":
            x1, y1, x2, y2 = xa + W, ya + H / 2, xb, yb + H / 2
        else:  # left
            x1, y1, x2, y2 = xa, ya + H / 2, xb + W, yb + H / 2
        f.arrow(x1, y1, x2, y2, dash=dash)
        if label:
            f.text((x1 + x2) / 2 + lx, (y1 + y2) / 2 + ly, label, 10.5, fill=GREY, anchor="start")

    node("trailer", 20, 20, GRAY, "trailer")
    node("info", 20, 100, GRAY, "Info")
    node("catalog", 200, 20, PURPLE, "Catalog")
    node("outlines", 400, 20, PURPLE, "Outlines")
    node("item", 600, 20, PURPLE, "outline items")
    node("pages", 300, 100, PURPLE, "Pages")
    node("p1", 140, 190, BLUE, "Page 1")
    node("p2", 480, 190, BLUE, "Page 2")
    node("annot", 700, 190, ORANGE, "Link annot")
    node("c1", 20, 290, BLUE, "Contents 1")
    node("r1", 160, 290, YELLOW, "Resources 1")
    node("font", 330, 290, YELLOW, "Font: Type0")
    node("r2", 500, 290, YELLOW, "Resources 2")
    node("c2", 640, 290, BLUE, "Contents 2")
    node("bg1", 40, 380, GREEN, "Image: JPX")
    node("m1", 170, 380, GREEN, "Image: JBIG2")
    node("cid", 330, 380, YELLOW, "CIDFont")
    node("m2", 490, 380, GREEN, "Image: JBIG2")
    node("bg2", 620, 380, GREEN, "Image: JPX")
    node("glob", 330, 450, GREEN, "JBIG2Globals")
    edge("trailer", "catalog", "Root", how="right", lx=-12)
    edge("trailer", "info", "Info")
    edge("catalog", "outlines", "Outlines", how="right", lx=-22)
    edge("outlines", "item", "First…", how="right", lx=-16)
    edge("catalog", "pages", "Pages", lx=8)
    edge("pages", "p1", "Kids", lx=-40)
    edge("pages", "p2")
    edge("p1", "c1", "Contents", lx=-58)
    edge("p1", "r1", "Resources", lx=6)
    edge("p2", "r2")
    edge("p2", "c2")
    edge("p2", "annot", "Annots", how="right", lx=-18)
    edge("r1", "font", how="right")
    edge("r2", "font", how="left")
    f.text(306, 300, "Font", 10.5, fill=GREY)
    edge("font", "cid")
    edge("r1", "bg1")
    edge("r1", "m1", "XObject", lx=6)
    edge("r2", "m2")
    edge("r2", "bg2")
    f.arrow(170 + W / 2, 380 + H, 330, 450 + H / 2)
    f.arrow(490 + W / 2, 380 + H, 330 + W, 450 + H / 2)
    f.text(560, 450, "DecodeParms", 10.5, fill=GREY, anchor="start")
    f.arrow(600 + W / 2, 20 + H, 140 + W, 190 + H / 2, dash="3 3")
    f.text(500, 120, "Dest [page …]", 10.5, fill=GREY, anchor="start")
    f.write()


main()
