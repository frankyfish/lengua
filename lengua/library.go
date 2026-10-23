package lengua

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/frankyfish/lengua/lsa"
	"github.com/frankyfish/lengua/lsd"
)

var (
	// ErrNoDictionaries is returned by [Open] if no readable dictionary
	// was found in the directory.
	ErrNoDictionaries = errors.New("lengua: no dictionaries found")
	// ErrUnknownDictionary is returned for dictionary names not in the
	// library.
	ErrUnknownDictionary = errors.New("lengua: unknown dictionary")
	// ErrNotFound is returned when a media file or abbreviation is missing.
	ErrNotFound = errors.New("lengua: not found")
	// ErrClosed is returned by methods called after [Library.Close].
	ErrClosed = errors.New("lengua: library is closed")
)

// Library is a collection of Lingvo dictionaries and sound archives found
// in a directory tree. Files are opened lazily on first use and kept open
// until [Library.Close].
//
// A Library is safe for concurrent use, except that Close must not run
// concurrently with other methods.
type Library struct {
	infos   []DictionaryInfo
	skipped map[string]error

	dicts    map[string]*lazy[*lsd.Dictionary] // by file path, incl. abbreviations
	archives map[string]*lazy[*lsa.Archive]    // by file path
	global   string                            // shared Abbrev.lsd, may be empty

	closed atomic.Bool
}

// Open scans dir recursively for Lingvo X3 files and returns a library.
// dir may be the installation media root, its Data/<id> directory or any
// directory that contains the files listed in the README:
//
//   - *.lsd   dictionaries (required)
//   - *_abrv.lsd  abbreviation lists, matched to X.lsd by file name
//   - *.lod   metadata linking dictionaries to sound archives
//   - *.lsa   sound archives
//
// Only file headers are read here, so Open is fast. Files that are not
// readable X3 dictionaries are skipped; see [Library.Skipped].
func Open(dir string) (*Library, error) {
	s, err := scanDir(dir)
	if err != nil {
		return nil, fmt.Errorf("lengua: scanning %s: %w", dir, err)
	}
	infos, skipped := s.catalog()
	if len(infos) == 0 {
		return nil, fmt.Errorf("%w in %s", ErrNoDictionaries, dir)
	}
	l := &Library{
		infos:    infos,
		skipped:  skipped,
		dicts:    map[string]*lazy[*lsd.Dictionary]{},
		archives: map[string]*lazy[*lsa.Archive]{},
		global:   s.global,
	}
	// All lazy slots are created upfront so the maps are read-only later.
	addDict := func(path string) {
		if path != "" && l.dicts[path] == nil {
			l.dicts[path] = newLazy(path, lsd.Open)
		}
	}
	for _, info := range infos {
		addDict(info.Path)
		addDict(info.AbbrevPath)
		if info.SoundPath != "" && l.archives[info.SoundPath] == nil {
			l.archives[info.SoundPath] = newLazy(info.SoundPath, lsa.Open)
		}
	}
	addDict(s.global)
	return l, nil
}

// Dictionaries returns all dictionaries, sorted by name.
func (l *Library) Dictionaries() []DictionaryInfo {
	return append([]DictionaryInfo(nil), l.infos...)
}

// Skipped returns the .lsd files that could not be read, e.g. because they
// come from another Lingvo version, with the reason.
func (l *Library) Skipped() map[string]error {
	out := make(map[string]error, len(l.skipped))
	for k, v := range l.skipped {
		out[k] = v
	}
	return out
}

// Find returns the dictionaries translating from source to target.
// A zero language matches any language. Languages are compared by
// [lsd.Language.Primary], so German matches GermanNewSpelling.
func (l *Library) Find(source, target lsd.Language) []DictionaryInfo {
	matches := func(want, have lsd.Language) bool {
		return want == 0 || want.Primary() == have.Primary()
	}
	var out []DictionaryInfo
	for _, info := range l.infos {
		if matches(source, info.Source) && matches(target, info.Target) {
			out = append(out, info)
		}
	}
	return out
}

// Info returns the description of the named dictionary. The name is
// either the dictionary name ("LingvoUniversal (En-Ru)") or its file name
// without extension ("LingvoUniversalEnRu"), compared case-insensitively.
func (l *Library) Info(name string) (DictionaryInfo, error) {
	for _, info := range l.infos {
		if strings.EqualFold(info.Name, name) || strings.EqualFold(fileStem(info.Path), name) {
			return info, nil
		}
	}
	return DictionaryInfo{}, fmt.Errorf("%w: %q", ErrUnknownDictionary, name)
}

// Dictionary returns the named dictionary (see [Library.Info] for the
// accepted names), opening it on first use. The returned dictionary is
// owned by the library: don't close it.
func (l *Library) Dictionary(name string) (*lsd.Dictionary, error) {
	info, err := l.Info(name)
	if err != nil {
		return nil, err
	}
	return l.open(info.Path)
}

func (l *Library) open(path string) (*lsd.Dictionary, error) {
	if l.closed.Load() {
		return nil, ErrClosed
	}
	return l.dicts[path].get()
}

func (l *Library) archive(path string) (*lsa.Archive, error) {
	if l.closed.Load() {
		return nil, ErrClosed
	}
	return l.archives[path].get()
}

// Preload opens all dictionaries in parallel so that later lookups don't
// pay the opening cost. It stops early if ctx is cancelled. Errors of
// individual dictionaries are joined.
func (l *Library) Preload(ctx context.Context) error {
	paths := make(chan string)
	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range paths {
				if _, err := l.open(p); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			}
		}()
	}
loop:
	for p := range l.dicts {
		select {
		case paths <- p:
		case <-ctx.Done():
			break loop
		}
	}
	close(paths)
	wg.Wait()
	return errors.Join(append(errs, ctx.Err())...)
}

// Close closes all opened files. The library can't be used afterwards.
func (l *Library) Close() error {
	if l.closed.Swap(true) {
		return nil
	}
	var errs []error
	for _, d := range l.dicts {
		errs = append(errs, d.close())
	}
	for _, a := range l.archives {
		errs = append(errs, a.close())
	}
	return errors.Join(errs...)
}

// lazy opens a file once, on first use, and remembers the result.
type lazy[T io.Closer] struct {
	once   sync.Once
	path   string
	open   func(string) (T, error)
	opened atomic.Bool
	value  T
	err    error
}

func newLazy[T io.Closer](path string, open func(string) (T, error)) *lazy[T] {
	return &lazy[T]{path: path, open: open}
}

func (z *lazy[T]) get() (T, error) {
	z.once.Do(func() {
		z.value, z.err = z.open(z.path)
		z.opened.Store(z.err == nil)
	})
	return z.value, z.err
}

func (z *lazy[T]) close() error {
	if !z.opened.Load() {
		return nil
	}
	return z.value.Close()
}
