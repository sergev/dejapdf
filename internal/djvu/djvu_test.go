// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/iff"
)

const corpus = "../../testdata/corpus"

func TestDumpCorpus(t *testing.T) {
	goldens, _ := filepath.Glob(filepath.Join(corpus, "*.dump"))
	more, _ := filepath.Glob(filepath.Join(corpus, "*", "*.dump"))
	goldens = append(goldens, more...)
	if len(goldens) == 0 {
		t.Fatal("no goldens")
	}
	for _, g := range goldens {
		want, err := os.ReadFile(g)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(strings.TrimSuffix(g, ".dump") + ".djvu")
		if err != nil {
			t.Fatal(err)
		}
		root, err := iff.Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", g, err)
		}
		var buf bytes.Buffer
		if err := Dump(&buf, root); err != nil {
			t.Fatal(err)
		}
		if got := buf.String(); got != string(want) {
			t.Errorf("%s:\n--- got\n%s--- want\n%s", g, got, want)
		}
	}
}

func TestParseInfo(t *testing.T) {
	mk := func(minor, major byte, dpi uint16, gamma, flags byte) []byte {
		b := binary.BigEndian.AppendUint16(nil, 2550)
		b = binary.BigEndian.AppendUint16(b, 3300)
		b = append(b, minor, major)
		b = binary.LittleEndian.AppendUint16(b, dpi)
		return append(b, gamma, flags)
	}
	cases := []struct {
		b    []byte
		want Info
	}{
		{mk(24, 0, 300, 22, 1), Info{2550, 3300, 24, 300, 22, 0}},
		{mk(24, 1, 600, 22, 6), Info{2550, 3300, 280, 600, 22, 270}},
		{mk(24, 0, 24, 2, 2), Info{2550, 3300, 24, 300, 3, 180}},
		{mk(24, 0, 6001, 51, 5), Info{2550, 3300, 24, 300, 50, 90}},
		{mk(24, 0, 25, 3, 0x0d), Info{2550, 3300, 24, 25, 3, 90}},
		{mk(24, 0, 6000, 50, 0)[:4], Info{2550, 3300, 0, 300, 22, 0}},
	}
	for i, c := range cases {
		got, err := ParseInfo(c.b)
		if err != nil || got != c.want {
			t.Errorf("case %d: got %+v, %v; want %+v", i, got, err, c.want)
		}
	}
	if _, err := ParseInfo([]byte{1, 2, 3}); err == nil {
		t.Error("short INFO accepted")
	}
}

func TestDirBody(t *testing.T) {
	d := &Dir{Components: make([]Component, 3)}
	body := []byte{0, 1, 2, 0, 0, 3, 1, 0, 0}
	body = append(body, 0x01, 0xc1, 0x03)
	body = append(body, "a.djvu\x00b.djvu\x00b-file.djvu\x00Two\x00s.iff\x00"...)
	if err := d.parseDirBody(body); err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{ID: "a.djvu", Name: "a.djvu", Title: "a.djvu", Type: PageComp, Size: 0x102},
		{ID: "b.djvu", Name: "b-file.djvu", Title: "Two", Type: PageComp, Size: 3, HasName: true, HasTitle: true},
		{ID: "s.iff", Name: "s.iff", Title: "s.iff", Type: SharedAnno, Size: 0x10000},
	}
	for i := range want {
		if d.Components[i] != want[i] {
			t.Errorf("%d: got %+v want %+v", i, d.Components[i], want[i])
		}
	}
	if d.NumPages() != 2 {
		t.Errorf("pages %d", d.NumPages())
	}
	if err := d.parseDirBody(body[:len(body)-1]); err == nil {
		t.Error("unterminated name accepted")
	}
	if err := d.parseDirBody(body[:5]); err == nil {
		t.Error("truncated body accepted")
	}
}

func TestSinglePage(t *testing.T) {
	d, err := Open(filepath.Join(corpus, "photo.djvu"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 1 || d.Dir != nil {
		t.Fatalf("pages %d", len(d.Pages))
	}
	p := d.Pages[0]
	if p.Info != (Info{850, 1100, 24, 100, 22, 0}) {
		t.Errorf("info %+v", p.Info)
	}
	if n := len(p.Chunks("BG44")); n != 3 {
		t.Errorf("BG44 chunks %d", n)
	}
}

// A component page of an indirect document opened on its own: INCL
// names a file next to it.
func TestIncludeFile(t *testing.T) {
	d, err := Open(filepath.Join(corpus, "indirect", "bilevel.djvu"))
	if err != nil {
		t.Fatal(err)
	}
	p := d.Pages[0]
	if len(p.Includes) != 1 || len(p.Chunks("ANTz")) != 2 {
		t.Fatalf("includes %d, ANTz %d", len(p.Includes), len(p.Chunks("ANTz")))
	}
	// Without a loader the include is missing: a warning.
	d, err = Parse(mustRead(t, filepath.Join(corpus, "indirect", "bilevel.djvu")), nil)
	if err != nil || len(d.Warnings) != 1 || len(d.Pages[0].Includes) != 0 {
		t.Errorf("INCL without loader: %v, warnings %v", err, d.Warnings)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBundled(t *testing.T) {
	d, err := Open(filepath.Join(corpus, "shared-dict.djvu"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 2 || !d.Dir.Bundled || len(d.Dir.Components) != 3 {
		t.Fatalf("pages %d", len(d.Pages))
	}
	for _, p := range d.Pages {
		if len(p.Chunks("Djbz")) != 1 || len(p.Chunks("Sjbz")) != 1 {
			t.Errorf("%s: Djbz %d Sjbz %d", p.ID, len(p.Chunks("Djbz")), len(p.Chunks("Sjbz")))
		}
	}
	if d.Pages[0].Includes[0] != d.Pages[1].Includes[0] {
		t.Error("shared dictionary not shared")
	}
	if d.Pages[0].ID != "p1.djvu" || d.Pages[1].ID != "p4.djvu" {
		t.Errorf("ids %q %q", d.Pages[0].ID, d.Pages[1].ID)
	}
}

func TestIndirect(t *testing.T) {
	d, err := Open(filepath.Join(corpus, "indirect", "index.djvu"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 4 || d.Dir.Bundled {
		t.Fatalf("pages %d", len(d.Pages))
	}
	for _, p := range d.Pages {
		if len(p.Includes) != 1 || p.Includes[0] != d.Pages[0].Includes[0] {
			t.Errorf("%s: shared annotations not resolved", p.ID)
		}
	}
	if d.Pages[3].Info.Width != 2550 || len(d.Pages[3].Chunks("FG44")) != 1 {
		t.Error("compound page wrong")
	}
}

// Bundled and indirect forms of the same document agree.
func TestBundledMatchesIndirect(t *testing.T) {
	b, err := Open(filepath.Join(corpus, "bundled.djvu"))
	if err != nil {
		t.Fatal(err)
	}
	i, err := Open(filepath.Join(corpus, "indirect", "index.djvu"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Pages) != len(i.Pages) {
		t.Fatalf("pages %d vs %d", len(b.Pages), len(i.Pages))
	}
	for n := range b.Pages {
		p, q := b.Pages[n], i.Pages[n]
		if p.ID != q.ID || p.Info != q.Info || len(p.Form.Children) != len(q.Form.Children) {
			t.Errorf("page %d differs", n+1)
		}
	}
}
