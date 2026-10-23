package lsd

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

// ErrMediaNotFound is returned by [Dictionary.ReadMedia] for unknown names.
var ErrMediaNotFound = errors.New("lsd: media file not found")

// MediaFile describes a file embedded in a dictionary, typically a picture
// referenced from articles as [s]name.jpg[/s].
type MediaFile struct {
	Name string // file name as referenced from articles
	Size int    // uncompressed size in bytes

	offset     uint32 // absolute file offset of the zlib stream
	streamSize uint32
}

// maxMediaEntries bounds the media index to protect against corrupt files.
const maxMediaEntries = 1 << 20

// readMediaIndex reads the media ("overlay") index that follows the
// heading pages. Each record is:
//
//	u8 name length, UTF-16LE name, u32 offset, u32 unknown,
//	u32 uncompressed size, u32 compressed size
//
// All integers are little-endian. Offsets are absolute in X3 files.
func readMediaIndex(f *os.File, start uint32, size int64) ([]MediaFile, error) {
	if int64(start)+4 > size {
		return nil, nil
	}
	r := bufio.NewReader(io.NewSectionReader(f, int64(start), size-int64(start)))
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("lsd: reading media index: %w", err)
	}
	if count > maxMediaEntries {
		return nil, fmt.Errorf("lsd: corrupt media index (%d entries)", count)
	}
	var files []MediaFile
	for range count {
		nameLen, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("lsd: reading media index: %w", err)
		}
		name := make([]uint16, nameLen)
		var rec struct {
			Offset, Unknown, Size, StreamSize uint32
		}
		if err := binary.Read(r, binary.LittleEndian, name); err != nil {
			return nil, fmt.Errorf("lsd: reading media index: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &rec); err != nil {
			return nil, fmt.Errorf("lsd: reading media index: %w", err)
		}
		if rec.Size == 0 {
			continue // placeholder entries without data
		}
		if int64(rec.Offset)+int64(rec.StreamSize) > size {
			return nil, fmt.Errorf("lsd: corrupt media index: %q out of range", string(utf16.Decode(name)))
		}
		files = append(files, MediaFile{
			Name:       string(utf16.Decode(name)),
			Size:       int(rec.Size),
			offset:     rec.Offset,
			streamSize: rec.StreamSize,
		})
	}
	return files, nil
}

// Media lists the files embedded in the dictionary. Most dictionaries have
// none.
func (d *Dictionary) Media() []MediaFile { return d.media }

// ReadMedia returns the content of the embedded file with the given name.
// The name is matched case-insensitively, as article references don't
// always use the exact case.
func (d *Dictionary) ReadMedia(name string) ([]byte, error) {
	for _, m := range d.media {
		if strings.EqualFold(m.Name, name) {
			return d.readMedia(m)
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrMediaNotFound, name)
}

func (d *Dictionary) readMedia(m MediaFile) ([]byte, error) {
	compressed, err := d.readBlock(m.offset, m.offset+m.streamSize)
	if err != nil {
		return nil, err
	}
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("lsd: decompressing %q: %w", m.Name, err)
	}
	defer zr.Close()
	out := make([]byte, 0, m.Size)
	buf := bytes.NewBuffer(out)
	if _, err := io.Copy(buf, io.LimitReader(zr, int64(m.Size))); err != nil {
		return nil, fmt.Errorf("lsd: decompressing %q: %w", m.Name, err)
	}
	return buf.Bytes(), nil
}
