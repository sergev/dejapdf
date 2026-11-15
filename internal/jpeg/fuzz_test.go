// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jpeg

import (
	"bytes"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"testing"
)

// sample encodes a small image with the standard encoder.
func sample(img image.Image) []byte {
	var b bytes.Buffer
	stdjpeg.Encode(&b, img, &stdjpeg.Options{Quality: 75})
	return b.Bytes()
}

func testImages() []image.Image {
	g := image.NewGray(image.Rect(0, 0, 37, 21))
	c := image.NewRGBA(image.Rect(0, 0, 37, 21))
	for y := range 21 {
		for x := range 37 {
			g.SetGray(x, y, color.Gray{uint8(x*7 + y*3)})
			c.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 12), uint8(x * y), 255})
		}
	}
	return []image.Image{g, c}
}

// TestDecodePlanes: our planes agree with the standard decoder within
// the IDCTs' difference, and have the right sizes.
func TestDecodePlanes(t *testing.T) {
	for _, img := range testImages() {
		data := sample(img)
		p, err := DecodePlanes(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		std, _ := stdjpeg.Decode(bytes.NewReader(data))
		if p.W != 37 || p.H != 21 {
			t.Errorf("size %dx%d", p.W, p.H)
		}
		y := p.Comps[0]
		for yy := range p.H {
			for x := range p.W {
				var want uint8
				switch m := std.(type) {
				case *image.Gray:
					want = m.GrayAt(x, yy).Y
				case *image.YCbCr:
					want = m.YCbCrAt(x, yy).Y
				}
				if d := int(y.Pix[yy*y.W+x]) - int(want); d > 2 || d < -2 {
					t.Fatalf("(%d,%d): %d, standard %d", x, yy, y.Pix[yy*y.W+x], want)
				}
			}
		}
		if len(p.Comps) == 3 && (!p.YCbCr || p.Comps[1].Fx != 2 || p.Comps[1].W != 19 || p.Comps[1].H != 11) {
			t.Errorf("chroma %+v", p.Comps[1])
		}
	}
}

func FuzzDecodePlanes(f *testing.F) {
	for _, img := range testImages() {
		f.Add(sample(img))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if c, err := stdjpeg.DecodeConfig(bytes.NewReader(data)); err != nil || c.Width*c.Height > 1<<20 {
			return
		}
		DecodePlanes(bytes.NewReader(data))
	})
}
