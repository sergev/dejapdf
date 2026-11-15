// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/sergev/dejapdf/internal/progress"
)

func TestProgressLine(t *testing.T) {
	half := progress.Status{Phase: "pages", Unit: "pages", Count: 3, Of: 10, Frac: 0.5}
	for _, c := range []struct {
		name  string
		st    progress.Status
		width int
		want  string
	}{
		{"a.pdf", half, 50, "a.pdf: 50%, pages 3/10"},
		{"a.pdf", progress.Status{Phase: "images", Unit: "images", Count: 1, Of: 4, Frac: 0.066}, 50,
			"a.pdf: 6%, images 1/4"},
		{"a.pdf", progress.Status{Phase: "pages", Unit: "pages", Count: 10, Of: 10, Frac: 1}, 50,
			"a.pdf: 100%, pages 10/10"},
		{"a.pdf", progress.Status{Phase: "reading", Frac: 0.02}, 50, "a.pdf: 2%, reading"},
		{"a.pdf", progress.Status{Phase: "writing", Frac: 0.975}, 50, "a.pdf: 97%, writing"},
		{"a.pdf", progress.Status{}, 50, "a.pdf: 0%"},
		{"long-name.pdf", half, 24, "…me.pdf: 50%, pages 3/10"},
		{"a.pdf", half, 10, "50%, pages 3/10"},
	} {
		got := progressLine(c.name, c.st, c.width)
		if got != c.want {
			t.Errorf("%q width %d:\n got %q\nwant %q", c.name, c.width, got, c.want)
		}
	}
}
