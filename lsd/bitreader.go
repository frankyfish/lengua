package lsd

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

// errTruncated is reported when a decoder tries to read past the end of a
// data block. It almost always means the file is corrupt or unsupported.
var errTruncated = errors.New("lsd: unexpected end of data")

// bitReader reads an MSB-first bit stream from an in-memory block.
//
// LSD files mix two kinds of fields: byte-aligned little-endian integers in
// the file header and bit-packed big-endian fields everywhere else. The
// reader supports both. Errors are sticky: after the first out-of-bounds
// read every subsequent read returns zero and err() reports the failure,
// which keeps the decoding loops simple and panic-free.
type bitReader struct {
	data   []byte
	pos    int  // index of the current byte
	bitPos uint // bits already consumed from data[pos], 0..7
	e      error
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data}
}

func (r *bitReader) err() error { return r.e }

// seek moves to an absolute byte offset and resets the bit position.
func (r *bitReader) seek(pos int) {
	r.pos, r.bitPos = pos, 0
	if pos < 0 || pos > len(r.data) {
		r.e = errTruncated
	}
}

// align skips the remaining bits of a partially consumed byte.
func (r *bitReader) align() {
	if r.bitPos != 0 {
		r.pos++
		r.bitPos = 0
	}
}

// bit reads a single bit.
func (r *bitReader) bit() uint32 {
	if r.e != nil {
		return 0
	}
	if r.pos >= len(r.data) {
		r.e = errTruncated
		return 0
	}
	b := uint32(r.data[r.pos]>>(7-r.bitPos)) & 1
	r.bitPos++
	if r.bitPos == 8 {
		r.bitPos = 0
		r.pos++
	}
	return b
}

// bits reads n (0..32) bits as a big-endian unsigned integer.
func (r *bitReader) bits(n uint) uint32 {
	var v uint32
	for n > 0 && r.e == nil {
		if r.pos >= len(r.data) {
			r.e = errTruncated
			return 0
		}
		avail := 8 - r.bitPos
		take := min(avail, n)
		chunk := uint32(r.data[r.pos]>>(avail-take)) & (1<<take - 1)
		v = v<<take | chunk
		n -= take
		r.bitPos += take
		if r.bitPos == 8 {
			r.bitPos = 0
			r.pos++
		}
	}
	return v
}

// bytes returns the next n bytes of a byte-aligned stream without copying.
func (r *bitReader) bytes(n int) []byte {
	r.align()
	if r.e != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.data) {
		r.e = errTruncated
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *bitReader) u8() uint8 {
	if b := r.bytes(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *bitReader) u16le() uint16 {
	if b := r.bytes(2); b != nil {
		return binary.LittleEndian.Uint16(b)
	}
	return 0
}

func (r *bitReader) u32le() uint32 {
	if b := r.bytes(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

// utf16le reads n little-endian UTF-16 code units (byte aligned).
func (r *bitReader) utf16le(n int) string {
	b := r.bytes(2 * n)
	if b == nil {
		return ""
	}
	units := make([]uint16, n)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(units))
}

// utf16be reads n big-endian UTF-16 code units from the bit stream.
func (r *bitReader) utf16be(n int) []uint16 {
	if n < 0 || 2*n > len(r.data) {
		r.e = errTruncated
		return nil
	}
	units := make([]uint16, n)
	for i := range units {
		units[i] = uint16(r.bits(16))
	}
	return units
}

// bitLength returns the number of bits needed to represent n (at least 1).
// LSD uses it to size back-reference and index fields.
func bitLength(n uint32) uint {
	l := uint(1)
	for n >>= 1; n != 0; n >>= 1 {
		l++
	}
	return l
}
