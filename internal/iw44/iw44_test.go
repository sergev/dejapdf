package iw44

import (
	"os"
	"testing"

	"github.com/sergev/dejapdf/internal/iff"
)

func bg44(t testing.TB, file string) [][]byte {
	b, err := os.ReadFile("../../testdata/corpus/" + file)
	if err != nil {
		t.Fatal(err)
	}
	root, err := iff.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	var r [][]byte
	for _, c := range root.All("BG44") {
		r = append(r, c.Data)
	}
	return r
}

func TestHeader(t *testing.T) {
	cs := bg44(t, "photo.djvu")
	var d Decoder
	if _, err := d.Image(); err == nil {
		t.Error("image without data")
	}
	if err := d.Decode(cs[1]); err == nil {
		t.Error("chunk 2 accepted first")
	}
	for _, c := range cs {
		if err := d.Decode(c); err != nil {
			t.Fatal(err)
		}
	}
	im, _ := d.Image()
	if im.W != 850 || im.H != 1100 || im.Gray || len(im.Pix) != 3*850*1100 || d.delay != 0 || d.chromaHalf {
		t.Errorf("got %dx%d gray=%v delay=%d half=%v", im.W, im.H, im.Gray, d.delay, d.chromaHalf)
	}
	for _, b := range [][]byte{nil, {0}, {0, 1, 1, 2, 0, 0, 0, 1, 0}, {0, 1, 2, 2, 0, 1, 0, 1, 0}} {
		var d Decoder
		if d.Decode(b) == nil {
			t.Errorf("% x accepted", b)
		}
	}
}

// Truncated chunks decode (past the end the coder reads 0xff).
func TestTruncated(t *testing.T) {
	c := bg44(t, "gray.djvu")[0]
	for _, n := range []int{9, 10, 100, len(c) / 2} {
		var d Decoder
		if err := d.Decode(c[:n]); err != nil {
			t.Fatalf("%d bytes: %v", n, err)
		}
		if _, err := d.Image(); err != nil {
			t.Fatal(err)
		}
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(bg44(f, "gray.djvu")[0][:200])
	f.Add([]byte{0, 3, 0, 2, 0, 40, 0, 30, 0x81, 1, 2, 3})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 9 && int(b[4])<<8|int(b[5]) > 512 || len(b) > 9 && int(b[6])<<8|int(b[7]) > 512 {
			return // keep runs fast
		}
		var d Decoder
		if d.Decode(b) == nil {
			d.Image()
		}
	})
}
