// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/pdf"
)

// label is a page label as a PDF label range sees it: a prefix, a
// numbering style (D, r, R or none) and a number.
type label struct {
	prefix, style string
	num           int
}

// parseLabel splits a page title: "C1" is prefix C, decimal 1; "xii"
// is roman 12; other titles are a prefix alone.
func parseLabel(t string) label {
	for _, st := range []string{"r", "R"} {
		if n := fromRoman(t, st == "R"); n > 0 {
			return label{"", st, n}
		}
	}
	i := len(t)
	for i > 0 && t[i-1] >= '0' && t[i-1] <= '9' {
		i--
	}
	if i < len(t) && t[i] != '0' && len(t)-i <= 9 {
		n, _ := strconv.Atoi(t[i:])
		return label{t[:i], "D", n}
	}
	return label{t, "", 0}
}

var romanDigits = []struct {
	v int
	s string
}{{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"}, {100, "c"}, {90, "xc"},
	{50, "l"}, {40, "xl"}, {10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"}}

func roman(n int) string {
	var b strings.Builder
	for _, d := range romanDigits {
		for ; n >= d.v; n -= d.v {
			b.WriteString(d.s)
		}
	}
	return b.String()
}

// fromRoman returns the value of a canonical roman numeral in the given
// case, or 0.
func fromRoman(s string, upper bool) int {
	if s == "" || len(s) > 15 {
		return 0
	}
	low := strings.ToLower(s)
	if (upper && strings.ToUpper(s) != s) || (!upper && low != s) {
		return 0
	}
	n, rest := 0, low
	for _, d := range romanDigits {
		for strings.HasPrefix(rest, d.s) {
			n += d.v
			rest = rest[len(d.s):]
		}
	}
	if rest != "" || n == 0 || n > 3999 || roman(n) != low {
		return 0
	}
	return n
}

// pageLabels returns the /PageLabels number tree for the converted pages
// sel, or nil if the labels are just the page numbers. Titles come from
// the directory; untitled pages are labelled by their page number in the
// DjVu document.
func pageLabels(doc *djvu.Document, sel []int) pdf.Dict {
	titles := make([]string, len(doc.Pages))
	if doc.Dir != nil {
		k := 0
		for _, c := range doc.Dir.Components {
			if c.Type == djvu.PageComp {
				if c.HasTitle && k < len(titles) {
					titles[k] = strings.TrimSpace(c.Title)
				}
				k++
			}
		}
	}
	var nums pdf.Array
	var prev label
	for k, i := range sel {
		l := label{"", "D", i + 1}
		if titles[i] != "" {
			l = parseLabel(titles[i])
		}
		cont := k > 0 && l.prefix == prev.prefix && l.style == prev.style &&
			(l.style == "" || l.num == prev.num+1)
		if !cont {
			d := pdf.Dict{}
			if l.prefix != "" {
				d["P"] = pdf.TextString(l.prefix)
			}
			if l.style != "" {
				d["S"] = pdf.Name(l.style)
				if l.num != 1 {
					d["St"] = l.num
				}
			}
			nums = append(nums, k, d)
		}
		prev = l
	}
	if len(nums) == 2 {
		if d := nums[1].(pdf.Dict); len(d) == 1 && d["S"] == pdf.Name("D") {
			return nil // 1, 2, 3...
		}
	}
	if len(nums) == 0 {
		return nil
	}
	return pdf.Dict{"Nums": nums}
}

var (
	xmpFormat   = regexp.MustCompile(`(<dc:format>)[^<]*(</dc:format>)`)
	xmpProducer = regexp.MustCompile(`(<pdf:Producer>)[^<]*(</pdf:Producer>)`)
)

// xmp returns the document's XMP packet from the page's (xmp "...")
// annotation, with the format and producer of the PDF, or nil.
func xmp(pg *djvu.Page, producer string) []byte {
	ann, _ := pg.Annotations()
	for _, a := range ann {
		if a.Head() != "xmp" || len(a.List) != 2 || !a.List[1].Quoted {
			continue
		}
		s := xmpFormat.ReplaceAllString(a.List[1].Atom, "${1}application/pdf${2}")
		if producer != "" {
			s = xmpProducer.ReplaceAllLiteralString(s, "<pdf:Producer>"+xmlEscape(producer)+"</pdf:Producer>")
		}
		return []byte(s)
	}
	return nil
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
