// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/pdf"
)

func need(t *testing.T, tools ...string) {
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
}

// convertFile converts a corpus document and returns the PDF path.
func convertFile(t *testing.T, name string) string {
	t.Helper()
	doc, err := djvu.Open(filepath.Join(corpus, name))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.pdf")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Convert(doc, f, Options{Producer: "test", MaskOnly: true}); err != nil {
		t.Fatal(err)
	}
	return out
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	b, err := exec.Command(name, args...).Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(b)
}

// The text pdftotext and MuPDF extract from each page has the words
// djvutxt prints.
func TestCorpusText(t *testing.T) {
	need(t, "pdftotext", "mutool", "djvutxt")
	for _, name := range []string{"bundled.djvu", "pdf2djvu.djvu", "indirect/index.djvu"} {
		out := convertFile(t, name)
		doc, _ := djvu.Open(filepath.Join(corpus, name))
		for p := 1; p <= len(doc.Pages); p++ {
			want := strings.Fields(run(t, "djvutxt", fmt.Sprintf("--page=%d", p), filepath.Join(corpus, name)))
			ps := fmt.Sprint(p)
			for tool, args := range map[string][]string{
				"pdftotext": {"-f", ps, "-l", ps, out, "-"},
				"mutool":    {"draw", "-q", "-F", "txt", "-o", "-", out, ps},
			} {
				got := strings.Fields(run(t, tool, args...))
				if strings.Join(got, " ") != strings.Join(want, " ") {
					t.Errorf("%s page %d, %s:\n got %q\nwant %q", name, p, tool, got, want)
				}
			}
		}
	}
}

// Bookmarks and links lead where djvused says they do.
func TestCorpusOutlineLinks(t *testing.T) {
	need(t, "mutool")
	js := filepath.Join(t.TempDir(), "links.js")
	os.WriteFile(js, []byte(`var doc = Document.openDocument(scriptArgs[0]);
for (var i = 0; i < doc.countPages(); i++) {
  var l = doc.loadPage(i).getLinks();
  for (var k = 0; k < l.length; k++) print(i + 1, l[k].getBounds().map(Math.round), l[k].getURI());
}
`), 0o644)
	outline := `|	"Text"	#page=1
+	"Pictures"	#page=2
|		"Background"	#page=2
|		"%s"	#page=3
|	"%s"	#page=4
`
	links := "1 72,720,240,737 https://example.com/\n1 300,720,%d,737 #page=3\n"
	for _, c := range []struct{ name, outline, links string }{
		{"bundled.djvu", fmt.Sprintf(outline, "Colours", "Compound"), fmt.Sprintf(links, 401)},
		{"indirect/index.djvu", fmt.Sprintf(outline, "Colours", "Compound"), fmt.Sprintf(links, 401)},
		{"pdf2djvu.djvu", fmt.Sprintf(outline, "Coloured text", "More text"), fmt.Sprintf(links, 400)},
	} {
		out := convertFile(t, c.name)
		if got := run(t, "mutool", "show", out, "outline"); got != c.outline {
			t.Errorf("%s outline:\n%s\nwant\n%s", c.name, got, c.outline)
		}
		if got := run(t, "mutool", "run", js, out); got != c.links {
			t.Errorf("%s links:\n%s\nwant\n%s", c.name, got, c.links)
		}
	}
}

func TestMetadata(t *testing.T) {
	need(t, "pdfinfo")
	out := convertFile(t, "pdf2djvu.djvu")
	info := run(t, "pdfinfo", "-isodates", out)
	for _, want := range []string{
		"Title:           dejapdf test corpus source",
		"Producer:        test",
		"CreationDate:    2026-11-13T19:38:50-08",
	} {
		if !strings.Contains(info, want) {
			t.Errorf("pdfinfo lacks %q:\n%s", want, info)
		}
	}
}

