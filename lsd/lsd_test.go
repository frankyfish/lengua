package lsd

import (
	"testing"
	"unicode/utf16"
)

func TestBitReader(t *testing.T) {
	r := newBitReader([]byte{0b1010_1100, 0xFF, 0x34, 0x12})
	if got := r.bit(); got != 1 {
		t.Errorf("bit() = %d, want 1", got)
	}
	if got := r.bits(3); got != 0b010 {
		t.Errorf("bits(3) = %b, want 010", got)
	}
	if got := r.bits(8); got != 0b1100_1111 {
		t.Errorf("bits(8) across bytes = %b, want 11001111", got)
	}
	if got := r.u16le(); got != 0x1234 {
		t.Errorf("u16le() after align = %#x, want 0x1234", got)
	}
	if r.err() != nil {
		t.Fatalf("unexpected error: %v", r.err())
	}
	if got := r.bit(); got != 0 || r.err() != errTruncated {
		t.Errorf("read past end: got %d, err %v; want 0, errTruncated", got, r.err())
	}
	if r.u32le(); r.err() != errTruncated {
		t.Error("error is not sticky")
	}
}

func TestBitLength(t *testing.T) {
	for n, want := range map[uint32]uint{0: 1, 1: 1, 2: 2, 3: 2, 255: 8, 256: 9, 1<<32 - 1: 32} {
		if got := bitLength(n); got != want {
			t.Errorf("bitLength(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestHeadingTexts(t *testing.T) {
	u := func(s string) []uint16 { return utf16.Encode([]rune(s)) }
	tests := []struct {
		name      string
		heading   rawHeading
		text, dsl string
	}{
		{"plain", rawHeading{sorted: u("apple")}, "apple", "apple"},
		{
			"unsorted suffix grouped",
			rawHeading{sorted: u("some"), ext: []extChar{{4, 't'}, {5, 'h'}}},
			"some", "some{th}",
		},
		{
			"unsorted in the middle",
			rawHeading{sorted: u("ab"), ext: []extChar{{1, 'x'}}},
			"ab", "a{x}b",
		},
		{
			"escaped sorted char",
			rawHeading{sorted: u(`a\(b`)},
			"a(b", `a\(b`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, dsl := tt.heading.texts()
			if text != tt.text || dsl != tt.dsl {
				t.Errorf("texts() = %q, %q; want %q, %q", text, dsl, tt.text, tt.dsl)
			}
		})
	}
}

func TestStripDSLHeading(t *testing.T) {
	for in, want := range map[string]string{
		"plain":           "plain",
		"Some{thing}":     "Something",
		`a\{b\} \(x\)`:    "a{b} (x)",
		`back\\slash{es}`: `back\slashes`,
	} {
		if got := StripDSLHeading(in); got != want {
			t.Errorf("StripDSLHeading(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFoldKey(t *testing.T) {
	for in, want := range map[string]string{
		"apple":  "apple",
		"Apple":  "apple",
		"за́мок": "замок",
		"Ёлка":   "елка",
		"don’t":  "don't",
		"où":     "où",
	} {
		if got := FoldKey(in); got != want {
			t.Errorf("FoldKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLanguage(t *testing.T) {
	tests := []struct {
		in   string
		want Language
		ok   bool
	}{
		{"1033", English, true},
		{"en", English, true},
		{"RU", Russian, true},
		{"english", English, true},
		{"German", German, true},
		{"GermanNewSpelling", GermanNewSpelling, true},
		{"klingon", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseLanguage(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParseLanguage(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestLanguagePrimary(t *testing.T) {
	for in, want := range map[Language]Language{
		GermanNewSpelling:  German,
		PortugueseStandard: 1046,
		English:            English,
	} {
		if got := in.Primary(); got != want {
			t.Errorf("%v.Primary() = %v, want %v", in, got, want)
		}
	}
	if got := English.String(); got != "English" {
		t.Errorf("English.String() = %q", got)
	}
}

func TestAppendSlice(t *testing.T) {
	src := []uint16{'a', 'b', 'c'}
	tests := []struct {
		start, n int
		want     string
	}{
		{0, 2, "ab"},
		{1, 10, "bc"}, // clamped to the end of src
		{3, 1, ""},    // start out of range
	}
	for _, tt := range tests {
		got := string(utf16.Decode(appendSlice(nil, src, tt.start, tt.n)))
		if got != tt.want {
			t.Errorf("appendSlice(%d, %d) = %q, want %q", tt.start, tt.n, got, tt.want)
		}
	}
}
