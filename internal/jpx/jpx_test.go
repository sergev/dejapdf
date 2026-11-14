// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jpx

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testImage is a smooth gradient with noise and a few edges.
func testImage(w, h, nc int, seed int64) [][]byte {
	r := rand.New(rand.NewSource(seed))
	planes := make([][]byte, nc)
	for c := range planes {
		p := make([]byte, w*h)
		for y := range h {
			for x := range w {
				v := 128 + 60*math.Sin(float64(x+c*7)/9)*math.Cos(float64(y)/13) + r.NormFloat64()*4
				if (x/16+y/16)%5 == 0 {
					v = 40
				}
				p[y*w+x] = byte(max(0, min(255, v)))
			}
		}
		planes[c] = p
	}
	return planes
}

// interleave returns the pixels as a PNM body.
func interleave(planes [][]byte) []byte {
	var b []byte
	for i := range planes[0] {
		for _, p := range planes {
			b = append(b, p[i])
		}
	}
	return b
}

// opjDecode decodes a JP2 file with opj_decompress to interleaved pixels.
func opjDecode(t *testing.T, jp2 []byte, nc int) []byte {
	t.Helper()
	dir := t.TempDir()
	in, out := filepath.Join(dir, "a.jp2"), filepath.Join(dir, "a.pgm")
	if nc == 3 {
		out = filepath.Join(dir, "a.ppm")
	}
	if err := os.WriteFile(in, jp2, 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("opj_decompress", "-i", in, "-o", out).CombinedOutput(); err != nil {
		t.Fatalf("opj_decompress: %v\n%s", err, b)
	}
	d, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// Skip the header: magic, optional comment, size, maxval.
	lines := 0
	for i := 0; i < len(d); i++ {
		if d[i] == '\n' {
			if d[i+1] != '#' {
				lines++
			}
			if lines == 3 {
				return d[i+1:]
			}
		}
	}
	t.Fatal("bad PNM")
	return nil
}

func psnr(a, b []byte) float64 {
	se := 0.0
	for i := range a {
		e := float64(a[i]) - float64(b[i])
		se += e * e
	}
	if se == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255*float64(len(a))/se)
}

func TestOpenJPEG(t *testing.T) {
	if _, err := exec.LookPath("opj_decompress"); err != nil {
		t.Skip("no opj_decompress")
	}
	for _, c := range []struct {
		w, h, nc, budget int
		min              float64 // dB
	}{
		{1, 1, 1, 0, 50},
		{3, 2, 3, 0, 50},
		{17, 9, 1, 0, 50},
		{130, 70, 3, 0, 50},
		{200, 150, 1, 0, 50},
		{200, 150, 3, 0, 50},
		{200, 150, 3, 12000, 35},
		{200, 150, 1, 3000, 30},
		{300, 257, 3, 2000, 20},
	} {
		name := fmt.Sprintf("%dx%dx%d/%d", c.w, c.h, c.nc, c.budget)
		planes := testImage(c.w, c.h, c.nc, int64(c.w))
		jp2, err := Encode(c.w, c.h, planes, c.budget)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.budget > 0 && len(jp2) > c.budget {
			t.Errorf("%s: %d bytes, over budget", name, len(jp2))
		}
		ref := interleave(planes)
		got := opjDecode(t, jp2, c.nc)
		if p := psnr(ref, got); p < c.min {
			t.Errorf("%s: %.1f dB, want ≥ %.0f", name, p, c.min)
		} else {
			t.Logf("%s: %d bytes, %.1f dB", name, len(jp2), p)
		}
	}
}

// TestBudget checks that a larger budget never gives a worse picture.
func TestBudget(t *testing.T) {
	if _, err := exec.LookPath("opj_decompress"); err != nil {
		t.Skip("no opj_decompress")
	}
	planes := testImage(256, 192, 3, 5)
	ref := interleave(planes)
	last := 0.0
	for _, b := range []int{1000, 2000, 4000, 8000, 16000, 32000} {
		jp2, err := Encode(256, 192, planes, b)
		if err != nil {
			t.Fatal(err)
		}
		p := psnr(ref, opjDecode(t, jp2, 3))
		if len(jp2) > b || p < last {
			t.Errorf("budget %d: %d bytes, %.2f dB after %.2f", b, len(jp2), p, last)
		}
		last = p
	}
}

func TestHull(t *testing.T) {
	cb := &block{passes: []pass{{10, 100}, {12, 101}, {20, 300}, {20, 310}, {40, 320}, {45, 320}}}
	hullOf(cb, 1)
	// (20,310) is above the chord from (0,0) through (10,100) and (12,101),
	// and beats (20,300); (45,320) gains nothing.
	want := []int{4, 5}
	if fmt.Sprint(cb.hull) != fmt.Sprint(want) {
		t.Fatalf("hull %v, want %v", cb.hull, want)
	}
	for i := 1; i < len(cb.slope); i++ {
		if cb.slope[i] >= cb.slope[i-1] {
			t.Fatalf("slopes %v not decreasing", cb.slope)
		}
	}
}

func TestBadInput(t *testing.T) {
	for _, c := range []struct {
		w, h   int
		planes [][]byte
	}{
		{0, 1, [][]byte{{}}},
		{2, 2, [][]byte{make([]byte, 4), make([]byte, 4)}},
		{2, 2, [][]byte{make([]byte, 3)}},
	} {
		if _, err := Encode(c.w, c.h, c.planes, 0); err == nil {
			t.Errorf("%dx%d %d planes: no error", c.w, c.h, len(c.planes))
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	planes := testImage(850, 1100, 3, 1)
	for b.Loop() {
		if _, err := Encode(850, 1100, planes, 16000); err != nil {
			b.Fatal(err)
		}
	}
}
