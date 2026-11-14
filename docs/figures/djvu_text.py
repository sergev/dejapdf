# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""TXTz zone placement: each zone is stored relative to its parent or previous sibling."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, GREY, INK, RED

BLUE_D = "#2060c0"
PALE = ("#fafafa", "#cccccc")


def dot(f, x, y, colour):
    f._add(f'<circle cx="{x}" cy="{y}" r="3.5" fill="{colour}"/>')


def main():
    f = Figure(940, 450, LARGE)
    # The page.
    f.rect(20, 20, 490, 410, fill="white", stroke=GREY)
    f.text(30, 42, "page", 10, anchor="start", fill=GREY)
    # Line 1 and its words.
    L1 = (50, 80, 420, 60)
    L2 = (80, 240, 390, 60)
    for (x, y, w, h), name in [(L1, "line 1"), (L2, "line 2")]:
        f.rect(x, y, w, h, fill=GREEN[0], stroke=GREEN[1], width=1.2)
        f.text(x + w - 6, y - 6, name, 9.5, anchor="end", fill=GREEN[1])
    words = [(70, 90, 110, 42, "Page"), (210, 96, 100, 36, "One:"), (340, 92, 110, 40, "Text")]
    for x, y, w, h, s in words:
        f.rect(x, y, w, h, fill=BLUE[0], stroke=BLUE[1], width=1.2)
        f.text(x + w / 2, y + h / 2 + 6, s, 12)
    f.rect(100, 252, 140, 40, fill=BLUE[0], stroke=BLUE[1], width=1.2)
    f.text(170, 278, "word", 12)
    # Rule 1: first child, from the parent's upper left, Y down.
    dot(f, L1[0], L1[1], RED)
    f.arrow(L1[0], L1[1], 70, 90, stroke=RED, width=1.5)
    dot(f, L2[0], L2[1], RED)
    f.arrow(L2[0], L2[1], 100, 252, stroke=RED, width=1.5)
    # Rule 3: next word, from the previous lower right to this lower left, Y up.
    for (x1, y1, w1, h1, _), (x2, y2, w2, h2, _) in zip(words, words[1:]):
        dot(f, x1 + w1, y1 + h1, BLUE_D)
        f.arrow(x1 + w1, y1 + h1, x2, y2 + h2, stroke=BLUE_D, width=1.5)
    # Rule 2: next line, from the previous lower left to this upper left, Y down.
    dot(f, L1[0], L1[1] + L1[3], "#b8860b")
    f.arrow(L1[0], L1[1] + L1[3], L2[0], L2[1], stroke="#b8860b", width=1.5)
    # Legend.
    lx = 530
    f.box(lx, 20, 400, 116, PALE)
    f.text(lx + 12, 44, "first child", 11, anchor="start", fill=RED, weight="bold")
    f.text(lx + 12, 64, "from the parent's upper left; Y down", 10, anchor="start")
    f.text(lx + 12, 94, "next page, para, line", 11, anchor="start", fill="#b8860b", weight="bold")
    f.text(lx + 12, 114, "previous lower left → upper left; Y down", 10, anchor="start")
    f.box(lx, 146, 400, 82, PALE)
    f.text(lx + 12, 170, "next column, region, word, char", 11, anchor="start", fill=BLUE_D, weight="bold")
    f.text(lx + 12, 192, "previous lower right → lower left; Y up", 10, anchor="start")
    f.text(lx + 12, 214, "(the spec forgets region)", 9.5, anchor="start", fill=GREY)
    f.box(lx, 238, 400, 192, YELLOW)
    f.lines(lx + 200, 264, ["zone record, 17 bytes:", "type 1 · x 2 · y 2 · w 2 · h 2", "text start 2 · text length 3",
                            "children 3", "", "x, y, w, h, start: value + 0x8000;",
                            "start: a gap after the previous sibling", "(or the parent's start); 0 in all files seen"], 10,
            fill=INK)
    f.write()


main()
