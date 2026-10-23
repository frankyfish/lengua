# lengua

A pure-Go library and command-line tool for reading the dictionaries of an
**ABBYY Lingvo X3** installation. It covers:

- dictionary lookup;
- word suggestions;
- article markup (DSL) parsing and rendering to text or HTML;
- pronunciation sounds;
- embedded pictures;
- abbreviation lists;
- full export to the open DSL source format.

> This project is not affiliated with, endorsed by or sponsored by ABBYY.
> ABBYY and Lingvo are trademarks of their respective owners and are used
> here only to describe compatibility. No dictionary data is included.

```
github.com/frankyfish/lengua              high-level API: Library, Lookup, Suggest, Media, Abbreviation
github.com/frankyfish/lengua/lsd          reader for compiled dictionaries (*.lsd)
github.com/frankyfish/lengua/dsl          parser and renderers for article markup
github.com/frankyfish/lengua/lsa          reader for sound archives (*.lsa)
github.com/frankyfish/lengua/cmd/lengua   command-line tool
```

Requires Go 1.23 or newer. The only dependency is a pure-Go Ogg Vorbis
decoder (`github.com/jfreymuth/oggvorbis`), so no cgo is needed.

## Quick start

```go
lib, err := lengua.Open("/path/to/L-X3")
if err != nil {
	log.Fatal(err)
}
defer lib.Close()

results, err := lib.Lookup(ctx, "serene", lengua.LookupOptions{
	Source: lsd.English,
	Target: lsd.Russian,
})
if err != nil {
	log.Print(err) // partial results may still be returned
}
for _, r := range results {
	doc := r.Document() // parsed DSL markup
	fmt.Println(r.Dictionary.Name)
	fmt.Println(doc.Text())                          // plain text
	fmt.Println(doc.Translations())                  // [безмятежный спокойный ...]
	html := doc.HTML(dsl.HTMLOptions{})              // safe, escaped HTML
	for _, name := range doc.Media() {               // e.g. "serene.wav"
		wav, err := lib.Media(r.Dictionary.Name, name) // WAV file bytes
		_, _ = wav, err
	}
	_ = html
}
```

Other entry points:

| Task | API |
|---|---|
| List and filter dictionaries | `lib.Dictionaries()`, `lib.Find(lsd.German, lsd.Russian)`, `lib.Info(name)` |
| Autocomplete | `lib.Suggest(ctx, "sere", 20, opts)` |
| Expand a label such as `прил.` | `lib.Abbreviation(dict, "прил.")` |
| Iterate over a whole dictionary | `d, _ := lib.Dictionary(name)`, then `for e := range d.Entries() { e.Article() }` |
| Convert to DSL source | `d.WriteDSL(w, lsd.ExportOptions{})` |
| Use one file directly | `lsd.Open("X.lsd")`, `lsa.Open("SoundEn.lsa")` |
| Warm up all dictionaries | `lib.Preload(ctx)` |

Language selection:
- `lsd.ParseLanguage` accepts ISO codes (`"en"`), English names
  (`"english"`) and numeric codes (`"1033"`).
- Language filters compare primary languages, so `lsd.German` also matches
  dictionaries tagged `GermanNewSpelling`.

All types are safe for concurrent use. Run `go doc` on any package for the
full API reference.

## Command-line tool

```sh
go install github.com/frankyfish/lengua/cmd/lengua@latest
export LINGVO_DATA=/path/to/L-X3      # or pass -data DIR

lengua list -from en -to ru                       # dictionaries
lengua lookup -from en -to ru serene              # articles as text
lengua lookup -dict LingvoUniversalEnRu -format html serene
lengua suggest -from de -n 10 sere                # flags go before the word
lengua media -dict LingvoUniversalEnRu serene.wav > serene.wav
lengua export-dsl -utf8 out/                      # all dictionaries to DSL
lengua export-sounds path/to/SoundEn.lsa out/     # a whole sound archive to WAV
```

## Required files

The library scans the directory passed to `Open` recursively. File names
and the folder layout don't matter. For the original installation
(`L-X3/`, 2.3 GB), only the files below are read.

| Files | Where in L-X3 | Required | Purpose |
|---|---|---|---|
| `*.lsd` (217 files, ~550 MB) | `Data/<id>/LingvoDic/` | **yes** | The dictionaries: headings, articles, embedded pictures |
| `*_abrv.lsd` (161 files, ~1 MB) | `Data/<id>/LingvoDic/` | optional | Per-dictionary abbreviation lists for `Abbreviation` |
| `Abbrev.lsd` | `ABBYY Lingvo x3/` | optional | Shared abbreviation list, used as a fallback by `Abbreviation` |
| `*.lod` (216 files, XML) | `Data/<id>/LingvoDic/` | optional | Link dictionaries to their sound archives and give product IDs. **Without them `Media` can't find sounds.** |
| `*.lsa` (12 files, ~340 MB) | `Data/<id>/Sound/` | optional | Pronunciation and phrase-book sounds (Ogg Vorbis) for `Media` |

A minimal setup that keeps every feature (~890 MB) is to copy the
`LingvoDic` and `Sound` folders plus `Abbrev.lsd`.

The library doesn't read these files, so they can be deleted:
- `Index/*.trs` (1.1 GB): full-text search indexes.
- `*.amd` and `*.amm`: morphology.
- `Licenses`, `dictinst.ini` and `*.mst`.
- Installers: `*.msi`, `*.cab` and `*.exe`.
- Fonts, plus the `Mobile_install` and `PDA_install` folders.

## Limitations

- Only the Lingvo X3 formats are supported (LSD versions `0x131001` and
  `0x132001`). Files from other Lingvo versions fail with
  `lsd.ErrUnsupportedVersion`.
- Lookup is exact or prefix matching on a normalized key:
  - case, stress marks and ё/е are ignored;
  - there is no morphology, so "went" doesn't find "go", and there is no
    full-text search.
- Optional heading parts are kept as stored. For example, `colo(u)r` is
  stored as two headings and stays two headings.
- Opening every dictionary (`Preload`) keeps all 7.6 million headings in
  memory: about 1.8 s and 360 MB of heap on an Apple M-series laptop.
  Dictionaries are opened lazily, so typical use costs far less.

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): package structure, data flow,
  concurrency and performance design.
- [docs/FORMATS.md](docs/FORMATS.md): the LSD, LSA, LOD and DSL file formats.

## Testing

```sh
go test ./...                                         # unit tests only
LINGVO_DATA=/path/to/L-X3 go test -race ./...         # plus integration tests
go test -fuzz FuzzParse ./dsl                         # markup parser fuzzing
```

The decoder was verified against the whole X3 collection:
- All 7.6 million headings match the reference implementations.
- Every article decodes.
- All 77,908 sound clips decode to within ±1 LSB of libvorbis.

## License and credits

MIT, see [LICENSE](LICENSE), which also contains the notices of the
projects below.

The LSD decoding algorithm is a port of two MIT-licensed projects:
- [lsd2dsl](https://github.com/nongeneric/lsd2dsl) by nongeneric;
- [lsdreader](https://github.com/sv99/lsdreader) by sv99.

The dictionary content belongs to ABBYY and the respective publishers.
Use this library only with dictionaries you are licensed to use.
