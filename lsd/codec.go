package lsd

import (
	"fmt"
	"unicode/utf16"
)

// articleScheme selects how article symbols are interpreted. ABBYY used two
// slightly different LZ-style schemes in X3 files.
type articleScheme int

const (
	// schemeSystem is used by version 0x131001 ("system" dictionaries).
	// Symbols >= 0x80 are characters offset by 0x80; smaller values are
	// back-references into the shared prefix or into the article itself.
	schemeSystem articleScheme = iota
	// schemeUser is used by version 0x132001 (dictionaries compiled by the
	// user-facing DSL compiler). Symbols < 0x10000 are characters; larger
	// values are back-references.
	schemeUser
)

// codec holds the compression model stored at Header.EncoderOffset. It is
// immutable after construction and therefore safe for concurrent use.
type codec struct {
	scheme         articleScheme
	prefix         []uint16 // shared dictionary of frequent substrings
	articleSymbols []uint32
	headingSymbols []uint32
	articles       *huffmanTable
	headings       *huffmanTable
	prefixLengths  *huffmanTable
	postfixLengths *huffmanTable
	refBits1       uint // bit width of references to child pages (inner B-tree nodes)
	refBits2       uint // bit width of references to articles
}

func readCodec(r *bitReader, scheme articleScheme) (*codec, error) {
	c := &codec{scheme: scheme}
	c.prefix = r.utf16be(int(r.bits(32)))
	c.articleSymbols = readSymbols(r)
	c.headingSymbols = readSymbols(r)
	var err error
	for _, t := range []**huffmanTable{&c.articles, &c.headings, &c.prefixLengths, &c.postfixLengths} {
		if *t, err = readHuffmanTable(r); err != nil {
			return nil, fmt.Errorf("lsd: reading compression tables: %w", err)
		}
	}
	c.refBits1 = bitLength(r.bits(32))
	c.refBits2 = bitLength(r.bits(32))
	if r.err() != nil {
		return nil, fmt.Errorf("lsd: reading compression tables: %w", r.err())
	}
	return c, nil
}

func readSymbols(r *bitReader) []uint32 {
	n := r.bits(32)
	width := uint(r.bits(8))
	if r.err() != nil || width > 32 || uint64(n)*uint64(width) > uint64(len(r.data))*8 {
		r.e = errTruncated
		return nil
	}
	syms := make([]uint32, n)
	for i := range syms {
		syms[i] = r.bits(width)
	}
	return syms
}

// symbol decodes one Huffman code and maps it through the symbol table.
func symbol(r *bitReader, t *huffmanTable, table []uint32) (uint32, error) {
	idx, err := t.decode(r)
	if err != nil {
		return 0, err
	}
	if idx >= len(table) {
		return 0, errBadCode
	}
	return table[idx], nil
}

func (c *codec) decodePrefixLen(r *bitReader) (int, error)  { return c.prefixLengths.decode(r) }
func (c *codec) decodePostfixLen(r *bitReader) (int, error) { return c.postfixLengths.decode(r) }

// decodeHeading decodes n UTF-16 code units of heading text.
func (c *codec) decodeHeading(r *bitReader, n int) ([]uint16, error) {
	out := make([]uint16, 0, n)
	for range n {
		sym, err := symbol(r, c.headings, c.headingSymbols)
		if err != nil {
			return nil, err
		}
		out = append(out, uint16(sym))
	}
	return out, nil
}

// readReference reads an article offset (or a page number) whose width is
// given by bits. The two-bit tag 3 introduces an explicit 32-bit value.
func readReference(r *bitReader, bits uint) uint32 {
	tag := r.bits(2)
	if tag == 3 {
		return r.bits(32)
	}
	if bits < 2 {
		return tag
	}
	return tag<<(bits-2) | r.bits(bits-2)
}

// decodeArticle decodes a length-prefixed article body at the reader's
// current position and returns it as UTF-8.
func (c *codec) decodeArticle(r *bitReader) (string, error) {
	size := r.bits(16)
	if size == 0xFFFF {
		size = r.bits(32)
	}
	if r.err() != nil {
		return "", r.err()
	}
	// Back-references make the output much larger than the input, so the
	// size can't be validated upfront. Instead, cap the preallocation and
	// the number of steps: every valid step consumes input bits or produces
	// output, so more steps than size + input bits means corrupt data.
	out := make([]uint16, 0, min(size, 1<<16))
	maxSteps := uint64(size) + uint64(len(r.data))*8
	prefixBits := bitLength(uint32(len(c.prefix)))
	sizeBits := bitLength(size)
	for step := uint64(0); uint32(len(out)) < size; step++ {
		if step > maxSteps {
			return "", errTruncated
		}
		sym, err := symbol(r, c.articles, c.articleSymbols)
		if err != nil {
			return "", err
		}
		switch ref := c.classify(sym); ref.kind {
		case symChar:
			out = append(out, ref.char)
		case symPrefixRef:
			start := int(r.bits(prefixBits))
			out = appendSlice(out, c.prefix, start, ref.length)
		case symSelfRef:
			start := int(r.bits(sizeBits))
			out = appendSlice(out, out, start, ref.length)
		}
		if r.err() != nil {
			return "", r.err()
		}
	}
	return string(utf16.Decode(out)), nil
}

type symKind int

const (
	symChar      symKind = iota // a literal UTF-16 code unit
	symPrefixRef                // copy from the shared prefix
	symSelfRef                  // copy from earlier in the same article
)

type decodedSymbol struct {
	kind   symKind
	char   uint16
	length int
}

func (c *codec) classify(sym uint32) decodedSymbol {
	if c.scheme == schemeSystem {
		switch {
		case sym >= 0x80:
			return decodedSymbol{kind: symChar, char: uint16(sym - 0x80)}
		case sym <= 0x3F:
			return decodedSymbol{kind: symPrefixRef, length: int(sym) + 3}
		default:
			return decodedSymbol{kind: symSelfRef, length: int(sym) - 0x3D}
		}
	}
	switch {
	case sym < 0x10000:
		return decodedSymbol{kind: symChar, char: uint16(sym)}
	case sym >= 0x10040:
		return decodedSymbol{kind: symSelfRef, length: int(sym) - 0x1003D}
	default:
		return decodedSymbol{kind: symPrefixRef, length: int(sym) - 0xFFFD}
	}
}

// appendSlice appends src[start:start+n] to dst, clamping to the bounds of
// src like std::u16string::substr does in the reference implementation.
func appendSlice(dst, src []uint16, start, n int) []uint16 {
	if start >= len(src) {
		return dst
	}
	end := min(start+n, len(src))
	return append(dst, src[start:end]...)
}
