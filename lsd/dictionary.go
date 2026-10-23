package lsd

import (
	"errors"
	"fmt"
	"iter"
	"os"
)

// ErrForeignHeading is returned when a Heading is passed to a Dictionary
// other than the one that produced it.
var ErrForeignHeading = errors.New("lsd: heading belongs to another dictionary")

// Info describes a dictionary. It is available without decoding articles.
type Info struct {
	Name         string   // e.g. "LingvoUniversal (En-Ru)"
	Source       Language // language of the headings
	Target       Language // language of the articles
	Version      uint32   // format version, 0x131001 or 0x132001
	HeadingCount int      // number of headings, including synonyms
	FirstHeading string   // first heading in ABBYY sort order
	LastHeading  string   // last heading in ABBYY sort order
	Alphabet     string   // characters used as index letters
}

// Dictionary is an opened LSD dictionary file.
//
// Opening a dictionary decodes all headings into a compact in-memory index;
// articles are decoded on demand. A Dictionary is safe for concurrent use
// by multiple goroutines.
type Dictionary struct {
	path   string
	file   *os.File
	header *header
	codec  *codec
	index  *headingIndex
	media  []MediaFile
}

// ReadInfo reads only the header of the dictionary at path. It is much
// cheaper than [Open] and is meant for building catalogs.
func ReadInfo(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Info{}, err
	}
	h, err := readHeader(f, st.Size())
	if err != nil {
		return Info{}, fmt.Errorf("%s: %w", path, err)
	}
	return h.info(), nil
}

