# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""A text region: strips, the S and T of each instance, and the coded deltas."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, GREY, INK, RED

ORIGIN_X, ORIGIN_Y = 40, 40
K = 6  # page pixels to figure pixels


def main():
    f = Figure(940, 470, LARGE)
    f.rect(ORIGIN_X, ORIGIN_Y, 600, 330, fill="white", stroke=GREY)
    f.text(ORIGIN_X + 6, ORIGIN_Y + 18, "region (0, 0) top left", 9.5, fill=GREY, anchor="start")
    # Two text lines; each instance: S of its left edge, T of its bottom row, width, height.
    lines = [(20, [(6, 9, 10), (17, 8, 7), (27, 10, 13), (40, 9, 10)]),
             (44, [(9, 9, 10), (20, 11, 10), (33, 8, 7), (43, 12, 10)])]
    blue = "#2060c0"
    for li, (t, inst) in enumerate(lines):
        ty = ORIGIN_Y + t * K
        f.line(ORIGIN_X, ty + K / 2, ORIGIN_X + 600, ty + K / 2, stroke=GREEN[1], dash="5 3")
        f.text(ORIGIN_X + 596, ty - 4, f"strip T = {t}", 9.5, fill=GREEN[1], anchor="end")
        prev_end = None
        for k, (s, w, h) in enumerate(inst):
            x, y = ORIGIN_X + s * K, ty - (h - 1) * K
            f.rect(x, y, w * K, h * K, fill=BLUE[0], stroke=BLUE[1])
            f._add(f'<circle cx="{x + K / 2}" cy="{ty + K / 2}" r="4" fill="{RED}"/>')
            if prev_end is not None:
                gap = s - prev_end
                f.arrow(ORIGIN_X + prev_end * K, ty + 22, x + K / 2, ty + 22, stroke=blue, width=1.2)
                f.text((ORIGIN_X + prev_end * K + x) / 2, ty + 40, f"{gap}", 9.5, fill=blue)
            else:
                f.text(x + 4, ty + 40, f"S = {s}", 9.5, fill=RED, anchor="start")
            prev_end = s + w - 1
    # Legend.
    lx = 670
    f.box(lx, 40, 250, 120, YELLOW)
    f.lines(lx + 125, 64, ["per strip: delta T (IADT)", "first S: delta from the", "previous strip's first S (IAFS)",
                           "then per instance: ID (IAID)"], 10, fill=INK)
    f.box(lx, 176, 250, 194, ("#fafafa", "#cccccc"))
    f.lines(lx + 125, 200, ["next S (IADS) = gap from", "the previous right edge:", "S − (S′ + W′ − 1) − DSOFFSET",
                            "", "OOB (IADS) ends the strip", "", "red dot: reference corner,", "BOTTOMLEFT"], 10,
            fill=INK)
    f.lines(470, 404, ["Strips of height 1, 2, 4 or 8 group instances by T; inside a strip each instance codes",
                       "its T offset (IAIT) unless the strip height is 1. All S and T values are in pixels."],
            10, fill=INK)
    f.write()


main()
