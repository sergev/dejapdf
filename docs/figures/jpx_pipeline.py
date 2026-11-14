"""The JPEG 2000 encoder pipeline, from IW44 pixels to a JP2 file."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, ORANGE, PURPLE, GRAY, GREY


def main():
    f = Figure(940, 330, LARGE)
    stages = [
        (GRAY, "pixels", "RGB or gray,\nnative size"),
        (BLUE, "DC shift +\nICT", "Y Cb Cr\n(G.1, G.3)"),
        (BLUE, "9/7 DWT", "5 levels,\nsub-bands (F)"),
        (YELLOW, "quantize", "a step per\nsub-band (E)"),
        (ORANGE, "tier 1", "code-block\nbit-planes (D)"),
        (GREEN, "rate\ncontrol", "cut passes\nto fit (J.13)"),
        (PURPLE, "tier 2", "packets,\nheaders (B)"),
        (GRAY, "JP2", "markers (A),\nboxes (I)"),
    ]
    w, gap = 98, 18
    for k, (colour, name, note) in enumerate(stages):
        x = 20 + k * (w + gap)
        f.box(x, 50, w, 64, colour, name, 10.5, weight="bold")
        f.lines(x + w / 2, 140, note.split("\n"), 9.5, fill=GREY)
        if k:
            f.arrow(x - gap, 82, x - 2, 82)
    # Bracket the lossy steps.
    f.path("M136,200 L136,212 L698,212 L698,200", stroke=GREY)
    f.text(417, 236, "lossy: the ICT and the 9/7 round; quantization and truncation drop detail", 10, fill=GREY)
    f.path("M484,256 L484,268 L698,268 L698,256", stroke=GREY)
    f.text(591, 292, "where the byte budget is spent", 10, fill=GREY)
    f.write()


main()
