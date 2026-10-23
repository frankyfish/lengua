package lengua

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/frankyfish/lengua/lsd"
)

const universal = "LingvoUniversal (En-Ru)"

// testLibrary opens the Lingvo installation named by LINGVO_DATA and
// skips the test if the variable is not set.
func testLibrary(t *testing.T) *Library {
	t.Helper()
	dir := os.Getenv("LINGVO_DATA")
	if dir == "" {
		t.Skip("LINGVO_DATA is not set")
	}
	lib, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { lib.Close() })
	return lib
}

func TestOpenEmptyDir(t *testing.T) {
	if _, err := Open(t.TempDir()); !errors.Is(err, ErrNoDictionaries) {
		t.Errorf("Open(empty) error = %v, want ErrNoDictionaries", err)
	}
}

func TestCatalog(t *testing.T) {
	lib := testLibrary(t)
	infos := lib.Dictionaries()
	if len(infos) == 0 {
		t.Fatal("no dictionaries")
	}
	for _, info := range infos {
		if info.Name == "Abbrev" {
			t.Error("abbreviation list listed as a dictionary")
		}
	}
	info, err := lib.Info("lingvouniversalenru") // by file stem
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != universal || info.SoundPath == "" || info.AbbrevPath == "" || info.ID == "" {
		t.Errorf("incomplete info: %+v", info)
	}
	if _, err := lib.Info("nope"); !errors.Is(err, ErrUnknownDictionary) {
		t.Errorf("Info(unknown) error = %v", err)
	}
	for _, d := range lib.Find(lsd.German, lsd.Russian) {
		if d.Source.Primary() != lsd.German || d.Target != lsd.Russian {
			t.Errorf("Find(German, Russian) returned %s", d.Name)
		}
	}
	if len(lib.Find(lsd.German, 0)) == 0 {
		t.Error("Find(German) found nothing; GermanNewSpelling must match German")
	}
}

func TestLookup(t *testing.T) {
	lib := testLibrary(t)
	ctx := context.Background()
	results, err := lib.Lookup(ctx, "serene", LookupOptions{Source: lsd.English, Target: lsd.Russian})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	var found bool
	for _, r := range results {
		if r.Dictionary.Name == universal {
			found = true
			if !strings.Contains(r.Document().Text(), "безмятежный") {
				t.Errorf("unexpected article:\n%s", r.Article)
			}
		}
	}
	if !found {
		t.Errorf("no result from %s", universal)
	}

	words, err := lib.Suggest(ctx, "seren", 5, LookupOptions{Dictionaries: []string{universal}})
	if err != nil || len(words) == 0 || len(words) > 5 {
		t.Errorf("Suggest = %v, %v", words, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := lib.Lookup(cancelled, "serene", LookupOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("Lookup with cancelled context: err = %v", err)
	}
}

func TestMediaAndAbbreviation(t *testing.T) {
	lib := testLibrary(t)
	wav, err := lib.Media(universal, "serene.wav")
	if err != nil {
		t.Fatalf("Media: %v", err)
	}
	if !bytes.HasPrefix(wav, []byte("RIFF")) || len(wav) <= 44 {
		t.Errorf("Media returned %d bytes that don't look like WAV", len(wav))
	}
	if _, err := lib.Media(universal, "no-such-file.wav"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing media: err = %v, want ErrNotFound", err)
	}

	text, err := lib.Abbreviation(universal, "прил.")
	if err != nil || !strings.Contains(text, "прилагательное") {
		t.Errorf("Abbreviation = %q, %v", text, err)
	}
	if _, err := lib.Abbreviation(universal, "xyz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown abbreviation: err = %v", err)
	}
}

func TestClosed(t *testing.T) {
	lib := testLibrary(t)
	lib.Close()
	if _, err := lib.Dictionary(universal); !errors.Is(err, ErrClosed) {
		t.Errorf("Dictionary after Close: err = %v, want ErrClosed", err)
	}
}