func TestPDFDate(t *testing.T) {
	for in, want := range map[string]string{
		"2026-11-13 19:38:50-08:00": "D:20261113193850-08'00'",
		"2026-11-13T19:38:50Z":      "D:20261113193850Z",
		"2026-11-13 19:38:50":       "D:20261113193850",
		"2026-11-13":                "D:20261113000000",
	} {
		if got, ok := pdfDate(in); !ok || got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	if _, ok := pdfDate("yesterday"); ok {
		t.Error("parsed a bad date")
	}
}

func TestResolve(t *testing.T) {
	tg := &targets{byName: map[string]int{"p0003.djvu": 2}, pages: []pdf.Ref{1, 2, 3, 4}}
	for url, want := range map[string]int{
		"#p0003.djvu": 2, "#1": 0, "#4": 3, "#5": -1, "#0": -1,
		"#+1": 2, "#-1": 0, "#-2": -1, "#nowhere": -1,
	} {
		if got, _ := tg.resolve(url, 1); got != want {
			t.Errorf("%q: got %d, want %d", url, got, want)
		}
	}
	if i, uri := tg.resolve("http://x/", 0); i != -1 || uri != "http://x/" {
		t.Errorf("URI: %d %q", i, uri)
	}
}

func TestArea(t *testing.T) {
	for in, want := range map[string]string{
		"(rect 10 20 30 40)":     "[10 20 40 60] true",
		"(oval 1 2 3 4)":         "[1 2 4 6] true",
		"(poly 0 0 10 5 3 9)":    "[0 0 10 9] true",
		"(rect 1 2 0 4)":         "[0 0 0 0] false",
		"(line 0 0 10 10)":       "[0 0 0 0] false",
		"(text 1 2 3 4)":         "[0 0 0 0] false",
		"(rect 1 2 x 4)":         "[0 0 0 0] false",
		"(poly 0 0 10 5 3)":      "[0 0 0 0] false",
		"(rect 1 2 3 (4))":       "[0 0 0 0] false",
		"(poly 0 0 10 0 20 0)":   "[0 0 0 0] false",
		"(rect)":                 "[0 0 0 0] false",
		"(oval 1.5 2 3 4 extra)": "[0 0 0 0] false",
		"(poly 1 1 2 2 3 3 4 4)": "[1 1 4 4] true",
		"(rect -10 -20 30 40)":   "[-10 -20 20 20] true",
		"(oval 0 0 1e1 1)":       "[0 0 10 1] true",
	} {
		s := djvu.ParseSexps([]byte(in))
		r, ok := area(s[0])
		if got := fmt.Sprintf("%v %v", r, ok); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}

// The size report adds up to the file sizes; a page selection keeps
// only the bookmarks and links that lead to converted pages.
func TestStatsAndSelection(t *testing.T) {
	need(t, "mutool")
	name := filepath.Join(corpus, "bundled.djvu")
	doc, err := djvu.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.pdf")
	f, _ := os.Create(out)
	var st Stats
	if err := Convert(doc, f, Options{Stats: &st}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	sum := func(a [nKinds]int64) (n int64) {
		for _, v := range a {
			n += v
		}
		return
	}
	fi, _ := os.Stat(out)
	di, _ := os.Stat(name)
	if sum(st.PDF) != fi.Size() || sum(st.DjVu) != di.Size() {
		t.Errorf("stats pdf %d djvu %d, files %d %d", sum(st.PDF), sum(st.DjVu), fi.Size(), di.Size())
	}
	if st.PDF[Mask] == 0 || st.PDF[Colour] == 0 || st.PDF[Text] == 0 || st.DjVu[Text] == 0 {
		t.Errorf("empty kinds: %+v", st)
	}

	f, _ = os.Create(out)
	if err := Convert(doc, f, Options{Pages: []int{0, 3}}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	want := "|\t\"Text\"\t#page=1\n|\t\"Compound\"\t#page=2\n"
	if got := run(t, "mutool", "show", out, "outline"); got != want {
		t.Errorf("outline:\n%s\nwant\n%s", got, want)
	}
	if got := run(t, "mutool", "show", out, "trailer/Root/Pages/Count"); strings.TrimSpace(got) != "2" {
		t.Errorf("page count %q", got)
	}
	f, _ = os.Create(out)
	if err := Convert(doc, f, Options{Pages: []int{0, 0}}); err == nil {
		t.Error("duplicate page accepted")
	}
	f.Close()
}
