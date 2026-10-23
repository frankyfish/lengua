package lsd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// testDictionary opens a dictionary from the Lingvo installation named by
// the LINGVO_DATA environment variable and skips the test if it's not set.
func testDictionary(t *testing.T, file string) *Dictionary {
	t.Helper()
	root := os.Getenv("LINGVO_DATA")
	if root == "" {
		t.Skip("LINGVO_DATA is not set")
	}
	matches, _ := filepath.Glob(filepath.Join(root, "Data", "*", "LingvoDic", file))
	if len(matches) == 0 {
		t.Skipf("%s not found under %s", file, root)
	}
	d, err := Open(matches[0])
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestDictionaryLookup(t *testing.T) {
	d := testDictionary(t, "LingvoUniversalEnRu.lsd")
	info := d.Info()
	if info.Source != English || info.Target != Russian {
		t.Errorf("languages = %v-%v, want English-Russian", info.Source, info.Target)
	}
	if d.Len() != info.HeadingCount {
		t.Errorf("Len() = %d, header says %d", d.Len(), info.HeadingCount)
	}
	hs := d.Lookup("SERENE")
	if len(hs) == 0 {
		t.Fatal(`Lookup("SERENE") found nothing`)
	}
	article, err := d.Article(hs[0])
	if err != nil {
		t.Fatalf("Article: %v", err)
	}
	if !strings.Contains(article, "безмятежный") {
		t.Errorf("article of serene lacks the expected translation:\n%s", article)
	}
	for _, h := range d.Prefix("seren", 10) {
		if !strings.HasPrefix(FoldKey(h.Text), "seren") {
			t.Errorf("Prefix returned %q", h.Text)
		}
	}
}

func TestForeignHeading(t *testing.T) {
	a := testDictionary(t, "AccountingEnRu.lsd")
	b := testDictionary(t, "LingvoUniversalEnRu.lsd")
	if _, err := a.Article(b.Heading(0)); !errors.Is(err, ErrForeignHeading) {
		t.Errorf("Article(foreign) error = %v, want ErrForeignHeading", err)
	}
}

func TestAllArticlesDecode(t *testing.T) {
	d := testDictionary(t, "AccountingEnRu.lsd")
	n := 0
	for e := range d.Entries() {
		if _, err := e.Article(); err != nil {
			t.Fatalf("entry %v: %v", e.Headings[0].Text, err)
		}
		n++
	}
	if n == 0 {
		t.Fatal("no entries")
	}
}

func TestWriteDSL(t *testing.T) {
	d := testDictionary(t, "AccountingEnRu.lsd")
	var buf bytes.Buffer
	if err := d.WriteDSL(&buf, ExportOptions{Encoding: UTF8}); err != nil {
		t.Fatalf("WriteDSL: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "#NAME\t") || !strings.Contains(out, "#INDEX_LANGUAGE\t\"English\"") {
		t.Errorf("unexpected DSL header:\n%.300s", out)
	}

	buf.Reset()
	if err := d.WriteDSL(&buf, ExportOptions{}); err != nil {
		t.Fatalf("WriteDSL UTF-16: %v", err)
	}
	b := buf.Bytes()
	if len(b) < 2 || b[0] != 0xFF || b[1] != 0xFE {
		t.Fatal("UTF-16 output lacks a BOM")
	}
	units := make([]uint16, (len(b)-2)/2)
	for i := range units {
		units[i] = uint16(b[2+2*i]) | uint16(b[3+2*i])<<8
	}
	if string(utf16.Decode(units)) != out {
		t.Error("UTF-16 and UTF-8 exports differ")
	}
}

func TestReadInfo(t *testing.T) {
	d := testDictionary(t, "AccountingEnRu.lsd")
	info, err := ReadInfo(d.Path())
	if err != nil {
		t.Fatalf("ReadInfo: %v", err)
	}
	if info != d.Info() {
		t.Errorf("ReadInfo = %+v, Open().Info() = %+v", info, d.Info())
	}
}
