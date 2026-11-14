// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
	"unicode/utf8"

	"github.com/sergev/dejapdf/internal/convert"
)

func TestProgressLine(t *testing.T) {
	half := convert.Progress{Page: 3, Pages: 10, Done: 50, Total: 100, Written: 3 << 20}
	for _, c := range []struct {
		name  string
		p     convert.Progress
		width int
		dumb  bool
		want  string
	}{
		{"a.pdf", half, 50, false, "a.pdf  [████████▌        ]   50%   3/10     3.0 MB"},
		{"a.pdf", half, 50, true, "a.pdf  [########         ]   50%   3/10     3.0 MB"},
		{"a.pdf", convert.Progress{Page: 1, Pages: 4, Done: 1, Total: 16}, 50, false,
			"a.pdf  [█▏                 ]    6%  1/4     0.0 MB"},
		{"a.pdf", convert.Progress{Page: 10, Pages: 10, Done: 100, Total: 100}, 50, false,
			"a.pdf  [█████████████████]  100%  10/10     0.0 MB"},
		{"a.pdf", convert.Progress{Page: 1, Pages: 2}, 50, false,
			"a.pdf  [█████████▌         ]   50%  1/2     0.0 MB"},
		{"a.pdf", half, 79, false, "a.pdf  [████████████████████                    ]   50%   3/10     3.0 MB"},
		{"a.pdf", half, 30, false, "a.pdf   50%   3/10     3.0 MB"},
		{"long-name.pdf", half, 30, false, "…e.pdf   50%   3/10     3.0 MB"},
		{"a.pdf", half, 10, false, " 50%   3/10     3.0 MB"},
	} {
		got := progressLine(c.name, c.p, c.width, c.dumb)
		if got != c.want {
			t.Errorf("%q width %d:\n got %q\nwant %q", c.name, c.width, got, c.want)
		}
		if n := utf8.RuneCountInString(got); n > c.width && c.width >= 20 {
			t.Errorf("%q width %d: %d runes", c.name, c.width, n)
		}
	}
}

func TestBar(t *testing.T) {
	for _, c := range []struct {
		f    float64
		want string
	}{
		{0, "    "}, {0.25, "█   "}, {0.5 + 1.0/32, "██▏ "}, {1, "████"},
	} {
		if got := bar(c.f, 4, false); got != c.want {
			t.Errorf("%v: got %q, want %q", c.f, got, c.want)
		}
	}
}
