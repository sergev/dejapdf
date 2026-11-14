"""Classic xref table vs PDF 1.5 object stream + cross-reference stream."""

from svg import Figure, BLUE, GREEN, YELLOW, PURPLE, GREY


def main():
    f = Figure(900, 380)
    # Left: classic.
    f.text(195, 26, "PDF 1.4: objects, xref table", 15, weight="bold")
    ys = [46, 92, 138]
    for k, s in enumerate(["11 0 obj << /Type /Font ... >>",
                           "12 0 obj << /Type /FontDescriptor ... >>",
                           "13 0 obj << /Type /Font ... >>"]):
        f.box(10, ys[k], 370, 34, BLUE, s, 11, mono=True)
    f.box(10, 200, 370, 110, YELLOW)
    f.text(22, 222, "xref", 11, anchor="start", mono=True)
    for k, s in enumerate(["0000001434 00000 n", "0000001735 00000 n", "0000002155 00000 n"]):
        y = 248 + k * 24
        f.text(22, y, s, 11, anchor="start", mono=True)
        x = 395 + k * 9
        f.path(f"M190,{y - 5} L{x},{y - 5} L{x},{ys[k] + 17} L384,{ys[k] + 17}", stroke="#b8860b")
        f.head(384, ys[k] + 17, 3.14159, "#b8860b")
    f.lines(195, 336, ["every dictionary is plain text;", "20 bytes of text per xref entry"], 12, fill=GREY)
    # Right: object stream.
    x = 470
    f.text(x + 210, 26, "PDF 1.5: object stream, xref stream", 15, weight="bold")
    f.box(x, 46, 420, 140, GREEN)
    f.text(x + 10, 66, "15 0 obj << /Type /ObjStm /N 3 /First 18", 11, anchor="start", mono=True)
    f.text(x + 10, 86, "/Filter /FlateDecode >> stream", 11, anchor="start", mono=True)
    f.rect(x + 10, 96, 400, 82, fill="white", stroke="#82b366", dash="3 2")
    f.text(x + 18, 114, "11 0 12 547 13 665", 11, anchor="start", mono=True, fill="#2d6a2d")
    f.text(x + 404, 114, "number, offset", 10, anchor="end", fill=GREY)
    f.text(x + 18, 134, "<< /Type /Font ... >>", 11, anchor="start", mono=True)
    f.text(x + 18, 152, "<< /Type /FontDescriptor ... >>", 11, anchor="start", mono=True)
    f.text(x + 18, 170, "<< /Type /Font ... >>", 11, anchor="start", mono=True)
    f.box(x, 200, 420, 110, PURPLE)
    f.text(x + 10, 222, "99 0 obj << /Type /XRef /W [1 2 2] >>", 11, anchor="start", mono=True)
    for k in range(3):
        y = 248 + k * 24
        f.text(x + 18, y, f"02 000F 000{k}", 11, anchor="start", mono=True)
        f.text(x + 150, y, f"in stream 15, index {k}", 10.5, anchor="start", fill=GREY)
    f.path(f"M{x + 420},{240} L{x + 428},{240} L{x + 428},{60} L{x + 422},{60}", stroke="#9673a6")
    f.head(x + 421, 60, 3.14159, "#9673a6")
    f.lines(x + 210, 336, ["the dictionaries compressed together;", "binary xref entries, compressed too"], 12,
            fill=GREY)
    f.write()


main()
