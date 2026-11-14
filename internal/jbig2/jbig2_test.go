// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testBitmap draws random boxes, lines and noise.
func testBitmap(w, h int, seed int64) *Bitmap {
	r := rand.New(rand.NewSource(seed))
	b := NewBitmap(w, h)
	for range 20 {
		x0, y0 := r.Intn(w), r.Intn(h)
		x1, y1 := min(w, x0+r.Intn(w/3+1)+1), min(h, y0+r.Intn(h/3+1)+1)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if (x+y)%7 != 0 {
					b.Set(x, y)
				}
			}
		}
	}
	for range w * h / 50 {
		b.Set(r.Intn(w), r.Intn(h))
	}
	return b
}

// pbm returns b as a P4 file.
func pbm(b *Bitmap) []byte {
	out := fmt.Appendf(nil, "P4\n%d %d\n", b.W, b.H)
	return append(out, b.Pix...)
}

// jbig2dec decodes a standalone file to PBM pixels.
func jbig2dec(t *testing.T, file []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	in, out := filepath.Join(dir, "a.jb2"), filepath.Join(dir, "a.pbm")
	if err := os.WriteFile(in, file, 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("jbig2dec", "-q", "-t", "pbm", "-o", out, in).CombinedOutput(); err != nil {
		t.Fatalf("jbig2dec: %v\n%s", err, b)
	}
	d, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestGenericJbig2dec(t *testing.T) {
	if _, err := exec.LookPath("jbig2dec"); err != nil {
		t.Skip("no jbig2dec")
	}
	for _, sz := range [][2]int{{1, 1}, {7, 3}, {8, 8}, {9, 17}, {100, 1}, {333, 211}, {2550, 300}} {
		b := testBitmap(sz[0], sz[1], int64(sz[0]))
		segs := []Segment{
			{Number: 0, Type: TypePageInfo, Page: 1, Data: PageInfo(b.W, b.H, 300)},
			{Number: 1, Type: TypeGenericRegion, Page: 1, Data: GenericRegion(b, 0, 0)},
		}
		if got, want := jbig2dec(t, File(segs)), pbm(b); !bytes.Equal(got, want) {
			t.Errorf("%dx%d: jbig2dec output differs", b.W, b.H)
		}
	}
	// A blank bitmap is all typical rows.
	b := NewBitmap(64, 64)
	segs := []Segment{
		{Number: 0, Type: TypePageInfo, Page: 1, Data: PageInfo(64, 64, 0)},
		{Number: 1, Type: TypeGenericRegion, Page: 1, Data: GenericRegion(b, 0, 0)},
	}
	if got := jbig2dec(t, File(segs)); !bytes.Equal(got, pbm(b)) {
		t.Error("blank: jbig2dec output differs")
	}
}

func TestSegmentHeader(t *testing.T) {
	for _, c := range []struct {
		s    Segment
		want string
	}{
		{Segment{Number: 1, Type: 38, Page: 1, Data: []byte{9}}, "00000001 26 00 01 00000001 09"},
		{Segment{Number: 5, Type: 6, Refs: []uint32{1, 3}, Page: 1}, "00000005 06 40 0103 01 00000000"},
		{Segment{Number: 300, Type: 6, Refs: []uint32{2}, Page: 0}, "0000012c 06 20 0002 00 00000000"},
		{Segment{Number: 9, Type: 6, Refs: []uint32{1, 2, 3, 4, 5}, Page: 300}, "00000009 46 e0000005 00 0102030405 0000012c 00000000"},
	} {
		got := fmt.Sprintf("% x", c.s.Append(nil))
		want := ""
		for _, f := range bytes.Fields([]byte(c.want)) {
			for i := 0; i < len(f); i += 2 {
				if want != "" {
					want += " "
				}
				want += string(f[i : i+2])
			}
		}
		if got != want {
			t.Errorf("segment %d: %s, want %s", c.s.Number, got, want)
		}
	}
}
