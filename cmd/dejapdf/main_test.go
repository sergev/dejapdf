// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"testing"
)

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
