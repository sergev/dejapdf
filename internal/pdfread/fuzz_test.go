// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"
	"os"
	"testing"
)

func FuzzOpen(f *testing.F) {
	maxDecoded = 1 << 22
	good := build(testObjs, "table", nil)
	f.Add(good)
	f.Add(good[:bytes.LastIndex(good, []byte("startxref"))])
	f.Add(build(testObjs, "hybrid", map[int]bool{2: true, 4: true, 8: true}))
	ostm := build(testObjs, "stream", map[int]bool{2: true, 4: true, 8: true})
	f.Add(ostm)
	f.Add(ostm[:bytes.LastIndex(ostm, []byte("startxref"))])
	for _, name := range []string{"inline", "encrypt-rc4-40", "encrypt-aes-256"} {
		if b, err := os.ReadFile("../../testdata/pdf/" + name + ".pdf"); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := Open(data)
		if err != nil {
			return
		}
		r.Pages()
		for _, n := range r.Objects() {
			if s, ok := r.Get(n).(*Stream); ok {
				r.Raw(s)
				r.Decode(s)
			}
		}
	})
}
