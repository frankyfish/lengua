package lsd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
)

// Errors returned when a file can't be opened as a dictionary. Use
// errors.Is to test for them.
var (
	ErrNotLSD             = errors.New("lsd: not an ABBYY Lingvo dictionary")
	ErrUnsupportedVersion = errors.New("lsd: unsupported dictionary version")
)

// Format versions of Lingvo X3 (v14) dictionaries.
const (
	versionX3System = 0x131001 // dictionaries shipped by ABBYY
	versionX3User   = 0x132001 // dictionaries compiled with the DSL compiler
)

const (
	pageSize        = 512 // size of a B-tree page with headings
	fixedHeaderSize = 52
	lsdMagic        = "LingVo"
)

// header is the parsed file header. Offsets are absolute file positions.
//
//	0x00 magic "LingVo\0\0"      0x24 pagesOffset
//	0x08 version                 0x28 unknown
//	0x0C unknown                 0x2C lastPage (u16), unknown (u16)
//	0x10 checksum                0x30 source language (u16)
//	0x14 entryCount              0x32 target language (u16)
//	0x18 annotationOffset        0x34 variable part: name, first/last
//	0x1C encoderOffset                heading, alphabet, icon, pagesEnd,
//	0x20 articlesOffset               overlayOffset
type header struct {
	version          uint32
	checksum         uint32
	entryCount       uint32
	annotationOffset uint32
	encoderOffset    uint32
	articlesOffset   uint32
	pagesOffset      uint32
	source, target   Language

	name         string
	firstHeading string
	lastHeading  string
	alphabet     string
	icon         []byte
	pagesEnd     uint32 // end of the heading pages; start of the media index
}

func readHeader(f io.ReaderAt, size int64) (*header, error) {
	fixed := make([]byte, fixedHeaderSize)
	if _, err := f.ReadAt(fixed, 0); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrNotLSD
		}
		return nil, err
	}
	if string(fixed[:len(lsdMagic)]) != lsdMagic {
		return nil, ErrNotLSD
	}
	le := binary.LittleEndian
	h := &header{
		version:          le.Uint32(fixed[0x08:]),
		checksum:         le.Uint32(fixed[0x10:]),
		entryCount:       le.Uint32(fixed[0x14:]),
		annotationOffset: le.Uint32(fixed[0x18:]),
		encoderOffset:    le.Uint32(fixed[0x1C:]),
		articlesOffset:   le.Uint32(fixed[0x20:]),
		pagesOffset:      le.Uint32(fixed[0x24:]),
		source:           Language(le.Uint16(fixed[0x30:])),
		target:           Language(le.Uint16(fixed[0x32:])),
	}
	if h.version != versionX3System && h.version != versionX3User {
		return nil, fmt.Errorf("%w: 0x%X (only Lingvo X3 files are supported)", ErrUnsupportedVersion, h.version)
	}
	if !(fixedHeaderSize <= h.annotationOffset && h.annotationOffset <= h.encoderOffset &&
		h.encoderOffset <= h.articlesOffset && h.articlesOffset <= h.pagesOffset &&
		int64(h.pagesOffset) <= size) {
		return nil, fmt.Errorf("lsd: corrupt header: inconsistent section offsets")
	}

	// The variable part lies between the fixed header and the annotation.
	block := make([]byte, h.annotationOffset)
	if _, err := f.ReadAt(block, 0); err != nil {
		return nil, fmt.Errorf("lsd: reading header: %w", err)
	}
	r := newBitReader(block)
	r.seek(fixedHeaderSize)
	h.name = r.utf16le(int(r.u8()))
	h.firstHeading = r.utf16le(int(r.u8()))
	h.lastHeading = r.utf16le(int(r.u8()))
	h.alphabet = r.utf16le(int(r.u32le()))
	h.icon = slices.Clone(r.bytes(int(r.u16le())))
	h.pagesEnd = r.u32le()
	_ = r.u32le() // overlay data offset; X3 media entries use absolute offsets instead
	if r.err() != nil {
		return nil, fmt.Errorf("lsd: corrupt header: %w", r.err())
	}
	if h.pagesEnd < h.pagesOffset || int64(h.pagesEnd) > size || (h.pagesEnd-h.pagesOffset)%pageSize != 0 {
		return nil, fmt.Errorf("lsd: corrupt header: invalid page area")
	}
	return h, nil
}

func (h *header) pageCount() int {
	return int(h.pagesEnd-h.pagesOffset) / pageSize
}
