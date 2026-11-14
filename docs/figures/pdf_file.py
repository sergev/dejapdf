# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

"""The minimal PDF of section 1: the body, the xref table pointing at each
object's offset, the trailer naming the catalog, startxref pointing at xref."""

from svg import Figure, BLUE, GREEN, YELLOW, PURPLE, GRAY, GREY, INK

LH = 21  # line height
X = 150  # left edge of the file text
LINES = [
    ("%PDF-1.4", "header", 0),
    ("1 0 obj", "body", 9), ("<< /Type /Catalog /Pages 2 0 R >>", "body", None), ("endobj", "body", None),
    ("2 0 obj", "body", 58), ("<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "body", None), ("endobj", "body", None),
    ("3 0 obj", "body", 115), ("<< /Type /Page /Parent 2 0 R ... /Contents 4 0 R >>", "body", None),
    ("endobj", "body", None),
    ("4 0 obj", "body", 222), ("<< /Length 27 >>", "body", None), ("stream", "body", None),
    ("0 0 1 rg 72 72 144 144 re f", "body", None), ("endstream", "body", None), ("endobj", "body", None),
    ("xref", "xref", 299), ("0 5", "xref", None),
    ("0000000000 65535 f", "xref", None), ("0000000009 00000 n", "xref", None),
    ("0000000058 00000 n", "xref", None), ("0000000115 00000 n", "xref", None),
    ("0000000222 00000 n", "xref", None),
    ("trailer", "trailer", None), ("<< /Size 5 /Root 1 0 R >>", "trailer", None),
    ("startxref", "trailer", None), ("299", "trailer", None), ("%%EOF", "trailer", None),
]
COLOURS = {"header": GRAY, "body": BLUE, "xref": YELLOW, "trailer": PURPLE}
NAMES = {"header": "header", "body": "body:\nindirect objects", "xref": "cross-reference\ntable",
         "trailer": "trailer"}


def main():
    top = 20
    f = Figure(810, top + LH * len(LINES) + 34)
    # Section bands with labels on the left.
    start = 0
    for i in range(1, len(LINES) + 1):
        if i == len(LINES) or LINES[i][1] != LINES[start][1]:
            sec = LINES[start][1]
            y0, y1 = top + start * LH, top + i * LH
            f.box(X - 8, y0 + 1, 520, y1 - y0 - 2, COLOURS[sec])
            f.lines(70, (y0 + y1) / 2 - (NAMES[sec].count("\n")) * 8 + 5, NAMES[sec].split("\n"), 13)
            start = i
    for i, (s, _, off) in enumerate(LINES):
        y = top + i * LH + 15.5
        f.text(X, y, s, 12, anchor="start", mono=True)
        if off is not None:
            f.text(X + 90, y, f"@{off}", 11, fill=GREY, anchor="start", mono=True)
    # xref entries point at the objects' "N 0 obj" lines.
    row = {s: i for i, (s, _, _) in enumerate(LINES)}
    for k, obj in enumerate(["1 0 obj", "2 0 obj", "3 0 obj", "4 0 obj"]):
        src = row["0000000009 00000 n"] + k
        ys, yd = top + src * LH + LH / 2, top + row[obj] * LH + LH / 2
        x0 = X + 540 + k * 14
        f.path(f"M{X + 300},{ys} L{x0},{ys} L{x0},{yd} L{X + 150},{yd}", stroke="#b8860b", width=1.1)
        f.head(X + 150, yd, 3.14159, "#b8860b")
    # startxref points at xref.
    ys, yd = top + row["299"] * LH + LH / 2, top + row["xref"] * LH + LH / 2
    f.path(f"M{X + 40},{ys} L{X + 610},{ys} L{X + 610},{yd} L{X + 150},{yd}", stroke="#9673a6", width=1.1)
    f.head(X + 150, yd, 3.14159, "#9673a6")
    # /Root points at the catalog.
    ys = top + row["<< /Size 5 /Root 1 0 R >>"] * LH + LH / 2
    yd = top + row["<< /Type /Catalog /Pages 2 0 R >>"] * LH + LH / 2
    f.path(f"M{X + 250},{ys} L{X + 640},{ys} L{X + 640},{yd} L{X + 330},{yd}", stroke=INK, width=1.1, dash="4 3")
    f.head(X + 330, yd, 3.14159, INK)
    f.text(X + 512, top + LH * len(LINES) + 24, "@n = byte offset from the file start", 11, fill=GREY, anchor="end")
    f.write()


main()
