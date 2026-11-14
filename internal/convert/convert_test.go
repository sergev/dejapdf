package convert

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/djvu"
)

const corpus = "../../testdata/corpus"

// Converted pages, rendered by mutool, match ddjvu -mode=mask
// (masks.sha256), and qpdf accepts the files.
func TestCorpusRender(t *testing.T) {
	for _, tool := range []string{"mutool", "qpdf"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	f, err := os.Open(filepath.Join(corpus, "masks.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dir := t.TempDir()
	pdfs := map[string]*djvu.Document{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		w := strings.Fields(sc.Text())
		out := filepath.Join(dir, strings.TrimSuffix(w[1], ".djvu")+".pdf")
		doc := pdfs[w[1]]
		if doc == nil {
			if doc, err = djvu.Open(filepath.Join(corpus, w[1])); err != nil {
				t.Fatal(err)
			}
			pdfs[w[1]] = doc
			g, err := os.Create(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := Convert(doc, g, Options{Producer: "test"}); err != nil {
				t.Fatalf("%s: %v", w[1], err)
			}
			g.Close()
			if b, err := exec.Command("qpdf", "--check", out).CombinedOutput(); err != nil {
				t.Errorf("%s: qpdf: %v\n%s", w[1], err, b)
			}
		}
		var page int
		fmt.Sscan(w[2], &page)
		dpi := doc.Pages[page-1].Info.DPI
		pbm := filepath.Join(dir, "page.pbm")
		if b, err := exec.Command("mutool", "draw", "-q", "-r", fmt.Sprint(dpi), "-c", "mono",
			"-o", pbm, out, w[2]).CombinedOutput(); err != nil {
			t.Fatalf("mutool: %v\n%s", err, b)
		}
		b, err := os.ReadFile(pbm)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != w[0] {
			t.Errorf("%s page %d: render differs from ddjvu mask", w[1], page)
		}
	}
}
