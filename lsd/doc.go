// Package lsd reads ABBYY Lingvo X3 compiled dictionaries (*.lsd).
//
// An LSD file contains a dictionary compiled from the ABBYY DSL source
// format: a header, Huffman code tables, LZ77-style compressed articles,
// a B-tree of headings and an optional block of embedded media files.
// See docs/FORMATS.md in the module for a byte-level description.
//
// Typical use:
//
//	d, err := lsd.Open("LingvoUniversalEnRu.lsd")
//	if err != nil { ... }
//	defer d.Close()
//	for _, h := range d.Lookup("serene") {
//		article, err := d.Article(h) // DSL markup, see package dsl
//		...
//	}
//
// Only the Lingvo X3 format versions 0x131001 (system dictionaries) and
// 0x132001 (user dictionaries) are supported.
//
// The decoding algorithms are ported from the MIT-licensed projects
// lsd2dsl (https://github.com/nongeneric/lsd2dsl, Copyright (c) nongeneric)
// and lsdreader (https://github.com/sv99/lsdreader, Copyright (c) sv99).
package lsd
