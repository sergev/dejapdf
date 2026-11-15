// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"
	"compress/lzw"
	"encoding/ascii85"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"testing"

	"github.com/sergev/dejapdf/internal/pdf"
)

func identity(v any) any { return v }

// samples returns test inputs: empty, short, runs, random, long text.
func samples() [][]byte {
	rnd := rand.New(rand.NewPCG(1, 2))
	random := make([]byte, 20000)
	for i := range random {
		random[i] = byte(rnd.IntN(256))
	}
	runs := bytes.Repeat([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab"), 50)
	text := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 2000)
	return [][]byte{{}, {0}, []byte("x"), []byte("ab"), []byte("abcd\x00\x00\x00\x00"), runs, random, text}
}

// TestLZWSpec decodes the example of 7.4.4.2.
func TestLZWSpec(t *testing.T) {
	in, _ := hex.DecodeString("800B6050220C0C8501")
	got, err := lzwDecode(in, true)
	if err != nil || string(got) != "-----A---B" {
		t.Errorf("got %q, %v", got, err)
	}
}

// TestLZW decodes Go's LZW, which changes width late (EarlyChange 0).
func TestLZW(t *testing.T) {
	for _, in := range samples() {
		var b bytes.Buffer
		w := lzw.NewWriter(&b, lzw.MSB, 8)
		w.Write(in)
		w.Close()
		got, err := lzwDecode(b.Bytes(), false)
		if err != nil || !bytes.Equal(got, in) {
			t.Errorf("%d bytes: got %d, %v", len(in), len(got), err)
		}
	}
}

func TestASCII85(t *testing.T) {
	for _, in := range samples() {
		enc := make([]byte, ascii85.MaxEncodedLen(len(in)))
		enc = enc[:ascii85.Encode(enc, in)]
		// Line breaks, as writers put them.
		var b []byte
		for i := 0; i < len(enc); i += 75 {
			b = append(b, enc[i:min(i+75, len(enc))]...)
			b = append(b, "\r\n"...)
		}
		got, err := ascii85Decode(append(b, "~>"...))
		if err != nil || !bytes.Equal(got, in) {
			t.Errorf("%d bytes: got %d, %v", len(in), len(got), err)
		}
	}
	for in, want := range map[string]string{
		"<~87cURD]i,\"Ebo80~>":      "Hello World!",
		"z~>":                       "\x00\x00\x00\x00",
		"zz!!~>":                    "\x00\x00\x00\x00\x00\x00\x00\x00\x00",
		"9jqo^":                     "Man ",
		"9jqo^BlbD-BleB1DJ+*+F(f,q": "Man is distinguished",
	} {
		if got, err := ascii85Decode([]byte(in)); err != nil || string(got) != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"!~>", "s8W-\"~>", "abc{~>", "uuuuu~>"} {
		if _, err := ascii85Decode([]byte(in)); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
}

func TestASCIIHex(t *testing.T) {
	for _, in := range samples() {
		got, err := asciiHexDecode([]byte(hex.EncodeToString(in) + ">"))
		if err != nil || !bytes.Equal(got, in) {
			t.Errorf("%d bytes: got %d, %v", len(in), len(got), err)
		}
	}
	if got, err := asciiHexDecode([]byte("4 1\n4a 7>junk")); err != nil || string(got) != "AJp" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := asciiHexDecode([]byte("4g>")); err == nil {
		t.Error("bad digit: no error")
	}
}

// runLengthEncode is a plain RunLength encoder for tests.
func runLengthEncode(in []byte) []byte {
	var out []byte
	for i := 0; i < len(in); {
		n := 1
		for i+n < len(in) && in[i+n] == in[i] && n < 128 {
			n++
		}
		if n > 1 {
			out = append(out, byte(257-n), in[i])
			i += n
			continue
		}
		j := i
		for j < len(in) && j-i < 128 && (j+1 >= len(in) || in[j+1] != in[j]) {
			j++
		}
		if j == i {
			j++
		}
		out = append(out, byte(j-i-1))
		out = append(out, in[i:j]...)
		i = j
	}
	return append(out, 128)
}

func TestRunLength(t *testing.T) {
	for _, in := range samples() {
		got, err := runLengthDecode(runLengthEncode(in))
		if err != nil || !bytes.Equal(got, in) {
			t.Errorf("%d bytes: got %d, %v", len(in), len(got), err)
		}
	}
	// Truncated data decodes as far as it goes.
	if got, _ := runLengthDecode([]byte{3, 'a', 'b'}); string(got) != "ab" {
		t.Errorf("truncated: %q", got)
	}
}

// tiffEncode applies TIFF predictor 2.
func tiffEncode(in []byte, colors, bpc, cols int) []byte {
	row := (colors*bpc*cols + 7) / 8
	out := bytes.Clone(in)
	for r := 0; r+row <= len(out); r += row {
		b := out[r : r+row]
		orig := in[r : r+row]
		get := func(b []byte, s int) int {
			switch bpc {
			case 8:
				return int(b[s])
			case 16:
				return int(binary.BigEndian.Uint16(b[2*s:]))
			}
			return int(b[s*bpc/8]) >> (8 - bpc - s*bpc%8) & (1<<bpc - 1)
		}
		for s := colors; s < cols*colors; s++ {
			v := (get(orig, s) - get(orig, s-colors)) & (1<<bpc - 1)
			switch bpc {
			case 8:
				b[s] = byte(v)
			case 16:
				binary.BigEndian.PutUint16(b[2*s:], uint16(v))
			default:
				sh := 8 - bpc - s*bpc%8
				b[s*bpc/8] = b[s*bpc/8]&^byte((1<<bpc-1)<<sh) | byte(v<<sh)
			}
		}
	}
	return out
}

func TestTIFFPredictor(t *testing.T) {
	rnd := rand.New(rand.NewPCG(3, 4))
	for _, bpc := range []int{1, 2, 4, 8, 16} {
		for _, colors := range []int{1, 3, 4} {
			for _, cols := range []int{1, 7, 64} {
				row := (colors*bpc*cols + 7) / 8
				in := make([]byte, row*5)
				for i := range in {
					in[i] = byte(rnd.IntN(256))
				}
				// Padding bits at the row end are not predicted: zero them.
				if pad := row*8 - colors*bpc*cols; pad > 0 {
					for r := row - 1; r < len(in); r += row {
						in[r] &^= byte(1<<pad - 1)
					}
				}
				p := pdf.Dict{"Predictor": 2, "Colors": colors, "BitsPerComponent": bpc, "Columns": cols}
				got, err := predict(tiffEncode(in, colors, bpc, cols), p, identity)
				if err != nil || !bytes.Equal(got, in) {
					t.Errorf("bpc %d colors %d cols %d: %v", bpc, colors, cols, err)
				}
			}
		}
	}
}

// idat returns the zlib data of a PNG.
func idat(t *testing.T, img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	d := b.Bytes()[8:]
	var z []byte
	for len(d) >= 12 {
		n := int(binary.BigEndian.Uint32(d))
		if string(d[4:8]) == "IDAT" {
			z = append(z, d[8:8+n]...)
		}
		d = d[12+n:]
	}
	return z
}

// TestPNGPredictor decodes the image data of PNGs from image/png, which
// picks a filter per row.
func TestPNGPredictor(t *testing.T) {
	const w, h = 97, 40
	rnd := rand.New(rand.NewPCG(5, 6))
	rgb := image.NewRGBA(image.Rect(0, 0, w, h))
	gray16 := image.NewGray16(image.Rect(0, 0, w, h))
	pal := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black, color.White, color.Gray{128}})
	var wantRGB, want16, wantPal []byte
	for y := range h {
		var bits, nbits int
		for x := range w {
			// Smooth areas, edges and noise, so that every filter type wins somewhere.
			v := uint8(x*2 + y)
			if y%7 == 0 {
				v = uint8(rnd.IntN(256))
			}
			c := color.RGBA{v, uint8(x * y), uint8(255 - x), 255}
			rgb.Set(x, y, c)
			wantRGB = append(wantRGB, c.R, c.G, c.B)
			g := uint16(x*700 + y*3)
			gray16.SetGray16(x, y, color.Gray16{g})
			want16 = append(want16, byte(g>>8), byte(g))
			i := uint8((x / 3 * y) % 3)
			pal.SetColorIndex(x, y, i)
			bits, nbits = bits<<2|int(i), nbits+2
			if nbits == 8 {
				wantPal = append(wantPal, byte(bits))
				bits, nbits = 0, 0
			}
		}
		if nbits > 0 {
			wantPal = append(wantPal, byte(bits<<(8-nbits)))
		}
	}
	for _, tt := range []struct {
		name        string
		img         image.Image
		colors, bpc int
		want        []byte
	}{
		{"rgb", rgb, 3, 8, wantRGB},
		{"gray16", gray16, 1, 16, want16},
		{"palette 2-bit", pal, 1, 2, wantPal},
	} {
		p := pdf.Dict{"Predictor": 15, "Colors": tt.colors, "BitsPerComponent": tt.bpc, "Columns": w}
		got, err := decodeFilters(idat(t, tt.img), pdf.Name("FlateDecode"), p, identity)
		if err != nil || !bytes.Equal(got, tt.want) {
			t.Errorf("%s: %d bytes, %v; want %d", tt.name, len(got), err, len(tt.want))
		}
	}
}

