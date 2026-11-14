// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/pdf"
)

// pageNum stands for the reference of page i until the page is written.
type pageNum int

// targets resolves DjVu link URLs.
type targets struct {
	byName map[string]int
	pages  []pdf.Ref // 0 for pages not converted
}

func newTargets(doc *djvu.Document, pages []pdf.Ref) *targets {
	t := &targets{byName: map[string]int{}, pages: pages}
	if doc.Dir != nil {
		k := 0
		for _, c := range doc.Dir.Components {
			if c.Type != djvu.PageComp {
				continue
			}
			for _, s := range []string{c.Title, c.Name, c.ID} {
				if _, ok := t.byName[s]; !ok && s != "" {
					t.byName[s] = k
				}
			}
			k++
		}
	}
	for i, pg := range doc.Pages {
		if _, ok := t.byName[pg.ID]; !ok && pg.ID != "" {
			t.byName[pg.ID] = i
		}
	}
	return t
}

// resolve turns url, seen from page cur, into the index of a converted
// page, or -1 and a URI. "#" links name a page id or title, a number
// from 1, or a relative +n or -n.
func (t *targets) resolve(url string, cur int) (int, string) {
	if !strings.HasPrefix(url, "#") {
		return -1, url
	}
	n := len(t.pages)
	s := url[1:]
	if i, ok := t.byName[s]; ok {
		if t.pages[i] == 0 {
			return -1, ""
		}
		return i, ""
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return -1, ""
	}
	if s[0] == '+' || s[0] == '-' {
		v += cur
	} else {
		v--
	}
	if v < 0 || v >= n || t.pages[v] == 0 {
		return -1, ""
	}
	return v, ""
}

// action is a link's /Dest or /A entry, or "" if it leads nowhere.
func (t *targets) action(url string, cur int) (string, any) {
	i, uri := t.resolve(url, cur)
	switch {
	case i >= 0:
		return "Dest", pdf.Array{pageNum(i), pdf.Name("XYZ"), nil, nil, nil}
	case uri != "":
		return "A", pdf.Dict{"S": pdf.Name("URI"), "URI": pdf.String(uri)}
	}
	return "", nil
}

// links adds the page's hyperlinks (maparea annotations).
func links(b *pageBuilder, pg *djvu.Page, t *targets, cur int) error {
	ann, err := pg.Annotations()
	if err != nil {
		return err
	}
	s := 72 / float64(b.in.DPI)
	for _, a := range ann {
		if a.Head() != "maparea" || len(a.List) < 4 {
			continue
		}
		url := a.List[1]
		if url.IsList && url.Head() == "url" && len(url.List) >= 2 {
			url = url.List[1]
		}
		if url.IsList {
			continue
		}
		key, act := t.action(url.Atom, cur)
		if key == "" {
			continue
		}
		r, ok := area(a.List[3])
		if !ok {
			continue
		}
		d := pdf.Dict{
			"Type": pdf.Name("Annot"), "Subtype": pdf.Name("Link"),
			"Rect":   pdf.Array{r[0] * s, r[1] * s, r[2] * s, r[3] * s},
			"Border": pdf.Array{0, 0, 0}, key: act,
		}
		if c := a.List[2]; !c.IsList && c.Atom != "" {
			d["Contents"] = pdf.TextString(c.Atom)
		}
		b.annots = append(b.annots, d)
	}
	return nil
}

// area returns the bounding box of a rect, oval or poly shape.
func area(sh *djvu.Sexp) ([4]float64, bool) {
	var v []float64
	for _, a := range sh.List[min(1, len(sh.List)):] {
		f, err := strconv.ParseFloat(a.Atom, 64)
		if a.IsList || err != nil {
			return [4]float64{}, false
		}
		v = append(v, f)
	}
	switch sh.Head() {
	case "rect", "oval":
		if len(v) == 4 && v[2] > 0 && v[3] > 0 {
			return [4]float64{v[0], v[1], v[0] + v[2], v[1] + v[3]}, true
		}
	case "poly":
		if len(v) >= 6 && len(v)%2 == 0 {
			r := [4]float64{v[0], v[1], v[0], v[1]}
			for i := 2; i < len(v); i += 2 {
				r[0], r[1] = min(r[0], v[i]), min(r[1], v[i+1])
				r[2], r[3] = max(r[2], v[i]), max(r[3], v[i+1])
			}
			if r[2] > r[0] && r[3] > r[1] {
				return r, true
			}
		}
	}
	return [4]float64{}, false
}

