// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/djvu"
)

func TestParseLabel(t *testing.T) {
	for in, want := range map[string]string{
		"1": "{ D 1}", "C1": "{C D 1}", "A-12": "{A- D 12}", "xiv": "{ r 14}", "XIV": "{ R 14}",
		"iiii": "{iiii  0}", "Xiv": "{Xiv  0}", "Cover": "{Cover  0}", "07": "{07  0}",
		"p0": "{p0  0}", "1234567890": "{1234567890  0}", "mmmcmxcix": "{ r 3999}",
	} {
		if got := fmt.Sprint(parseLabel(in)); got != want {
			t.Errorf("%q: got %s, want %s", in, got, want)
		}
	}
	for n := 1; n < 4000; n++ {
		if fromRoman(roman(n), false) != n || fromRoman(strings.ToUpper(roman(n)), true) != n {
			t.Fatalf("roman %d", n)
		}
	}
}

func TestPageLabels(t *testing.T) {
	doc := func(titles ...string) *djvu.Document {
		d := &djvu.Document{Dir: &djvu.Dir{}, Pages: make([]*djvu.Page, len(titles))}
		for _, s := range titles {
			c := djvu.Component{Type: djvu.PageComp, Title: s, HasTitle: s != "-"}
			d.Dir.Components = append(d.Dir.Components, c)
		}
		return d
	}
	all := func(n int) []int {
		r := make([]int, n)
		for i := range r {
			r[i] = i
		}
		return r
	}
	for _, c := range []struct {
		titles []string
		sel    []int
		want   string
	}{
		{[]string{"1", "2", "3"}, nil, "map[]"},
		{[]string{"-", "-", "-"}, nil, "map[]"},
		{[]string{"", "", ""}, nil, "map[]"},
		{[]string{"C1", "i", "ii", "1", "2", "4"}, nil,
			"map[Nums:[0 map[P:C S:D] 1 map[S:r] 3 map[S:D] 5 map[S:D St:4]]]"},
		{[]string{"Cover", "Cover", "1"}, nil, "map[Nums:[0 map[P:Cover] 2 map[S:D]]]"},
		{[]string{"-", "-", "-", "-"}, []int{2, 3}, "map[Nums:[0 map[S:D St:3]]]"},
		{[]string{"-", "-", "-", "-"}, []int{3, 0}, "map[Nums:[0 map[S:D St:4] 1 map[S:D]]]"},
	} {
		sel := c.sel
		if sel == nil {
			sel = all(len(c.titles))
		}
		got := fmt.Sprint(map[string]any(pageLabels(doc(c.titles...), sel)))
		if got != c.want {
			t.Errorf("%q %v: got %s, want %s", c.titles, sel, got, c.want)
		}
	}
}

// The XMP packet is carried over with the format and producer of the PDF.
func TestXMP(t *testing.T) {
	need(t, "pdfinfo")
	out := convertFile(t, "pdf2djvu.djvu")
	meta := run(t, "pdfinfo", "-meta", out)
	for _, want := range []string{"<dc:format>application/pdf</dc:format>",
		"<pdf:Producer>test</pdf:Producer>", "<xmpMM:DocumentID>"} {
		if !strings.Contains(meta, want) {
			t.Errorf("XMP lacks %q", want)
		}
	}
}
