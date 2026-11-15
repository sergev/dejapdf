// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package rewrite

import (
	"bytes"
	"os"
	"testing"

	"github.com/sergev/dejapdf/internal/pdfread"
)

// FuzzCopy: whatever opens copies into a file that opens without
// repair and has the same number of pages.
func FuzzCopy(f *testing.F) {
	f.Add([]byte("%PDF-1.7\n1 0 obj <</Type/Catalog/Pages<</Kids[2 0 R]>>>> endobj\n" +
		"2 0 obj <</Type/Page/X[1.5 -0.25 3 0 R]>> endobj\n3 0 obj <</Length 3>>stream\nq Q\nendstream endobj\n"))
	// Small seeds only: the fuzzer minimizes slowly on large inputs.
	for _, name := range []string{"inline"} {
		if b, err := os.ReadFile("../../testdata/pdf/" + name + ".pdf"); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := pdfread.Open(data)
		if err != nil {
			return
		}
		p1, _ := r.Pages()
		var b bytes.Buffer
		if err := Copy(r, &b); err != nil {
			t.Fatal(err)
		}
		c, err := pdfread.Open(b.Bytes())
		if err != nil || c.Repaired {
			t.Fatalf("copy: %v, repaired %v", err, c != nil && c.Repaired)
		}
		if p2, _ := c.Pages(); len(p2) != len(p1) {
			t.Fatalf("%d pages, want %d", len(p2), len(p1))
		}
	})
}
