package lsa

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

// Fast seeking
//
// oggvorbis.Reader.SetPosition finds the target page by scanning all page
// headers from the start of the stream, which makes random access to the
// end of a large archive slow. To avoid that, the archive indexes the page
// granule positions once, and each decoder reads the stream through a
// rebasedReader whose origin is moved to the last page that ends before
// the target sample. SetPosition then only scans a page or two.
//
// The decoder's Vorbis headers are parsed from the real stream start when
// the decoder is created; SetPosition never needs them again.

// oggPage is an entry of the page index.
type oggPage struct {
	offset  int64 // byte offset of the page in the stream
	granule int64 // last sample position completed on the page
}

// scanPages indexes the pages of an Ogg stream. Pages on which no packet
// ends (granule -1) and header pages (granule 0) are skipped, so granules
// are strictly increasing.
func scanPages(r io.ReaderAt, size int64) ([]oggPage, error) {
	br := bufio.NewReaderSize(io.NewSectionReader(r, 0, size), 1<<20)
	var pages []oggPage
	var hdr [27]byte
	var segments [255]byte
	for off := int64(0); off < size; {
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			return nil, fmt.Errorf("lsa: indexing ogg pages at offset %d: %w", off, err)
		}
		if string(hdr[:4]) != "OggS" {
			return nil, fmt.Errorf("lsa: indexing ogg pages: no page at offset %d", off)
		}
		table := segments[:hdr[26]]
		if _, err := io.ReadFull(br, table); err != nil {
			return nil, fmt.Errorf("lsa: indexing ogg pages at offset %d: %w", off, err)
		}
		body := 0
		for _, s := range table {
			body += int(s)
		}
		if _, err := br.Discard(body); err != nil {
			return nil, fmt.Errorf("lsa: indexing ogg pages at offset %d: %w", off, err)
		}
		granule := int64(binary.LittleEndian.Uint64(hdr[6:14]))
		if granule > 0 && (len(pages) == 0 || granule > pages[len(pages)-1].granule) {
			pages = append(pages, oggPage{offset: off, granule: granule})
		}
		off += int64(len(hdr) + len(table) + body)
	}
	return pages, nil
}

// seekBase returns the offset of the last indexed page whose granule is
// at most pos, or 0 if pos lies before the first such page.
func seekBase(pages []oggPage, pos int64) int64 {
	i := sort.Search(len(pages), func(i int) bool { return pages[i].granule > pos })
	if i == 0 {
		return 0
	}
	return pages[i-1].offset
}

// rebasedReader is an io.ReadSeeker over src whose origin can be moved to
// any offset; positions are relative to base.
type rebasedReader struct {
	src  *io.SectionReader
	base int64
	pos  int64
}

func (r *rebasedReader) Read(p []byte) (int, error) {
	n, err := r.src.ReadAt(p, r.base+r.pos)
	r.pos += int64(n)
	if n > 0 && errors.Is(err, io.EOF) {
		err = nil
	}
	return n, err
}

func (r *rebasedReader) Seek(offset int64, whence int) (int64, error) {
	var pos int64
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = r.pos + offset
	case io.SeekEnd:
		pos = r.src.Size() - r.base + offset
	default:
		return 0, errors.New("lsa: invalid whence")
	}
	if pos < 0 {
		return 0, errors.New("lsa: negative position")
	}
	r.pos = pos
	return pos, nil
}
