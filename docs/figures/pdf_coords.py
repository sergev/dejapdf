"""DjVu pixels to PDF points; an image's unit square stretched by cm."""

from svg import Figure, BLUE, GREEN, YELLOW, GREY, INK, RED


def main():
    f = Figure(880, 370)
    # Left: DjVu page in pixels, origin top left for layers (rows top-down).
    x0, y0, w, h = 75, 50, 180, 260
    f.text(x0 + w / 2, 28, "DjVu: W × H pixels", 14, weight="bold")
    f.box(x0, y0, w, h, YELLOW)
    for i in range(1, 10):
        f.line(x0, y0 + i * h / 10, x0 + w, y0 + i * h / 10, stroke="#e6d39a", width=0.6)
        f.line(x0 + i * w / 10, y0, x0 + i * w / 10, y0 + h, stroke="#e6d39a", width=0.6)
    f.text(x0 + w / 2, y0 + h + 22, "W = 2550 px", 12)
    f.lines(x0 - 8, y0 + h / 2, ["H =", "3300"], 12, anchor="end")
    f.text(x0 + w / 2, y0 + h + 40, "300 dpi", 12, fill=GREY)
    # Arrow.
    f.arrow(280, 180, 350, 180, width=1.5)
    f.lines(315, 165, ["× 72 / dpi"], 12)
    # Right: PDF page in points, origin bottom left.
    px, py, pw, ph = 380, 50, 200, 260
    f.text(px + pw / 2, 28, "PDF: MediaBox in points", 14, weight="bold")
    f.rect(px, py, pw, ph, fill="white", stroke=INK, width=1.2)
    f.arrow(px, py + ph, px + 60, py + ph, stroke=RED, width=1.4)
    f.arrow(px, py + ph, px, py + ph - 60, stroke=RED, width=1.4)
    f.text(px + 64, py + ph + 4, "x", 12, fill=RED, anchor="start")
    f.text(px - 4, py + ph - 64, "y", 12, fill=RED, anchor="end")
    f.text(px - 6, py + ph + 14, "(0, 0)", 11, fill=RED, anchor="end")
    f.text(px + pw + 6, py + 4, "(612, 792)", 11, anchor="start")
    f.text(px + pw / 2, py + ph + 22, "[0 0 612 792]", 12, mono=True)
    # An image placed by cm, inside the page.
    ix, iy, iw, ih = px + 40, py + 90, 120, 130
    f.box(ix, iy, iw, ih, BLUE)
    f.text(ix + iw / 2, iy + ih / 2 - 4, "image", 12)
    f.text(ix + iw / 2, iy + ih / 2 + 12, "w h", 11, fill=GREY)
    f.text(ix + 4, iy + 14, "row 0", 10, fill=GREY, anchor="start")
    f.text(ix + 4, iy + ih - 6, "row h−1", 10, fill=GREY, anchor="start")
    f.rect(ix - 3, iy + ih - 3, 6, 6, fill=INK, stroke="none")
    f.text(ix - 6, iy + ih + 14, "(e, f)", 11, anchor="end")
    # Unit square explanation.
    ux, uy = 700, 120
    f.text(ux + 75, 28, "image → page", 14, weight="bold")
    f.rect(ux, uy, 50, 50, fill=BLUE[0], stroke=BLUE[1])
    f.text(ux - 4, uy + 64, "(0,0)", 10, anchor="end")
    f.text(ux + 54, uy - 4, "(1,1)", 10, anchor="start")
    f.text(ux + 25, uy + 30, "unit", 11)
    f.arrow(ux + 60, uy + 25, ux + 100, uy + 25)
    f.rect(ux + 105, uy - 20, 60, 90, fill=BLUE[0], stroke=BLUE[1])
    f.lines(ux + 85, uy + 110, ["a b c d e f cm", "a = width, d = height", "(e, f) = lower left"], 11.5, mono=False)
    f.lines(ux + 85, uy + 175, ["q 612 0 0 792 0 0 cm", "/Bg Do Q"], 11.5, mono=True)
    f.text(ux + 85, uy + 215, "stretches Bg over the page", 11, fill=GREY)
    f.write()


main()
