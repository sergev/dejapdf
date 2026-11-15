// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"fmt"
	"strings"
	"testing"
)

// ops renders the operators of a content stream as text.
func ops(data string) string {
	var out []string
	Content([]byte(data), func(op string, args []any) bool {
		if op == "BI" {
			im := args[0].(*InlineImage)
			out = append(out, fmt.Sprintf("BI %v[%s]", im.Dict, im.Data))
			return true
		}
		out = append(out, fmt.Sprintf("%v%s", args, op))
		return true
	})
	return strings.Join(out, " ")
}

func TestContent(t *testing.T) {
	tests := []struct{ in, want string }{
		{"q 1 0 0 1 10 20 cm /Im0 Do Q", "[]q [1 0 0 1 10 20]cm [Im0]Do []Q"},
		{"BT /F1 12 Tf (a) Tj [(b) 3 (c)] TJ ET", "[]BT [F1 12]Tf [a]Tj [[b 3 c]]TJ []ET"},
		// Unfiltered: the length decides, even with EI inside the data.
		{"BI /W 2 /H 1 /CS /G /BPC 8 ID EI\nEI Q", "BI map[BPC:8 CS:G H:1 W:2][EI] []Q"},
		// Filtered: the first EI between white space.
		{"BI /W 9 /H 9 /F /AHx ID 4142EIx 4344 EI q", "BI map[F:AHx H:9 W:9][4142EIx 4344] []q"},
		{"BI /W 1 /H 1 /F /AHx ID 41> EI", "BI map[F:AHx H:1 W:1][41>]"},
		// Junk is skipped.
		{") ] 1 2 m >> 3 l", "[1 2]m [3]l"},
		{"{ 1 } d0", "[]{ [1]} []d0"},
		// A sign inside a number ends it, as in Reader, MuPDF and
		// Ghostscript (Poppler skips it: 0.55); McKusick's FineReader
		// writes such Tz operands. CR separates.
		{"q\r0.5-5 0 0 1 0 0 cm\r/im2 Do\rQ\r99.0-9 Tz", "[]q [0.5 0 0 1 0 0]cm [im2]Do []Q [99]Tz"},
	}
	for _, tt := range tests {
		if got := ops(tt.in); got != tt.want {
			t.Errorf("%q:\n got %s\nwant %s", tt.in, got, tt.want)
		}
	}
}

func FuzzContent(f *testing.F) {
	f.Add([]byte("q 1 0 0 1 10 20 cm /Im0 Do Q BI /W 2 /H 1 /CS /G /BPC 8 ID EI\nEI Q"))
	f.Add([]byte("BI /W 9 /H 9 /F /AHx ID 4142EIx 4344 EI q BT 3 Tr (x) Tj ET"))
	f.Fuzz(func(t *testing.T, data []byte) {
		Content(data, func(op string, args []any) bool { return true })
	})
}