func TestChain(t *testing.T) {
	in := []byte("chained filters")
	var b bytes.Buffer
	w := lzw.NewWriter(&b, lzw.MSB, 8)
	w.Write(in)
	w.Close()
	data := []byte(hex.EncodeToString(runLengthEncode(b.Bytes())) + ">")
	got, err := decodeFilters(data,
		pdf.Array{pdf.Name("AHx"), pdf.Name("RunLengthDecode"), pdf.Name("LZWDecode")},
		pdf.Array{nil, nil, pdf.Dict{"EarlyChange": 0}}, identity)
	if err != nil || !bytes.Equal(got, in) {
		t.Errorf("got %q, %v", got, err)
	}
	// An image filter stops decoding and is reported.
	got, err = decodeFilters(data, pdf.Array{pdf.Name("ASCIIHexDecode"), pdf.Name("DCTDecode")}, nil, identity)
	u, ok := err.(*UnsupportedFilterError)
	if !ok || u.Filter != "DCTDecode" || u.Index != 1 || !bytes.Equal(got, runLengthEncode(b.Bytes())) {
		t.Errorf("image filter: %v", err)
	}
}

func FuzzLZW(f *testing.F) {
	maxDecoded = 1 << 22
	for _, s := range samples()[:6] {
		f.Add(s, false)
	}
	in, _ := hex.DecodeString("800B6050220C0C8501")
	f.Add(in, true)
	f.Fuzz(func(t *testing.T, data []byte, early bool) {
		lzwDecode(data, early)
		var b bytes.Buffer
		w := lzw.NewWriter(&b, lzw.MSB, 8)
		w.Write(data)
		w.Close()
		if got, err := lzwDecode(b.Bytes(), false); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("round trip: %v", err)
		}
	})
}

