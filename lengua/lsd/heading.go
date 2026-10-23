package lsd

import (
	"strings"
	"unicode/utf16"
)

// Heading is a headword of a dictionary entry.
//
// LSD headings consist of "sorted" characters, which take part in searching,
// and "unsorted" ones, which are only displayed. In DSL notation unsorted
// parts are wrapped in curly braces: the heading "Some{thing}" is found by
// searching for "Some" and displayed as "Something".
//
// Headings are obtained from a [Dictionary] and can only be passed back to
// the dictionary they came from.
type Heading struct {
	// Text is the searchable (sorted) part of the heading.
	Text string
	// DSL is the complete heading in DSL notation, including unsorted parts
	// in curly braces and backslash escapes. It equals Text for most entries.
	DSL string

	owner *Dictionary
	index int32  // position of the heading in the file
	ref   uint32 // article offset relative to the articles block
}

// Display returns the heading as it is shown to the user: all characters,
// without DSL braces and escapes.
func (h Heading) Display() string {
	return StripDSLHeading(h.DSL)
}

// Index returns the position of the heading in file order. Headings are
// numbered from 0 to [Dictionary.Len]-1.
func (h Heading) Index() int { return int(h.index) }

// StripDSLHeading converts a heading in DSL notation to display form by
// removing unsorted-part braces and backslash escapes:
// `Some{thing} \(x\)` becomes `Something (x)`.
func StripDSLHeading(dsl string) string {
	if !strings.ContainsAny(dsl, `{}\`) {
		return dsl
	}
	var b strings.Builder
	escaped := false
	for _, r := range dsl {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '{' || r == '}':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// headingChar is a character of a decoded heading together with its flags.
type headingChar struct {
	ch      uint16
	sorted  bool
	escaped bool
}

// extChar is an unsorted character inserted at a given position.
type extChar struct {
	pos int
	ch  uint16
}

// rawHeading is a heading as it is stored on a leaf page.
type rawHeading struct {
	sorted []uint16 // sorted characters; becomes the prefix of the next heading
	ext    []extChar
	ref    uint32
}

// readHeading decodes one heading. known holds the sorted text of the
// previous heading on the page; headings share common prefixes with it.
func readHeading(r *bitReader, c *codec, known []uint16) (rawHeading, error) {
	prefixLen, err := c.decodePrefixLen(r)
	if err != nil {
		return rawHeading{}, err
	}
	postfixLen, err := c.decodePostfixLen(r)
	if err != nil {
		return rawHeading{}, err
	}
	postfix, err := c.decodeHeading(r, postfixLen)
	if err != nil {
		return rawHeading{}, err
	}

	h := rawHeading{ref: readReference(r, c.refBits2)}
	prefixLen = min(prefixLen, len(known))
	h.sorted = append(append(make([]uint16, 0, prefixLen+len(postfix)), known[:prefixLen]...), postfix...)

	if r.bit() == 1 {
		n := int(r.bits(8))
		h.ext = make([]extChar, n)
		for i := range h.ext {
			h.ext[i] = extChar{pos: int(r.bits(8)), ch: uint16(r.bits(16))}
		}
	}
	return h, r.err()
}

// chars merges sorted and unsorted characters into one sequence. A
// backslash marks the following character as escaped and is consumed.
// This mirrors ArticleHeading::makeCharsFromPairs in lsd2dsl.
func (h rawHeading) chars() []headingChar {
	text, ext := h.sorted, h.ext
	out := make([]headingChar, 0, len(text)+len(ext))
	pos := 0
	next := func() (uint16, bool, bool) {
		switch {
		case len(ext) > 0 && (ext[0].pos == pos || len(text) == 0):
			c := ext[0].ch
			ext = ext[1:]
			return c, false, true
		case len(text) > 0:
			c := text[0]
			text = text[1:]
			return c, true, true
		}
		return 0, false, false
	}
	for len(text) > 0 || len(ext) > 0 {
		c, sorted, _ := next()
		info := headingChar{ch: c, sorted: sorted}
		if c == '\\' {
			pos++
			if c2, sorted2, ok := next(); ok {
				info = headingChar{ch: c2, sorted: sorted2, escaped: true}
			}
		}
		out = append(out, info)
		pos++
	}
	return out
}

// texts returns the sorted text and the DSL form of the heading.
func (h rawHeading) texts() (text, dsl string) {
	if len(h.ext) == 0 && !containsBackslash(h.sorted) {
		s := string(utf16.Decode(h.sorted))
		return s, s
	}
	var sortedUnits, dslUnits []uint16
	inGroup := false
	for _, c := range h.chars() {
		if c.sorted {
			sortedUnits = append(sortedUnits, c.ch)
		}
		if inGroup && c.sorted {
			dslUnits = append(dslUnits, '}')
			inGroup = false
		} else if !inGroup && !c.sorted {
			dslUnits = append(dslUnits, '{')
			inGroup = true
		}
		if c.escaped {
			dslUnits = append(dslUnits, '\\')
		}
		dslUnits = append(dslUnits, c.ch)
	}
	if inGroup {
		dslUnits = append(dslUnits, '}')
	}
	return string(utf16.Decode(sortedUnits)), string(utf16.Decode(dslUnits))
}

func containsBackslash(s []uint16) bool {
	for _, c := range s {
		if c == '\\' {
			return true
		}
	}
	return false
}
