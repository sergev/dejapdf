// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/sergev/dejapdf/internal/convert"
)

func TestProgressLine(t *testing.T) {
	half := convert.Progress{Page: 3, Pages: 10, Done: 50, Total: 100}
	for _, c := range []struct {
		name  string
		p     convert.Progress
		width int
		want  string
	}{
		{"a.pdf", half, 50, "a.pdf: 50%, pages 3/10"},
		{"a.pdf", convert.Progress{Page: 1, Pages: 4, Done: 1, Total: 16}, 50, "a.pdf: 6%, pages 1/4"},
		{"a.pdf", convert.Progress{Page: 10, Pages: 10, Done: 100, Total: 100}, 50, "a.pdf: 100%, pages 10/10"},
		{"a.pdf", convert.Progress{Page: 1, Pages: 2}, 50, "a.pdf: 50%, pages 1/2"},
		{"a.pdf", convert.Progress{}, 50, "a.pdf: 0%"},
		{"long-name.pdf", half, 24, "…me.pdf: 50%, pages 3/10"},
		{"a.pdf", half, 10, "50%, pages 3/10"},
	} {
		got := progressLine(c.name, "pages", c.p, c.width)
		if got != c.want {
			t.Errorf("%q width %d:\n got %q\nwant %q", c.name, c.width, got, c.want)
		}
	}
}
