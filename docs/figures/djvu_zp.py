# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""One ZP decode step, as verified against DjVuLibre streams."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, ORANGE, PURPLE, GREY, INK


def diamond(f, cx, cy, w, h, colour, label, size=11):
    f.polygon([(cx, cy - h / 2), (cx + w / 2, cy), (cx, cy + h / 2), (cx - w / 2, cy)],
              fill=colour[0], stroke=colour[1], width=1.2)
    f.lines(cx, cy - (label.count("\n")) * 8 + 5, label.split("\n"), size, mono=True)


def main():
    f = Figure(940, 600, LARGE)
    cx = 250
    f.box(cx - 190, 20, 380, 40, BLUE, "s = state[ctx];  Z = A + Δ(s)", 10, mono=True)
    f.arrow(cx, 60, cx, 84)
    diamond(f, cx, 130, 300, 90, YELLOW, "Z < 0x8000\nand Z ≤ C ?", 10)
    f.arrow(cx + 150, 130, 528, 130)
    f.text(cx + 175, 122, "yes", 11, fill=GREY)
    f.box(530, 106, 400, 48, GREEN, "MPS fast path: bit = ctx & 1\nA = Z, no state change", 10, mono=True)
    f.arrow(cx, 175, cx, 200)
    f.text(cx + 8, 194, "no", 11, fill=GREY, anchor="start")
    f.box(cx - 220, 200, 440, 40, BLUE, "Z = min(Z, 0x6000 + (Z + A)/4)", 10, mono=True)
    f.arrow(cx, 240, cx, 264)
    diamond(f, cx, 300, 220, 72, YELLOW, "C ≥ Z ?", 10)
    # MPS with adaptation.
    f.arrow(cx + 110, 300, 528, 300)
    f.text(cx + 135, 292, "yes", 11, fill=GREY)
    f.box(530, 264, 400, 72, GREEN, "MPS: bit = ctx & 1\nif A ≥ θ(s): ctx = μ(s)\nA = Z", 10, mono=True)
    # LPS.
    f.arrow(cx, 336, cx, 370)
    f.text(cx + 8, 360, "no", 11, fill=GREY, anchor="start")
    f.box(cx - 220, 370, 440, 72, ORANGE, "LPS: bit = 1 − (ctx & 1)\nctx = λ(s)\nA += 0x10000 − Z;  C += same", 10,
          mono=True)
    # Renormalise.
    f.box(530, 440, 400, 72, PURPLE, "renormalise: while A ≥ 0x8000:\nA = 2A − 0x10000\nC = 2C − 0x10000 + next bit",
          10, mono=True)
    f.arrow(730, 336, 730, 438)
    f.arrow(cx + 220, 406, 528, 470)
    f.lines(470, 548, ["A: interval register, C: code register, starting as the first two bytes;",
                      "past the end, input bits are 1. Pass-through: Z = 0x8000 + A/2 (BZZ)",
                      "or 0x8000 + 3A/8 (IW44), no context, no adaptation."], 10,
            fill=INK)
    f.text(730, 92, "the common case", 10, fill=GREY)
    f.write()


main()
