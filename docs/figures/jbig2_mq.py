# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""The MQ encoder (T.88 Annex E): code one decision, renormalise, output bytes."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, ORANGE, PURPLE, GREY, INK


def main():
    f = Figure(940, 560, LARGE)
    # Encode one decision.
    f.box(20, 20, 420, 44, BLUE, "Qe = Qe(I(CX));  A = A − Qe", 10.5, mono=True)
    f.arrow(230, 64, 230, 86)
    f.box(130, 88, 200, 40, YELLOW, "D = MPS(CX)?", 10.5, mono=True)
    # MPS branch.
    f.arrow(130, 108, 90, 108)
    f.text(110, 100, "yes", 10, fill=GREY)
    f.box(10, 150, 250, 124, GREEN, "MPS\nif A ≥ 0x8000: C += Qe\nelse: if A < Qe: A = Qe\n"
          "      else C += Qe\n  I(CX) = NMPS; RENORME", 9.5, mono=True)
    f.line(90, 108, 90, 148, INK)
    f.head(90, 150, 1.5708, INK)
    # LPS branch.
    f.arrow(330, 108, 370, 108)
    f.line(370, 108, 370, 148, INK)
    f.head(370, 150, 1.5708, INK)
    f.text(350, 100, "no", 10, fill=GREY)
    f.box(275, 150, 240, 124, ORANGE, "LPS\nif A < Qe: C += Qe\nelse A = Qe\nif SWITCH: flip MPS\n"
          "I(CX) = NLPS; RENORME", 9.5, mono=True)
    f.text(260, 300, "the swap when A < Qe is the conditional exchange", 10, fill=GREY)
    # Renormalise and byte out.
    f.box(560, 20, 360, 96, PURPLE, "RENORME: repeat\nA <<= 1; C <<= 1; CT −= 1\nif CT = 0: BYTEOUT\n"
          "until A ≥ 0x8000", 10, mono=True)
    f.box(560, 136, 360, 150, PURPLE, "BYTEOUT (B = last byte out)\nif B = 0xFF: B = C >> 20 (7 bits)\n"
          "else if carry in C:\n   B += 1, and if B = 0xFF\n   stuff as above\nelse B = C >> 19 (8 bits)", 10,
          mono=True)
    f.arrow(740, 116, 740, 134)
    # Start and flush.
    f.box(20, 330, 440, 70, GRAY_BOX, "INITENC: A = 0x8000, C = 0, CT = 12\nall contexts: I = 0, MPS = 0", 10,
          mono=True)
    f.box(480, 330, 440, 116, GRAY_BOX, "FLUSH: set the low 16 bits of C to 1\nif that leaves the interval, "
          "drop 0x8000\ntwo BYTEOUTs, then the marker FF AC\n(an FF already out is reused)", 10, mono=True)
    f.lines(470, 488, ["Registers: A interval (16 bits), C code (28 bits), CT bits left before the next byte.",
                       "After an 0xFF byte the next byte carries 7 bits, so a carry can never ripple past it.",
                       "Checked: the Annex H test sequence encodes to the 30 bytes in the spec."], 10, fill=INK)
    f.write()


GRAY_BOX = ("#f5f5f5", "#999999")

main()