func FuzzASCII85(f *testing.F) {
	f.Add([]byte("<~87cURD]i,\"Ebo80~>"))
	f.Add([]byte("zz!!~>"))
	f.Fuzz(func(t *testing.T, data []byte) {
		ascii85Decode(data)
		enc := make([]byte, ascii85.MaxEncodedLen(len(data)))
		enc = enc[:ascii85.Encode(enc, data)]
		if got, err := ascii85Decode(append(enc, "~>"...)); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("round trip: %v", err)
		}
	})
}

func FuzzASCIIHex(f *testing.F) {
	f.Add([]byte("4 1\n4a 7>"))
	f.Fuzz(func(t *testing.T, data []byte) {
		asciiHexDecode(data)
		if got, err := asciiHexDecode([]byte(hex.EncodeToString(data))); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("round trip: %v", err)
		}
	})
}

func FuzzRunLength(f *testing.F) {
	f.Add([]byte{3, 'a', 'b', 'c', 'd', 0xfe, 'x', 128})
	f.Fuzz(func(t *testing.T, data []byte) {
		runLengthDecode(data)
		if got, err := runLengthDecode(runLengthEncode(data)); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("round trip: %v", err)
		}
	})
}

func FuzzPredictor(f *testing.F) {
	maxDecoded = 1 << 22
	f.Add([]byte{2, 1, 2, 3, 0, 4, 5, 6}, 12, 1, 8, 3)
	f.Add([]byte{1, 2, 3, 4, 5, 6}, 2, 3, 4, 2)
	f.Fuzz(func(t *testing.T, data []byte, pred, colors, bpc, cols int) {
		p := pdf.Dict{"Predictor": pred, "Colors": colors, "BitsPerComponent": bpc, "Columns": cols}
		predict(data, p, identity)
		inflate(data)
	})
}
