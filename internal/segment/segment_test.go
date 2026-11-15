// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package segment

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/shapes"
)

// TestDdjvu segments corpus pages rendered in colour by ddjvu, and
// compares the mask with ddjvu -mode=mask: pixels differing, against
// the pixels of the mask, under maxXOR. Pages with dark print only:
// compound, jpeg and mmr-compound have light text on colour, and page 2
// of pdf2djvu is all mask.
func TestDdjvu(t *testing.T) {
	if _, err := exec.LookPath("ddjvu"); err != nil {
		t.Skip("no ddjvu")
	}
	const maxXOR = 0.01
	dir := t.TempDir()
	for _, c := range []struct {
		name  string
		pages int
	}{{"palette", 1}, {"pdf2djvu", 4}} {
		path := filepath.Join("../../testdata/corpus", c.name+".djvu")
		doc, err := djvu.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		for p := 1; p <= c.pages; p++ {
			if c.name == "pdf2djvu" && p == 2 {
				continue
			}
			n := fmt.Sprint(p)
			ppm, pbm := filepath.Join(dir, "c.ppm"), filepath.Join(dir, "m.pbm")
			for _, args := range [][]string{
				{"-format=ppm", "-page=" + n, path, ppm},
				{"-format=pbm", "-mode=mask", "-page=" + n, path, pbm},
			} {
				if out, err := exec.Command("ddjvu", args...).CombinedOutput(); err != nil {
					t.Fatalf("ddjvu %v: %v\n%s", args, err, out)
				}
			}
			px := readPPM(t, ppm)
			want := readPBM(t, pbm)
			got := Mask(px, float64(doc.Pages[p-1].Info.DPI))
			diff, ink := 0, 0
			for y := range got.H {
				for x := range got.W {
					a := got.Pix[y*got.Stride+x/8]>>(7-x%8)&1 != 0
					b := want.Pix[y*want.Stride+x/8]>>(7-x%8)&1 != 0
					if a != b {
						diff++
					}
					if b {
						ink++
					}
				}
			}
			r := float64(diff) / float64(max(ink, 1))
			t.Logf("%s page %d: %d pixels differ, %.1f%% of %d", c.name, p, diff, 100*r, ink)
			if r > maxXOR {
				t.Errorf("%s page %d: mask differs by %.1f%%", c.name, p, 100*r)
			}
		}
	}
}

func readHeader(t *testing.T, r *bufio.Reader, magic string) (int, int) {
	t.Helper()
	var m string
	var w, h int
	if _, err := fmt.Fscan(r, &m, &w, &h); err != nil || m != magic {
		t.Fatalf("bad %s header: %v", magic, err)
	}
	if magic == "P6" {
		var maxv int
		fmt.Fscan(r, &maxv)
	}
	r.ReadByte()
	return w, h
}

func readPPM(t *testing.T, path string) *raster.Pixmap {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := bufio.NewReader(f)
	w, h := readHeader(t, r, "P6")
	p := &raster.Pixmap{W: w, H: h, N: 3, Pix: make([]byte, 3*w*h)}
	if _, err := io.ReadFull(r, p.Pix); err != nil {
		t.Fatal(err)
	}
	return p
}

func readPBM(t *testing.T, path string) *raster.Bitmap {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := bufio.NewReader(f)
	w, h := readHeader(t, r, "P4")
	b := &raster.Bitmap{W: w, H: h, Stride: (w + 7) / 8}
	b.Pix = make([]byte, b.Stride*h)
	if _, err := io.ReadFull(r, b.Pix); err != nil {
		t.Fatal(err)
	}
	return b
}

// TestColors draws marks in black, near black, red and blue on paper
// and checks their colours: near black is black, the others in the
// palette.
func TestColors(t *testing.T) {
	const w, h = 400, 100
	p := &raster.Pixmap{W: w, H: h, N: 3, Pix: make([]byte, 3*w*h)}
	for i := range p.Pix {
		p.Pix[i] = 240
	}
	colours := []Color{{0, 0, 0}, {40, 40, 50}, {220, 30, 30}, {30, 40, 200}}
	for k, c := range colours {
		for y := 30; y < 70; y++ {
			for x := 20 + 100*k; x < 70+100*k; x++ {
				if (x/5+y/5)%2 == 0 { // a pattern of dots, not one blob
					copy(p.Pix[3*(y*w+x):], c[:])
				}
			}
		}
	}
	m := Mask(p, 300)
	j := shapes.Split(m)
	fg := Colors(p, j)
	got := map[int]Color{}
	for k, bl := range j.Blits {
		got[bl.X/100] = fg.Palette[fg.Index[k]]
	}
	want := []Color{{0, 0, 0}, {0, 0, 0}, {220, 30, 30}, {30, 40, 200}}
	for k := range want {
		if got[k] != want[k] {
			t.Errorf("mark %d: colour %v, want %v", k, got[k], want[k])
		}
	}
}

// Images smaller than a tile or the window do not break anything.
func TestTiny(t *testing.T) {
	for _, n := range []int{1, 3, 4} {
		for _, sz := range [][2]int{{1, 1}, {3, 5}, {17, 2}, {80, 80}} {
			p := &raster.Pixmap{W: sz[0], H: sz[1], N: n, Pix: make([]byte, n*sz[0]*sz[1])}
			for i := range p.Pix {
				p.Pix[i] = byte(i * 37)
			}
			m := Mask(p, 300)
			Colors(p, shapes.Split(m))
		}
	}
}
