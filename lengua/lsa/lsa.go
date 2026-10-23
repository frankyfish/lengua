// Package lsa reads ABBYY Lingvo sound archives (*.lsa).
//
// An archive holds the pronunciations referenced from dictionary articles
// as [s]word.wav[/s]. Despite the .wav names, all clips are stored as one
// continuous Ogg Vorbis stream; the archive index maps each name to a
// range of samples. This package decodes a clip on request and returns
// it as 16-bit PCM or as a WAV file.
//
//	a, err := lsa.Open("SoundEn.lsa")
//	if err != nil { ... }
//	defer a.Close()
//	wav, err := a.WAV("serene.wav")
//
// The archive format is ported from lsd2dsl
// (https://github.com/nongeneric/lsd2dsl, MIT license).
package lsa

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/jfreymuth/oggvorbis"
)

var (
	// ErrNotLSA is returned when a file is not a Lingvo sound archive.
	ErrNotLSA = errors.New("lsa: not a Lingvo sound archive")
	// ErrNotFound is returned for names that are not in the archive.
	ErrNotFound = errors.New("lsa: sound not found")
)

const magic = "L9SA"

// Entry is a sound clip in the archive.
type Entry struct {
	Name    string // file name as referenced from articles, e.g. "serene.wav"
	Samples int    // clip length in samples per channel

	offset int64 // first sample in the Ogg stream
}

// Archive is an opened sound archive. It is safe for concurrent use.
type Archive struct {
	file     *os.File
	entries  []Entry
	byName   map[string]int // lower-cased name -> index in entries
	stream   *io.SectionReader
	rate     int
	channels int

	decoders sync.Pool // *decoder

	pagesOnce sync.Once
	pages     []oggPage
	pagesErr  error
}

// decoder is a reusable Vorbis decoder with its own view of the stream.
type decoder struct {
	view *rebasedReader
	ogg  *oggvorbis.Reader
}

// Open opens the sound archive at path and reads its index. The caller
// must call [Archive.Close] when done.
func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a, err := open(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

func open(f *os.File) (*Archive, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	counter := &countingReader{r: bufio.NewReader(f)}
	entries, err := readIndex(counter)
	if err != nil {
		return nil, err
	}
	a := &Archive{
		file:    f,
		entries: entries,
		byName:  make(map[string]int, len(entries)),
		stream:  io.NewSectionReader(f, counter.n, st.Size()-counter.n),
	}
	for i, e := range entries {
		key := strings.ToLower(e.Name)
		if _, dup := a.byName[key]; !dup {
			a.byName[key] = i
		}
	}

	// Decode the stream headers once to learn the format and to validate
	// the archive; the decoder is kept for reuse.
	dec, err := a.newDecoder()
	if err != nil {
		return nil, err
	}
	a.rate, a.channels = dec.ogg.SampleRate(), dec.ogg.Channels()
	a.decoders.Put(dec)
	return a, nil
}

// readIndex parses the archive header and the list of clips:
//
//	string magic "L9SA", u32 count, then count records of
//	string name, [u32 sample offset, u8 marker,] u32 sample count
//
// The offset and marker are absent in the first record (offset 0). A
// marker of 0 denotes a group header without audio and no sample count.
func readIndex(r io.ByteReader) ([]Entry, error) {
	if s, err := readString(r); err != nil || s != magic {
		return nil, ErrNotLSA
	}
	count, err := readU32(r)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, min(count, 1<<20))
	for i := range count {
		name, err := readString(r)
		if err != nil {
			return nil, err
		}
		var offset uint32
		if i > 0 {
			if offset, err = readU32(r); err != nil {
				return nil, err
			}
			marker, err := r.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("lsa: reading index: %w", err)
			}
			if marker == 0 {
				continue
			}
			if marker != 0xFF {
				return nil, fmt.Errorf("lsa: corrupt index: marker %#x at entry %d", marker, i)
			}
		}
		samples, err := readU32(r)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{
			Name:    strings.TrimSpace(name),
			Samples: int(samples),
			offset:  int64(offset),
		})
	}
	return entries, nil
}

// readString reads a UTF-16LE string terminated by a 0xFF byte in either
// half of a code unit.
func readString(r io.ByteReader) (string, error) {
	var sb strings.Builder
	for {
		lo, err := r.ReadByte()
		if err != nil {
			return "", fmt.Errorf("lsa: reading index: %w", err)
		}
		if lo == 0xFF {
			return sb.String(), nil
		}
		hi, err := r.ReadByte()
		if err != nil {
			return "", fmt.Errorf("lsa: reading index: %w", err)
		}
		if hi == 0xFF {
			return sb.String(), nil
		}
		sb.WriteRune(rune(lo) | rune(hi)<<8)
	}
}

