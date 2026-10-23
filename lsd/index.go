package lsd

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// FoldKey normalizes a word for dictionary lookup. It is applied both to
// headings and to queries, so lookups ignore:
//
//   - letter case ("Apple" matches "apple");
//   - combining accents used as stress marks ("за́мок" matches "замок");
//   - the difference between Russian "ё" and "е";
//   - typographic apostrophes (’ matches ').
//
// Precomposed letters with diacritics are kept as they are, so French "où"
// and "ou" remain different words.
func FoldKey(s string) string {
	if isASCIILower(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'ё' || r == 'Ё':
			r = 'е'
		case r == '’' || r == 'ʼ':
			r = '\''
		default:
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isASCIILower(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 0x80 || ('A' <= c && c <= 'Z') {
			return false
		}
	}
	return true
}

// headingIndex stores all headings of a dictionary compactly: strings live
// in one shared buffer and each heading costs 12 bytes plus its text. With
// 7.6 million headings in the full X3 collection this matters.
type headingIndex struct {
	strings string          // concatenated heading texts
	records []headingRecord // in file order
	sorted  []int32         // record indices ordered by FoldKey(Text)
	refs    []uint32        // distinct article references, ascending
}

type headingRecord struct {
	offset  uint32 // start of Text in strings; DSL follows Text if present
	textLen uint16
	dslLen  uint16 // 0 when DSL equals Text
	ref     uint32
}

type indexBuilder struct {
	buf     strings.Builder
	records []headingRecord
}

func (b *indexBuilder) add(text, dsl string, ref uint32) {
	rec := headingRecord{
		offset:  uint32(b.buf.Len()),
		textLen: uint16(min(len(text), 0xFFFF)),
		ref:     ref,
	}
	b.buf.WriteString(text[:rec.textLen])
	if dsl != text {
		rec.dslLen = uint16(min(len(dsl), 0xFFFF))
		b.buf.WriteString(dsl[:rec.dslLen])
	}
	b.records = append(b.records, rec)
}

func (b *indexBuilder) build() *headingIndex {
	idx := &headingIndex{strings: b.buf.String(), records: b.records}

	keys := make([]string, len(idx.records))
	idx.sorted = make([]int32, len(idx.records))
	refs := make([]uint32, len(idx.records))
	for i, rec := range idx.records {
		keys[i] = FoldKey(idx.text(i))
		idx.sorted[i] = int32(i)
		refs[i] = rec.ref
	}
	// Ties keep file order so that results are deterministic.
	slices.SortFunc(idx.sorted, func(a, b int32) int {
		return cmp.Or(strings.Compare(keys[a], keys[b]), cmp.Compare(a, b))
	})
	slices.Sort(refs)
	idx.refs = slices.Compact(refs)
	return idx
}

func (idx *headingIndex) len() int { return len(idx.records) }

func (idx *headingIndex) text(i int) string {
	rec := idx.records[i]
	return idx.strings[rec.offset : rec.offset+uint32(rec.textLen)]
}

func (idx *headingIndex) dsl(i int) string {
	rec := idx.records[i]
	if rec.dslLen == 0 {
		return idx.text(i)
	}
	start := rec.offset + uint32(rec.textLen)
	return idx.strings[start : start+uint32(rec.dslLen)]
}

// lowerBound returns the first position in idx.sorted whose key is >= key.
func (idx *headingIndex) lowerBound(key string) int {
	pos, _ := slices.BinarySearchFunc(idx.sorted, key, func(i int32, key string) int {
		return strings.Compare(FoldKey(idx.text(int(i))), key)
	})
	return pos
}

// articleEnd returns the end offset of the article starting at ref, i.e.
// the start of the next article or limit for the last one.
func (idx *headingIndex) articleEnd(ref, limit uint32) uint32 {
	pos, found := slices.BinarySearch(idx.refs, ref)
	if found {
		pos++
	}
	if pos < len(idx.refs) {
		return idx.refs[pos]
	}
	return limit
}
