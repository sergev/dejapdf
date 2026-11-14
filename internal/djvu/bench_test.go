// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"os"
	"testing"
)

func BenchmarkBookMasks(b *testing.B) {
	d, err := Open(os.Getenv("DJVU_BOOK"))
	if err != nil {
		b.Skip(err)
	}
	for range b.N {
		for _, p := range d.Pages[:60] {
			d.Mask(p)
		}
	}
}
