// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package segment

import (
	"math/rand/v2"
	"testing"

	"github.com/sergev/dejapdf/internal/raster"
)

// page is gray paper, a gradient with grain, under blurred text: bars
// with a halo several pixels wide, as a JPEG scan has. It returns the
// paper without the text, too.
func page(w, h int) (*raster.Pixmap, []float64) {
	rng := rand.New(rand.NewPCG(1, 2))
	paper := make([]float64, w*h)
	ink := make([]float64, w*h)
	for y := range h {
		for x := range w {
			paper[y*w+x] = 200 + 40*float64(x)/float64(w)
			if y%40 >= 10 && y%40 < 30 && x%12 < 4 && x > 20 && x < w-20 {
				ink[y*w+x] = 1
			}
		}
	}
	// Blur the ink four times: a halo of about 4 pixels.
	for range 4 {
		next := make([]float64, w*h)
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				s := 0.0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						s += ink[(y+dy)*w+x+dx]
					}
				}
				next[y*w+x] = s / 9
			}
		}
		ink = next
	}
	p := &raster.Pixmap{W: w, H: h, N: 1, Pix: make([]byte, w*h)}
	for i := range p.Pix {
		v := paper[i]*(1-min(1, 1.5*ink[i])) + 2*rng.NormFloat64()
		p.Pix[i] = byte(min(max(v+0.5, 0), 255))
	}
	return p, paper
}

// TestBackground: the text leaves no ghost; the fill follows the paper.
func TestBackground(t *testing.T) {
	const w, h, k = 600, 400, 3
	p, paper := page(w, h)
	m := Mask(p, 300)
	bg := Background(p, m, k, 3, 15)
	if bg.W != (w+k-1)/k || bg.H != (h+k-1)/k || bg.N != 1 {
		t.Fatalf("background %dx%dx%d", bg.W, bg.H, bg.N)
	}
	// A ghost would make the text rows darker than the paper.
	worst, bias, n := 0.0, 0.0, 0
	for y := range h / k {
		for x := range w / k {
			d := float64(bg.Pix[y*bg.W+x]) - paper[(y*k+k/2)*w+x*k+k/2]
			worst = max(worst, abs(d))
			if r := y * k % 40; r >= 6 && r < 34 && x*k > 20 && x*k < w-20 {
				bias += d
				n++
			}
		}
	}
	bias /= float64(n)
	t.Logf("worst error %.1f, bias under the text %.2f", worst, bias)
	if worst > 6 || abs(bias) > 0.5 {
		t.Errorf("background off the paper by %.1f, under the text by %.2f", worst, bias)
	}
}

func TestUniform(t *testing.T) {
	p := &raster.Pixmap{W: 50, H: 40, N: 3, Pix: make([]byte, 3*50*40)}
	for i := range p.Pix {
		p.Pix[i] = byte(200 + i%3*10 + i/3%2)
	}
	c, ok := Uniform(p)
	if !ok || c != (Color{201, 211, 221}) {
		t.Errorf("uniform %v %v", c, ok)
	}
	p.Pix[3*1000] = 190
	if _, ok := Uniform(p); ok {
		t.Error("not uniform, but said so")
	}
}

// A page all mask, and tiny pages, fill without trouble.
func TestBackgroundEdge(t *testing.T) {
	for _, n := range []int{1, 3, 4} {
		for _, sz := range [][2]int{{1, 1}, {3, 5}, {17, 2}, {80, 80}} {
			p := &raster.Pixmap{W: sz[0], H: sz[1], N: n, Pix: make([]byte, n*sz[0]*sz[1])}
			m := &raster.Bitmap{W: sz[0], H: sz[1], Stride: (sz[0] + 7) / 8}
			m.Pix = make([]byte, m.Stride*m.H)
			for i := range m.Pix {
				m.Pix[i] = 0xff
			}
			bg := Background(p, m, 3, 1, 8)
			if c, ok := Uniform(bg); !ok || c != (Color{255, 255, 255}) {
				t.Errorf("%d %v: all mask fills %v %v", n, sz, c, ok)
			}
		}
	}
}
