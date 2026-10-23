package lsd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

// Encoding selects the text encoding of exported DSL files.
type Encoding int

const (
	// UTF16LE is the encoding ABBYY DSL Compiler expects (with BOM).
	UTF16LE Encoding = iota
	// UTF8 is convenient for further processing with standard tools.
	UTF8
)

// ExportOptions configure [Dictionary.WriteDSL]. The zero value produces a
// UTF-16LE file without an icon reference.
type ExportOptions struct {
	Encoding Encoding
	// IconFile, if not empty, is written as the #ICON_FILE directive.
	// Save [Dictionary.Icon] under this name next to the DSL file.
	IconFile string
}

// WriteDSL writes the whole dictionary in the ABBYY DSL source format:
// a header with #NAME, #INDEX_LANGUAGE and #CONTENTS_LANGUAGE directives,
// followed by entries. Each entry consists of one or more heading lines
// and tab-indented article lines.
//
// Headings that share an article are written as one entry. Variant
// headings such as "aa(bb)cc" are not collapsed back and appear as
// separate headings.
func (d *Dictionary) WriteDSL(w io.Writer, opts ExportOptions) error {
	bw := bufio.NewWriterSize(w, 1<<16)
	out := newTextWriter(bw, opts.Encoding)

	info := d.Info()
	out.directive("NAME", info.Name)
	out.directive("INDEX_LANGUAGE", info.Source.String())
	out.directive("CONTENTS_LANGUAGE", info.Target.String())
	if opts.IconFile != "" {
		out.directive("ICON_FILE", opts.IconFile)
	}
	out.write("\n")

	for e := range d.Entries() {
		article, err := e.Article()
		if err != nil {
			return err
		}
		for _, h := range e.Headings {
			out.write(h.DSL)
			out.write("\n")
		}
		for _, line := range strings.Split(article, "\n") {
			out.write("\t")
			out.write(strings.TrimSuffix(line, "\r"))
			out.write("\n")
		}
		if out.err != nil {
			return out.err
		}
	}
	if out.err != nil {
		return out.err
	}
	return bw.Flush()
}

// Article decodes the article shared by the entry's headings.
func (e Entry) Article() (string, error) {
	if len(e.Headings) == 0 {
		return "", fmt.Errorf("lsd: empty entry")
	}
	h := e.Headings[0]
	return h.owner.Article(h)
}

// textWriter writes strings in the selected encoding with a sticky error.
type textWriter struct {
	w   io.Writer
	enc Encoding
	buf []byte
	err error
}

func newTextWriter(w io.Writer, enc Encoding) *textWriter {
	t := &textWriter{w: w, enc: enc}
	if enc == UTF16LE {
		t.writeBytes([]byte{0xFF, 0xFE}) // byte order mark
	}
	return t
}

// directive writes a header line such as: #NAME	"Value"
func (t *textWriter) directive(name, value string) {
	t.write("#" + name + "\t\"" + value + "\"\n")
}

func (t *textWriter) write(s string) {
	if t.enc == UTF8 {
		t.writeBytes([]byte(s))
		return
	}
	t.buf = t.buf[:0]
	for _, u := range utf16.Encode([]rune(s)) {
		t.buf = append(t.buf, byte(u), byte(u>>8))
	}
	t.writeBytes(t.buf)
}

func (t *textWriter) writeBytes(b []byte) {
	if t.err == nil {
		_, t.err = t.w.Write(b)
	}
}
