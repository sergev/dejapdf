"""A JP2 file: its boxes, and the markers of the codestream inside (bytes from an opj-made file)."""

from svg import Figure, LARGE, BLUE, GREEN, YELLOW, ORANGE, PURPLE, GRAY, GREY, INK


def box(f, x, y, w, h, colour, title, sub):
    f.box(x, y, w, h, colour)
    f.text(x + 12, y + 22, title, 11, anchor="start", weight="bold")
    f.text(x + 12, y + 42, sub, 9.5, anchor="start", fill=GREY, mono=True)


def main():
    f = Figure(940, 480, LARGE)
    f.text(20, 26, "JP2 file (Annex I): every box is length (4) + type (4) + contents", 12, weight="bold",
           anchor="start")
    box(f, 20, 40, 420, 56, GRAY, "jP␣␣  signature, 12 bytes", "00 00 00 0C 6A 50 20 20 0D 0A 87 0A")
    box(f, 20, 104, 420, 56, GRAY, "ftyp  file type, 20 bytes", "brand 'jp2 ', minor 0, compat 'jp2 '")
    f.box(20, 168, 420, 180, BLUE)
    f.text(32, 190, "jp2h  header superbox, 45 bytes", 11, anchor="start", weight="bold")
    box(f, 36, 202, 388, 70, YELLOW, "ihdr  image header, 22 bytes", "H 32, W 64; NC 3; BPC 7; C 7;")
    f.text(48, 264, "UnkC 0; IPR 0  (BPC = depth − 1)", 9.5, anchor="start", fill=GREY, mono=True)
    box(f, 36, 280, 388, 56, YELLOW, "colr  colour, 15 bytes", "METH 1, PREC 0, APPROX 0, EnumCS 16")
    box(f, 20, 356, 420, 56, ORANGE, "jp2c  contiguous codestream", "length = codestream + 8")
    f.text(230, 440, "85 bytes before the codestream starts", 10.5, fill=INK)
    f.text(230, 462, "EnumCS 16 = sRGB, 17 = greyscale; C 7 = JPEG 2000", 10.5, fill=INK)
    # Codestream.
    x = 480
    f.text(x, 66, "codestream (Annex A)", 12, weight="bold", anchor="start")
    marks = [
        (PURPLE, "FF4F  SOC", "start of codestream"),
        (PURPLE, "FF51  SIZ", "size, tile = image, components, depth"),
        (PURPLE, "FF52  COD", "layers, MCT, levels, block size, 9/7"),
        (PURPLE, "FF5C  QCD", "guard bits, a step per sub-band"),
        (GREEN, "FF90  SOT", "tile 0, its length"),
        (GREEN, "FF93  SOD", "then the packets"),
        (GREEN, "packets", "header + code-block bytes"),
        (PURPLE, "FFD9  EOC", "end of codestream"),
    ]
    for k, (c, name, note) in enumerate(marks):
        y = 80 + k * 44
        f.box(x, y, 140, 38, c, name, 10.5, mono=True)
        f.text(x + 152, y + 25, note, 10, anchor="start")
    f.text(x + 220, 448, "main header: SIZ, COD, QCD; tile-part header: SOT … SOD", 10, fill=GREY)
    f.write()


main()