// Open opens the dictionary file at path (usually *.lsd) and indexes its
// headings. The caller must call [Dictionary.Close] when done.
//
// Only Lingvo X3 files are supported; other versions yield an error
// wrapping [ErrUnsupportedVersion].
func Open(path string) (*Dictionary, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	d, err := open(f, path)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func open(f *os.File, path string) (*Dictionary, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	h, err := readHeader(f, st.Size())
	if err != nil {
		return nil, err
	}
	d := &Dictionary{path: path, file: f, header: h}

	scheme := schemeSystem
	if h.version == versionX3User {
		scheme = schemeUser
	}
	block, err := d.readBlock(h.encoderOffset, h.articlesOffset)
	if err != nil {
		return nil, err
	}
	if d.codec, err = readCodec(newBitReader(block), scheme); err != nil {
		return nil, err
	}
	if d.index, err = d.readHeadings(); err != nil {
		return nil, err
	}
	if d.media, err = readMediaIndex(f, h.pagesEnd, st.Size()); err != nil {
		return nil, err
	}
	return d, nil
}

// readHeadings decodes the headings from all leaf pages of the B-tree.
// Inner pages only exist to speed up searching and are skipped.
func (d *Dictionary) readHeadings() (*headingIndex, error) {
	pages, err := d.readBlock(d.header.pagesOffset, d.header.pagesEnd)
	if err != nil {
		return nil, err
	}
	var b indexBuilder
	b.records = make([]headingRecord, 0, d.header.entryCount)
	for p := range d.header.pageCount() {
		r := newBitReader(pages[p*pageSize : (p+1)*pageSize])
		isLeaf := r.bit() == 1
		r.bits(16 * 4) // page number, previous, parent, next
		count := int(r.bits(16))
		r.align()
		if !isLeaf {
			continue
		}
		var known []uint16
		for range count {
			h, err := readHeading(r, d.codec, known)
			if err != nil {
				return nil, fmt.Errorf("lsd: decoding heading page %d: %w", p, err)
			}
			known = h.sorted
			text, dsl := h.texts()
			b.add(text, dsl, h.ref)
		}
	}
	if len(b.records) != int(d.header.entryCount) {
		return nil, fmt.Errorf("lsd: decoded %d headings, header declares %d", len(b.records), d.header.entryCount)
	}
	return b.build(), nil
}

// readBlock reads the file range [start, end).
func (d *Dictionary) readBlock(start, end uint32) ([]byte, error) {
	if end < start {
		return nil, fmt.Errorf("lsd: invalid block [%d, %d)", start, end)
	}
	buf := make([]byte, end-start)
	if _, err := d.file.ReadAt(buf, int64(start)); err != nil {
		return nil, fmt.Errorf("lsd: reading block at %d: %w", start, err)
	}
	return buf, nil
}

// Close releases the file handle. Headings remain readable, but articles,
// annotation and media can no longer be read.
func (d *Dictionary) Close() error {
	return d.file.Close()
}

// Path returns the file path the dictionary was opened from.
func (d *Dictionary) Path() string { return d.path }

// Info returns the dictionary metadata.
func (d *Dictionary) Info() Info { return d.header.info() }

func (h *header) info() Info {
	return Info{
		Name:         h.name,
		Source:       h.source,
		Target:       h.target,
		Version:      h.version,
		HeadingCount: int(h.entryCount),
		FirstHeading: h.firstHeading,
		LastHeading:  h.lastHeading,
		Alphabet:     h.alphabet,
	}
}

// Icon returns the dictionary icon as a BMP image, or nil if there is none.
func (d *Dictionary) Icon() []byte { return d.header.icon }

// Annotation returns the dictionary description (copyright, authors,
// editions) in DSL markup. It may be empty.
func (d *Dictionary) Annotation() (string, error) {
	if d.header.annotationOffset == d.header.encoderOffset {
		return "", nil
	}
	block, err := d.readBlock(d.header.annotationOffset, d.header.encoderOffset)
	if err != nil {
		return "", err
	}
	text, err := d.codec.decodeArticle(newBitReader(block))
	if err != nil {
		return "", fmt.Errorf("lsd: decoding annotation: %w", err)
	}
	return text, nil
}

// Len returns the number of headings.
func (d *Dictionary) Len() int { return d.index.len() }

// Heading returns the i-th heading in file order, 0 <= i < Len().
func (d *Dictionary) Heading(i int) Heading {
	return Heading{
		Text:  d.index.text(i),
		DSL:   d.index.dsl(i),
		owner: d,
		index: int32(i),
		ref:   d.index.records[i].ref,
	}
}

// Headings iterates over all headings in file order.
func (d *Dictionary) Headings() iter.Seq[Heading] {
	return func(yield func(Heading) bool) {
		for i := range d.Len() {
			if !yield(d.Heading(i)) {
				return
			}
		}
	}
}

// Lookup returns all headings whose searchable text equals word after
// normalization with [FoldKey]. Several headings may share one article.
func (d *Dictionary) Lookup(word string) []Heading {
	key := FoldKey(word)
	var out []Heading
	for pos := d.index.lowerBound(key); pos < len(d.index.sorted); pos++ {
		i := int(d.index.sorted[pos])
		if FoldKey(d.index.text(i)) != key {
			break
		}
		out = append(out, d.Heading(i))
	}
	return out
}

// Prefix returns up to limit headings whose normalized text starts with
// the normalized prefix, ordered alphabetically. A limit <= 0 means no limit.
func (d *Dictionary) Prefix(prefix string, limit int) []Heading {
	key := FoldKey(prefix)
	var out []Heading
	for pos := d.index.lowerBound(key); pos < len(d.index.sorted); pos++ {
		if limit > 0 && len(out) >= limit {
			break
		}
		i := int(d.index.sorted[pos])
		if !hasPrefix(FoldKey(d.index.text(i)), key) {
			break
		}
		out = append(out, d.Heading(i))
	}
	return out
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Article decodes the article of heading h and returns it in DSL markup
// (see package dsl for parsing and rendering).
func (d *Dictionary) Article(h Heading) (string, error) {
	if h.owner != d {
		return "", ErrForeignHeading
	}
	limit := d.header.pagesOffset - d.header.articlesOffset
	end := d.index.articleEnd(h.ref, limit)
	if h.ref >= limit || end > limit {
		return "", fmt.Errorf("lsd: article reference %d out of range", h.ref)
	}
	block, err := d.readBlock(d.header.articlesOffset+h.ref, d.header.articlesOffset+end)
	if err != nil {
		return "", err
	}
	text, err := d.codec.decodeArticle(newBitReader(block))
	if err != nil {
		return "", fmt.Errorf("lsd: decoding article of %q: %w", h.Text, err)
	}
	return text, nil
}

// Entry is an article together with all headings that point to it.
type Entry struct {
	Headings []Heading
}

// Entries iterates over all articles. Headings sharing an article are
// grouped into one Entry; entries appear in the file order of their first
// heading. Use [Dictionary.Article] with any of the entry's headings to
// read the article.
func (d *Dictionary) Entries() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		groups := make(map[uint32][]int32, d.Len())
		order := make([]uint32, 0, d.Len())
		for i, rec := range d.index.records {
			if _, seen := groups[rec.ref]; !seen {
				order = append(order, rec.ref)
			}
			groups[rec.ref] = append(groups[rec.ref], int32(i))
		}
		for _, ref := range order {
			idx := groups[ref]
			e := Entry{Headings: make([]Heading, len(idx))}
			for j, i := range idx {
				e.Headings[j] = d.Heading(int(i))
			}
			if !yield(e) {
				return
			}
		}
	}
}
