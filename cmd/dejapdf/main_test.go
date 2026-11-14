// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"testing"
	"time"
)

func TestSummary(t *testing.T) {
	for _, c := range []struct {
		size, djvu int64
		d          time.Duration
		want       string
	}{
		{12641280, 10534400, 9100 * time.Millisecond, "book.pdf: 12345 kbytes, 1.20x in 9.1s"},
		{1000, 0, 40 * time.Millisecond, "book.pdf: 1 kbytes in 0.0s"},
	} {
		if got := summary("book.pdf", c.size, c.djvu, c.d); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestParsePages(t *testing.T) {
	for in, want := range map[string]string{
		"1":       "[0]",
		"2-4":     "[1 2 3]",
		"8-":      "[7 8 9]",
		"-2":      "[0 1]",
		"3,1-2,3": "[2 0 1]",
		"0":       "error",
		"11":      "error",
		"5-3":     "error",
		"x":       "error",
		"1-x":     "error",
		"":        "error",
		"1,,2":    "error",
		"10-10":   "[9]",
	} {
		r, err := parsePages(in, 10)
		got := fmt.Sprint(r)
		if err != nil {
			got = "error"
		}
		if got != want {
			t.Errorf("%q: got %s, want %s", in, got, want)
		}
	}
}
