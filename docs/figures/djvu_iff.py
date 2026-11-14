# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""IFF85 bytes of a single-page file: magic, FORM header, chunks, padding."""

from svg import Figure, LARGE, BLUE, YELLOW, PURPLE, GRAY, GREY, INK

CELL = 34  # width of one byte


def bytes_row(f, x, y, items):
    """items: (n bytes, label, colour, sub) drawn left to right."""
    for n, label, colour, sub in items:
        w = n * CELL
        f.box(x, y, w, 40, colour, label, 10, mono=True)
        if sub:
            f.text(x + w / 2, y + 58, sub, 10, fill=GREY)
        x += w
    return x


def main():
    f = Figure(940, 420, LARGE)
    f.text(20, 26, "Bytes of a one-page file", 14, weight="bold", anchor="start")
    x = bytes_row(f, 20, 44, [
        (4, "AT&T", GRAY, "magic"),
        (4, "FORM", PURPLE, "id"),
        (4, "00 00 68 A6", PURPLE, "size, big-endian"),
        (4, "DJVU", PURPLE, "form type"),
        (4, "INFO", BLUE, "id"),
        (4, "00 00 00 0A", BLUE, "size = 10"),
    ])
    f.text(x + 8, 70, "…", 14, anchor="start")
    # The size covers everything after itself.
    f.path("M428,112 L428,124 L920,124 L920,112", stroke=GREY)
    f.text(674, 146, "0x68A6 = 26790 bytes: the form type and all chunks inside", 11, fill=GREY)
    # Nested view.
    f.text(20, 192, "The same file as a tree (djvudump)", 14, weight="bold", anchor="start")
    rows = [
        (0, "FORM:DJVU [26790]", PURPLE, "the page; INFO comes first"),
        (1, "INFO [10]", BLUE, "size, version, dpi, gamma, rotation"),
        (1, "Sjbz [13133]", BLUE, "mask, JB2"),
        (1, "FG44 [185]", BLUE, "foreground colour, IW44"),
        (1, "BG44 [935]  BG44 [1672] …", BLUE, "background, IW44, refined in order"),
    ]
    y = 206
    for depth, label, colour, note in rows:
        x0 = 20 + depth * 34
        f.box(x0, y, 320 - depth * 34, 30, colour, label, 10, mono=True)
        f.text(354, y + 21, note, 10.5, fill=INK, anchor="start")
        y += 38
    f.box(690, 206, 230, 136, YELLOW)
    f.lines(805, 232, ["each chunk: 4-byte id,", "4-byte size, data,", "a 0x00 pad byte if the",
                       "size is odd (not counted)"], 10.5, fill=INK)
    f.lines(805, 370, ["FORM data: 4-byte", "type, then chunks"], 10.5, fill=GREY)
    f.write()


main()
