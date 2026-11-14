"""Three ways to mask an image: stencil, explicit /Mask, soft /SMask."""

from svg import Figure, BLUE, GREEN, PURPLE, GREY, INK

N = 6
C = 16
GLYPH = ["..##..",
         ".#..#.",
         "#....#",
         "######",
         "#....#",
         "#....#"]
PAGE = "#f4d7a1"  # what is already on the page


def grid(f, x, y, cell):
    for r in range(N):
        for c in range(N):
            f.rect(x + c * C, y + r * C, C, C, fill=cell(r, c), stroke="#bbbbbb", width=0.5)


def main():
    f = Figure(820, 350)
    cols = [("stencil", "/ImageMask true", "paints the fill colour", "where the mask is set"),
            ("explicit mask", "/Mask <stencil>", "paints the base image", "where the mask is set"),
            ("soft mask", "/SMask <gray image>", "mixes the image in", "by each sample's alpha")]
    for k, (name, key, l1, l2) in enumerate(cols):
        x = 30 + k * 270
        f.text(x + 105, 26, name, 15, weight="bold")
        f.text(x + 105, 50, key, 12, mono=True)
        # Inputs.
        if k == 0:
            grid(f, x, 74, lambda r, c: INK if GLYPH[r][c] == "#" else "white")
            f.text(x + N * C / 2, 186, "mask", 11, fill=GREY)
            f.rect(x + 130, 74 + 30, 36, 36, fill="#2060c0", stroke="none")
            f.text(x + 148, 186, "fill colour", 11, fill=GREY)
        elif k == 1:
            grid(f, x, 74, lambda r, c: INK if GLYPH[r][c] == "#" else "white")
            f.text(x + N * C / 2, 186, "mask", 11, fill=GREY)
            grid(f, x + 112, 74, lambda r, c: ["#e05050", "#e0a030", "#40a040", "#3070d0", "#9050c0", "#e05050"][c])
            f.text(x + 112 + N * C / 2, 186, "base image", 11, fill=GREY)
        else:
            grid(f, x, 74, lambda r, c: "#%02x%02x%02x" % ((51 * c,) * 3))
            f.text(x + N * C / 2, 186, "alpha", 11, fill=GREY)
            grid(f, x + 112, 74, lambda r, c: "#3070d0")
            f.text(x + 112 + N * C / 2, 186, "image", 11, fill=GREY)
        # Result over the page.
        rx, ry = x + 56, 206
        f.arrow(x + 105, 192, x + 105, ry - 4)

        def res(r, c, k=k):
            on = GLYPH[r][c] == "#"
            if k == 0:
                return "#2060c0" if on else PAGE
            if k == 1:
                return ["#e05050", "#e0a030", "#40a040", "#3070d0", "#9050c0", "#e05050"][c] if on else PAGE
            a = c / 5
            mix = [int(p * (1 - a) + q * a) for p, q in zip((0xf4, 0xd7, 0xa1), (0x30, 0x70, 0xd0))]
            return "#%02x%02x%02x" % tuple(mix)

        grid(f, rx, ry, res)
        f.lines(x + 105, ry + N * C + 18, [l1, l2], 11.5, fill=GREY)
    f.write()


main()
