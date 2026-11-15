// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package stencil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/pdf"
)

// TestJbig2dec codes the corpus masks with and without globals, whole
// and cropped to some blits, and checks jbig2dec's decode.
func TestJbig2dec(t *testing.T) {
	if _, err := exec.LookPath("jbig2dec"); err != nil {
		t.Skip("no jbig2dec")
	}
	n := 0
	for _, name := range []string{"shared-dict", "bilevel", "compound", "mmr"} {
		doc, err := djvu.Open(filepath.Join("../../testdata/corpus", name+".djvu"))
		if err != nil {
			t.Fatal(err)
		}
		c := &Cache{}
		for i, pg := range doc.Pages {
			mask, err := doc.Mask(pg)
			if err != nil || mask == nil {
				continue
			}
			odd := func(k int) bool { return k%2 == 1 }
			for _, cache := range []*Cache{c, nil} {
				for _, keep := range []func(int) bool{nil, odd} {
					d, data := Code(mask, 0, 0, mask.W, mask.H, keep, cache)
					if d == nil {
						continue
					}
					var glob []byte
					if p, ok := d["DecodeParms"].(pdf.Dict); ok {
						glob = p["JBIG2Globals"].(*Globals).Data()
					}
					n++
					want := mask.RenderSome(mask.W, mask.H, keep)
					if got := decode(t, glob, data); !bytes.Equal(got, pbm(want)) {
						t.Errorf("%s page %d (globals %v, odd %v): jbig2dec output differs",
							name, i+1, glob != nil, keep != nil)
					}
				}
			}
		}
	}
	if n < 8 {
		t.Errorf("%d decodes", n)
	}
	t.Logf("%d decodes", n)
}

// decode runs jbig2dec on embedded streams, returning PBM.
func decode(t *testing.T, glob, page []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	out, in := filepath.Join(dir, "a.pbm"), filepath.Join(dir, "p")
	args := []string{"-q", "-e", "-t", "pbm", "-o", out}
	if glob != nil {
		g := filepath.Join(dir, "g")
		os.WriteFile(g, glob, 0o644)
		args = append(args, g)
	}
	os.WriteFile(in, page, 0o644)
	if b, err := exec.Command("jbig2dec", append(args, in)...).CombinedOutput(); err != nil {
		t.Fatalf("jbig2dec: %v\n%s", err, b)
	}
	d, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// pbm packs a byte-per-pixel bitmap as PBM.
func pbm(b *jb2.Bitmap) []byte {
	m := pack(b)
	return append(fmt.Appendf(nil, "P4\n%d %d\n", m.W, m.H), m.Pix...)
}
