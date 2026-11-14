"""Rate control: each code-block's passes are cut where they stop paying, then packed into packets."""

from svg import Figure, LARGE, GREEN, PURPLE, GREY, INK, RED

# Pass sizes (bytes, drawn as heights) of six code-blocks, and the chosen cut (passes kept).
BLOCKS = [([10, 14, 18, 22, 26, 30, 36], 4), ([6, 8, 12, 16, 20], 3), ([12, 16, 22, 28, 34, 40, 46], 5),
          ([4, 6], 1), ([8, 12, 16, 20, 26], 3), ([14, 18, 24, 30, 36, 42], 4)]
K = 1.05


def main():
    f = Figure(940, 450, LARGE)
    f.text(20, 26, "Code-block bit-streams, one column each; passes bottom up", 12, weight="bold",
           anchor="start")
    base = 300
    for i, (passes, cut) in enumerate(BLOCKS):
        x = 40 + i * 76
        y = base
        for k, p in enumerate(passes):
            h = p * K
            y -= h
            keep = k < cut
            f.rect(x, y, 52, h, fill=GREEN[0] if keep else "#f2f2f2", stroke=GREEN[1] if keep else "#bbbbbb",
                   width=0.8)
        f.text(x + 26, base + 18, f"B{i + 1}", 10)
        # Cut line.
        ycut = base - sum(passes[:cut]) * K
        f.line(x - 6, ycut, x + 58, ycut, stroke=RED, width=2)
    f.text(510, 60, "kept (green) up to the red cut:", 10.5, anchor="start", weight="bold")
    f.lines(510, 82, ["each pass lowers the distortion by ΔD", "and costs ΔR bytes; keep the passes",
                      "whose ΔD/ΔR ≥ λ, and search for the λ", "whose total R fits the byte budget",
                      "(J.13). One layer: one cut per block.", "",
                      "Several layers = several cuts, for", "progressive display; dejapdf uses one."],
            10.5, fill=INK, anchor="start")
    # Packet.
    f.text(20, 344, "One packet = (layer, resolution, component, precinct)", 12, weight="bold", anchor="start")
    parts = [(PURPLE, 340, "header: included? new: zero bit-planes;\npasses (B.4 codes); byte lengths"),
             (GREEN, 280, "body: the bytes of each block's\nnew passes, in order")]
    x = 20
    for c, w, label in parts:
        f.box(x, 360, w, 56, c, label, 10)
        x += w
    f.lines(x + 16, 380, ["packet headers are bit-packed", "with tag trees; after 0xFF a 0 bit"], 10,
            fill=GREY, anchor="start")
    f.write()


main()