// prune drops bookmarks that lead to no converted page and have no
// children left.
func (t *targets) prune(bms []*djvu.Bookmark) []*djvu.Bookmark {
	var r []*djvu.Bookmark
	for _, bm := range bms {
		k := *bm
		k.Children = t.prune(bm.Children)
		if key, _ := t.action(bm.URL, 0); key != "" || len(k.Children) > 0 {
			r = append(r, &k)
		}
	}
	return r
}

// outline writes the bookmarks and returns the outline dictionary.
func outline(p *pdf.Writer, bms []*djvu.Bookmark, t *targets, pages []pdf.Ref) pdf.Ref {
	root := p.Alloc()
	first, last := items(p, bms, root, t, pages)
	p.Object(root, pdf.Dict{"Type": pdf.Name("Outlines"), "First": first, "Last": last, "Count": len(bms)})
	return root
}

// items writes a list of bookmarks, closed, under parent.
func items(p *pdf.Writer, bms []*djvu.Bookmark, parent pdf.Ref, t *targets, pages []pdf.Ref) (pdf.Ref, pdf.Ref) {
	refs := make([]pdf.Ref, len(bms))
	for i := range refs {
		refs[i] = p.Alloc()
	}
	for i, bm := range bms {
		d := pdf.Dict{"Title": pdf.TextString(bm.Title), "Parent": parent}
		if i > 0 {
			d["Prev"] = refs[i-1]
		}
		if i+1 < len(refs) {
			d["Next"] = refs[i+1]
		}
		if len(bm.Children) > 0 {
			d["First"], d["Last"] = items(p, bm.Children, refs[i], t, pages)
			d["Count"] = -len(bm.Children)
		}
		if key, act := t.action(bm.URL, 0); key != "" {
			if a, ok := act.(pdf.Array); ok {
				a[0] = pages[a[0].(pageNum)]
			}
			d[key] = act
		}
		p.Object(refs[i], d)
	}
	if len(refs) == 0 {
		return 0, 0
	}
	return refs[0], refs[len(refs)-1]
}

// infoKeys maps DjVu metadata keys, lower case, to document info keys.
var infoKeys = map[string]string{
	"title": "Title", "author": "Author", "subject": "Subject", "keywords": "Keywords",
	"creator": "Creator", "creationdate": "CreationDate", "moddate": "ModDate",
}

// metadata fills info from the page's (metadata ...) annotations.
func metadata(pg *djvu.Page, info pdf.Dict) {
	ann, _ := pg.Annotations()
	for _, a := range ann {
		if a.Head() != "metadata" {
			continue
		}
		for _, kv := range a.List[1:] {
			if len(kv.List) != 2 || kv.List[1].IsList {
				continue
			}
			k, ok := infoKeys[strings.ToLower(kv.Head())]
			if !ok {
				continue
			}
			v := kv.List[1].Atom
			if strings.HasSuffix(k, "Date") {
				d, ok := pdfDate(v)
				if !ok {
					continue
				}
				info[k] = pdf.String(d)
				continue
			}
			info[k] = pdf.TextString(v)
		}
	}
}

// pdfDate converts a date like djvused's "2006-01-02 15:04:05-07:00"
// to a PDF date (7.9.4).
func pdfDate(s string) (string, bool) {
	for _, f := range []string{"2006-01-02 15:04:05Z07:00", time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		t, err := time.Parse(f, strings.TrimSpace(s))
		if err != nil {
			continue
		}
		d := t.Format("D:20060102150405")
		_, off := t.Zone()
		switch {
		case off == 0 && f != "2006-01-02 15:04:05" && f != "2006-01-02":
			d += "Z"
		case off != 0:
			sign := '+'
			if off < 0 {
				sign, off = '-', -off
			}
			d += fmt.Sprintf("%c%02d'%02d'", sign, off/3600, off/60%60)
		}
		return d, true
	}
	return "", false
}
