// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sergev/dejapdf/internal/djvu"
)

// Truncated and corrupted corpus files convert without panics, with
// warnings; truncated ones still give valid PDF.
func TestDamaged(t *testing.T) {
	t.Parallel()
	qpdf := true
	if _, err := exec.LookPath("qpdf"); err != nil {
		qpdf = false
	}
	rng := rand.New(rand.NewSource(1))
	for _, name := range []string{"bundled.djvu", "pdf2djvu.djvu", "shared-dict.djvu", "compound.djvu",
		"jpeg.djvu", "mmr.djvu", "palette.djvu", "photo.djvu"} {
		good, err := os.ReadFile(filepath.Join(corpus, name))
		if err != nil {
			t.Fatal(err)
		}
		var cases [][]byte
		var kinds []string
		for _, frac := range []float64{0.1, 0.3, 0.5, 0.7, 0.9, 0.99} {
			cases = append(cases, good[:int(float64(len(good))*frac)])
			kinds = append(kinds, fmt.Sprintf("cut %.0f%%", frac*100))
		}
		for k := range 20 {
			b := bytes.Clone(good)
			for range 1 + k%4 {
				b[16+rng.Intn(len(b)-16)] = byte(rng.Intn(256))
			}
			cases = append(cases, b)
			kinds = append(kinds, fmt.Sprintf("corrupt %d", k))
		}
		// Cases made in order: the same corruptions every run.
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			damaged(t, name, cases, kinds, qpdf)
		})
	}
}

// damaged converts the damaged copies of one file.
func damaged(t *testing.T, name string, cases [][]byte, kinds []string, qpdf bool) {
	dir := t.TempDir()
	converted := 0
	for c, b := range cases {
		doc, err := djvu.Parse(b, nil)
		if err != nil {
			continue
		}
		warns := 0
		opt := Options{Warn: func(int, error) { warns++ }}
		out := io.Discard
		path := filepath.Join(dir, "d.pdf")
		var f *os.File
		if qpdf && c < 6 {
			f, _ = os.Create(path)
			out = f
		}
		if err := Convert(doc, out, opt); err != nil {
			t.Errorf("%s %s: %v", name, kinds[c], err)
			continue
		}
		converted++
		if f != nil {
			f.Close()
			if b, err := exec.Command("qpdf", "--check", path).CombinedOutput(); err != nil {
				t.Errorf("%s %s: qpdf: %s", name, kinds[c], b)
			}
		}
	}
	t.Logf("%s: %d of %d damaged files converted", name, converted, len(cases))
}

// Whole small documents through the converter. Pages over 16 megapixels
// are skipped to keep runs fast; the decoders have their own targets.
func FuzzConvert(f *testing.F) {
	for _, n := range []string{"bundled.djvu", "palette.djvu", "mmr.djvu", "jpeg.djvu", "compound.djvu"} {
		b, err := os.ReadFile(filepath.Join(corpus, n))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		doc, err := djvu.Parse(b, nil)
		if err != nil {
			return
		}
		for _, pg := range doc.Pages {
			if pg.Info.Width*pg.Info.Height > 1<<24 {
				return
			}
		}
		Convert(doc, io.Discard, Options{Warn: func(int, error) {}})
	})
}
