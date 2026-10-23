package lsa

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// indexBuilder assembles synthetic LSA index bytes.
type indexBuilder struct{ bytes.Buffer }

func (b *indexBuilder) str(s string) {
	for _, r := range s {
		b.WriteByte(byte(r))
		b.WriteByte(byte(r >> 8))
	}
	b.WriteByte(0xFF)
}

func (b *indexBuilder) u32(v uint32) { binary.Write(b, binary.LittleEndian, v) }

func TestReadIndex(t *testing.T) {
	var b indexBuilder
	b.str(magic)
	b.u32(3)
	b.str("first.wav ") // the first record has no offset and marker
	b.u32(100)
	b.str("group") // a group header
	b.u32(100)
	b.WriteByte(0)
	b.str("second.wav")
	b.u32(100)
	b.WriteByte(0xFF)
	b.u32(50)

	entries, err := readIndex(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatalf("readIndex: %v", err)
	}
	want := []Entry{
		{Name: "first.wav", Samples: 100, offset: 0},
		{Name: "second.wav", Samples: 50, offset: 100},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, entries[i], want[i])
		}
	}
}

func TestReadIndexErrors(t *testing.T) {
	if _, err := readIndex(bytes.NewReader([]byte("RIFF...."))); !errors.Is(err, ErrNotLSA) {
		t.Errorf("wrong magic: err = %v, want ErrNotLSA", err)
	}
	var b indexBuilder
	b.str(magic)
	b.u32(2)
	b.str("a")
	b.u32(1)
	b.str("b")
	b.u32(1)
	b.WriteByte(0x42)
	if _, err := readIndex(bytes.NewReader(b.Bytes())); err == nil {
		t.Error("bad marker accepted")
	}
	if _, err := readIndex(bytes.NewReader(b.Bytes()[:10])); err == nil {
		t.Error("truncated index accepted")
	}
}

func TestWriteWAV(t *testing.T) {
	var buf bytes.Buffer
	samples := []int16{1, -1, 300}
	if err := WriteWAV(&buf, samples, 22050, 1); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if len(b) != 44+2*len(samples) {
		t.Fatalf("size = %d, want %d", len(b), 44+2*len(samples))
	}
	le := binary.LittleEndian
	checks := []struct {
		name      string
		got, want uint32
	}{
		{"chunk size", le.Uint32(b[4:]), uint32(len(b) - 8)},
		{"format", uint32(le.Uint16(b[20:])), 1},
		{"channels", uint32(le.Uint16(b[22:])), 1},
		{"sample rate", le.Uint32(b[24:]), 22050},
		{"byte rate", le.Uint32(b[28:]), 44100},
		{"data size", le.Uint32(b[40:]), 6},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if string(b[0:4]) != "RIFF" || string(b[8:16]) != "WAVEfmt " || string(b[36:40]) != "data" {
		t.Error("bad chunk identifiers")
	}
	if err := WriteWAV(io.Discard, nil, 0, 1); err == nil {
		t.Error("zero sample rate accepted")
	}
}

func TestSeekBase(t *testing.T) {
	pages := []oggPage{{offset: 100, granule: 1000}, {offset: 200, granule: 2000}}
	for pos, want := range map[int64]int64{0: 0, 999: 0, 1000: 100, 1999: 100, 5000: 200} {
		if got := seekBase(pages, pos); got != want {
			t.Errorf("seekBase(%d) = %d, want %d", pos, got, want)
		}
	}
}

func TestRebasedReader(t *testing.T) {
	src := io.NewSectionReader(bytes.NewReader([]byte("0123456789")), 0, 10)
	r := &rebasedReader{src: src, base: 4}
	buf := make([]byte, 3)
	if n, err := r.Read(buf); n != 3 || err != nil || string(buf) != "456" {
		t.Errorf("Read = %d, %v, %q; want 3, nil, \"456\"", n, err, buf)
	}
	if pos, _ := r.Seek(0, io.SeekEnd); pos != 6 {
		t.Errorf("Seek(0, End) = %d, want 6", pos)
	}
	if pos, _ := r.Seek(-2, io.SeekCurrent); pos != 4 {
		t.Errorf("Seek(-2, Current) = %d, want 4", pos)
	}
	if n, err := r.Read(buf); n != 2 || err != nil || string(buf[:n]) != "89" {
		t.Errorf("Read at end = %d, %v, %q", n, err, buf[:n])
	}
	if _, err := r.Seek(-1, io.SeekStart); err == nil {
		t.Error("negative position accepted")
	}
}

func TestArchive(t *testing.T) {
	root := os.Getenv("LINGVO_DATA")
	if root == "" {
		t.Skip("LINGVO_DATA is not set")
	}
	matches, _ := filepath.Glob(filepath.Join(root, "Data", "*", "Sound", "sounden.lsa"))
	if len(matches) == 0 {
		t.Skip("sounden.lsa not found")
	}
	a, err := Open(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	e, ok := a.Lookup("SERENE.WAV")
	if !ok {
		t.Fatal("serene.wav not found")
	}
	pcm, err := a.PCM(e.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != e.Samples*a.Channels() {
		t.Errorf("got %d samples, want %d", len(pcm), e.Samples*a.Channels())
	}
	// Decode the last clip too, which exercises seeking deep into the stream.
	last := a.Entries()[a.Len()-1]
	if _, err := a.PCM(last.Name); err != nil {
		t.Errorf("PCM(%q): %v", last.Name, err)
	}
	if _, err := a.PCM("no such clip"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing clip: err = %v, want ErrNotFound", err)
	}
}
