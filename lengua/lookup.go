package lengua

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/frankyfish/lengua/dsl"
	"github.com/frankyfish/lengua/lsd"
)

// LookupOptions select the dictionaries searched by [Library.Lookup] and
// [Library.Suggest]. The zero value searches all dictionaries.
type LookupOptions struct {
	// Source and Target restrict the language pair; zero means any.
	Source, Target lsd.Language
	// Dictionaries restricts the search to the named dictionaries
	// (see [Library.Info] for accepted names).
	Dictionaries []string
}

// Result is an article found by [Library.Lookup].
type Result struct {
	Dictionary DictionaryInfo
	Heading    lsd.Heading
	Article    string // DSL markup; see Document
}

// Document parses the article markup for rendering or data extraction.
func (r Result) Document() *dsl.Document { return dsl.Parse(r.Article) }

// Lookup finds the articles for word in all selected dictionaries. Words
// are compared case-insensitively and ignoring diacritics (see
// [lsd.FoldKey]). Results are ordered by dictionary name, then by heading.
//
// Dictionaries are opened on first use, in parallel. If some fail, the
// results from the others are returned along with the joined errors.
func (l *Library) Lookup(ctx context.Context, word string, opts LookupOptions) ([]Result, error) {
	perDict, err := each(ctx, l, opts, func(info DictionaryInfo, d *lsd.Dictionary) ([]Result, error) {
		var out []Result
		seen := map[string]bool{}
		for _, h := range d.Lookup(word) {
			article, err := d.Article(h)
			if err != nil {
				return out, err
			}
			if seen[article] { // several headings may share one article
				continue
			}
			seen[article] = true
			out = append(out, Result{Dictionary: info, Heading: h, Article: article})
		}
		return out, nil
	})
	return slices.Concat(perDict...), err
}

// Suggest returns up to limit distinct headings starting with prefix
// across the selected dictionaries, sorted alphabetically (by
// [lsd.FoldKey]). It is meant for autocompletion.
func (l *Library) Suggest(ctx context.Context, prefix string, limit int, opts LookupOptions) ([]string, error) {
	perDict, err := each(ctx, l, opts, func(_ DictionaryInfo, d *lsd.Dictionary) ([]string, error) {
		var out []string
		for _, h := range d.Prefix(prefix, limit) {
			out = append(out, h.Display())
		}
		return out, nil
	})
	words := slices.Concat(perDict...)
	slices.SortFunc(words, func(a, b string) int {
		if c := strings.Compare(lsd.FoldKey(a), lsd.FoldKey(b)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	words = slices.Compact(words)
	if limit > 0 && len(words) > limit {
		words = words[:limit]
	}
	return words, err
}

// each runs fn for every selected dictionary in parallel and returns the
// results in dictionary order.
func each[T any](ctx context.Context, l *Library, opts LookupOptions,
	fn func(DictionaryInfo, *lsd.Dictionary) ([]T, error)) ([][]T, error) {
	infos, err := l.selectDicts(opts)
	if err != nil {
		return nil, err
	}
	results := make([][]T, len(infos))
	errs := make([]error, len(infos))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, info := range infos {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			d, err := l.open(info.Path)
			if err == nil {
				results[i], err = fn(info, d)
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	return results, errors.Join(append(errs, ctx.Err())...)
}

func (l *Library) selectDicts(opts LookupOptions) ([]DictionaryInfo, error) {
	candidates := l.Find(opts.Source, opts.Target)
	if len(opts.Dictionaries) == 0 {
		return candidates, nil
	}
	var out []DictionaryInfo
	for _, name := range opts.Dictionaries {
		info, err := l.Info(name)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(candidates, func(c DictionaryInfo) bool { return c.Path == info.Path }) {
			out = append(out, info)
		}
	}
	return out, nil
}

// Media returns a file referenced from an article of the named dictionary
// with [s]name[/s]: a sound as a WAV file from the dictionary's sound
// archive, or a picture embedded in the dictionary.
func (l *Library) Media(dictionary, name string) ([]byte, error) {
	info, err := l.Info(dictionary)
	if err != nil {
		return nil, err
	}
	if info.SoundPath != "" {
		a, err := l.archive(info.SoundPath)
		if err != nil {
			return nil, err
		}
		if a.Has(name) {
			return a.WAV(name)
		}
	}
	d, err := l.open(info.Path)
	if err != nil {
		return nil, err
	}
	data, err := d.ReadMedia(name)
	if errors.Is(err, lsd.ErrMediaNotFound) {
		return nil, fmt.Errorf("%w: media %q in %q", ErrNotFound, name, info.Name)
	}
	return data, err
}

// Abbreviation expands a label used in the named dictionary, such as
// "прил." to "имя прилагательное". It consults the dictionary's own
// abbreviation list first and then the shared Abbrev.lsd, if present.
func (l *Library) Abbreviation(dictionary, label string) (string, error) {
	info, err := l.Info(dictionary)
	if err != nil {
		return "", err
	}
	for _, path := range []string{info.AbbrevPath, l.global} {
		if path == "" {
			continue
		}
		d, err := l.open(path)
		if err != nil {
			return "", err
		}
		for _, h := range d.Lookup(label) {
			article, err := d.Article(h)
			if err != nil {
				return "", err
			}
			if text := strings.TrimSpace(dsl.Parse(article).Text()); text != "" {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("%w: abbreviation %q in %q", ErrNotFound, label, info.Name)
}
