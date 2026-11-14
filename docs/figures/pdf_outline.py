"""The outline tree: First/Last down, Next/Prev across, Parent up."""

from svg import Figure, PURPLE, BLUE, GREY, INK

W, H = 136, 50


def main():
    f = Figure(900, 320)
    pos = {"root": (20, 135), "A": (230, 30), "B": (230, 220), "B1": (450, 175), "B2": (450, 262)}
    labels = {"root": "Outlines\nCount 4", "A": "Text\n#1", "B": "Pictures\nCount 2",
              "B1": "Background\n#2", "B2": "Colours\n#3"}
    for k, (x, y) in pos.items():
        f.box(x, y, W, H, PURPLE if k == "root" else BLUE, labels[k], 12)
    rx, ry = pos["root"]
    ax, ay = pos["A"]
    bx, by = pos["B"]
    cx, cy = pos["B1"]
    dx, dy = pos["B2"]
    # First / Last from the root.
    f.curve(rx + W, ry + 14, 190, ry + 14, 200, ay + H / 2, ax, ay + H / 2)
    f.text(186, 92, "First", 11, fill=GREY, anchor="end")
    f.curve(rx + W, ry + 36, 190, ry + 36, 200, by + H / 2, bx, by + H / 2)
    f.text(186, 222, "Last", 11, fill=GREY, anchor="end")
    # Next / Prev between Text and Pictures.
    f.arrow(ax + 45, ay + H, bx + 45, by)
    f.text(ax + 39, 140, "Next", 11, fill=GREY, anchor="end")
    f.arrow(ax + 90, by, ax + 90, ay + H)
    f.text(ax + 96, 140, "Prev", 11, fill=GREY, anchor="start")
    # Pictures' children.
    f.arrow(bx + W, by + 14, cx, cy + H / 2)
    f.text(410, 196, "First", 11, fill=GREY)
    f.arrow(bx + W, by + 36, dx, dy + H / 2)
    f.text(410, 290, "Last", 11, fill=GREY)
    f.arrow(cx + 45, cy + H, dx + 45, dy)
    f.arrow(cx + 90, dy, cx + 90, cy + H)
    # Notes.
    f.lines(620, 50, ["NAVM bookmarks map one to one:", "(\"Pictures\" \"#2\"", "  (\"Background\" \"#2\") …)"], 11.5,
            anchor="start", fill=INK)
    f.arrow(cx + W + 8, cy + H / 2, 612, cy + H / 2, stroke=GREY, dash="3 2")
    f.lines(620, cy + H / 2 + 4, ["each item: Parent → its parent,", "Title, Dest [page /XYZ left top null]",
                                  "Count: open descendants;", "negative = closed"], 11.5, fill=GREY, anchor="start")
    f.write()


main()