func readU32(r io.ByteReader) (uint32, error) {
	var b [4]byte
	for i := range b {
		c, err := r.ReadByte()
		if err != nil {
			return 0, fmt.Errorf("lsa: reading index: %w", err)
		}
		b[i] = c
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

// countingReader tracks the number of bytes consumed through a buffered
// reader, i.e. the offset where the Ogg stream starts.
type countingReader struct {
	r *bufio.Reader
	n int64
}

func (c *countingReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}

// Close closes the archive file.
func (a *Archive) Close() error { return a.file.Close() }

// Entries returns all clips in archive order.
func (a *Archive) Entries() []Entry { return a.entries }

// Len returns the number of clips.
func (a *Archive) Len() int { return len(a.entries) }

// SampleRate returns the sample rate in Hz shared by all clips.
func (a *Archive) SampleRate() int { return a.rate }

// Channels returns the number of audio channels shared by all clips.
func (a *Archive) Channels() int { return a.channels }

// Lookup returns the clip with the given name, compared case-insensitively.
func (a *Archive) Lookup(name string) (Entry, bool) {
	i, ok := a.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Entry{}, false
	}
	return a.entries[i], true
}

// Has reports whether the archive contains a clip with the given name.
func (a *Archive) Has(name string) bool {
	_, ok := a.Lookup(name)
	return ok
}

// PCM decodes the named clip into interleaved signed 16-bit samples.
func (a *Archive) PCM(name string) ([]int16, error) {
	e, ok := a.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	return a.decode(e)
}

// WAV decodes the named clip and returns it as a 16-bit PCM WAV file.
func (a *Archive) WAV(name string) ([]byte, error) {
	pcm, err := a.PCM(name)
	if err != nil {
		return nil, err
	}
	var buf bytesWriter
	if err := WriteWAV(&buf, pcm, a.rate, a.channels); err != nil {
		return nil, err
	}
	return buf, nil
}

func (a *Archive) newDecoder() (*decoder, error) {
	view := &rebasedReader{src: a.stream}
	ogg, err := oggvorbis.NewReader(view)
	if err != nil {
		return nil, fmt.Errorf("lsa: opening ogg stream: %w", err)
	}
	return &decoder{view: view, ogg: ogg}, nil
}

// pageIndex returns the page index, building it on first use.
func (a *Archive) pageIndex() ([]oggPage, error) {
	a.pagesOnce.Do(func() {
		a.pages, a.pagesErr = scanPages(a.stream, a.stream.Size())
	})
	return a.pages, a.pagesErr
}

func (a *Archive) decode(e Entry) ([]int16, error) {
	pages, err := a.pageIndex()
	if err != nil {
		return nil, err
	}
	dec, _ := a.decoders.Get().(*decoder)
	if dec == nil {
		if dec, err = a.newDecoder(); err != nil {
			return nil, err
		}
	}
	dec.view.base = seekBase(pages, e.offset)
	pcm, err := readClip(dec.ogg, e, a.channels)
	if err != nil {
		return nil, err // the decoder state is unknown; don't reuse it
	}
	a.decoders.Put(dec)
	return pcm, nil
}

func readClip(dec *oggvorbis.Reader, e Entry, channels int) ([]int16, error) {
	if err := dec.SetPosition(e.offset); err != nil {
		return nil, fmt.Errorf("lsa: seeking to %q: %w", e.Name, err)
	}
	want := e.Samples * channels
	out := make([]int16, 0, want)
	buf := make([]float32, 8192*channels)
	for len(out) < want {
		n, err := dec.Read(buf[:min(len(buf), want-len(out))])
		for _, v := range buf[:n] {
			out = append(out, toInt16(v))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("lsa: decoding %q: %w", e.Name, err)
		}
	}
	if len(out) != want {
		return nil, fmt.Errorf("lsa: decoding %q: got %d samples, want %d", e.Name, len(out), want)
	}
	return out, nil
}

// toInt16 converts a float sample in [-1, 1] the way libvorbis does.
func toInt16(v float32) int16 {
	return int16(max(-32768, min(32767, math.Round(float64(v)*32768))))
}

// bytesWriter is a minimal growable io.Writer.
type bytesWriter []byte

func (b *bytesWriter) Write(p []byte) (int, error) {
	*b = append(*b, p...)
	return len(p), nil
}
