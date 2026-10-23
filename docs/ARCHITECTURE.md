# Architecture

## Package layout

```
                       ┌──────────────────────────┐
  cmd/lengua  ───────▶ │  lengua  (root package)  │  Library: catalog, lookup,
  (CLI)                │  catalog.go library.go   │  media, abbreviations
                       │  lookup.go               │
                       └──┬──────────┬─────────┬──┘
                          │          │         │
                 ┌────────▼───┐ ┌────▼────┐ ┌──▼──────┐
                 │    lsd     │ │   lsa   │ │   dsl   │
                 │ .lsd files │ │ .lsa    │ │ article │
                 │            │ │ sounds  │ │ markup  │
                 └────────────┘ └────┬────┘ └─────────┘
                                     │
                         jfreymuth/oggvorbis (pure Go)
```

Each lower package works on its own and doesn't import the others:

| Package | Responsibility | Main types |
|---|---|---|
| `lsd` | Decode one compiled dictionary: header, compression model, heading B-tree, articles, embedded media. Export it to DSL source. | `Dictionary`, `Heading`, `Entry`, `Info`, `Language`, `MediaFile` |
| `lsa` | Read one sound archive: the clip index and random-access Ogg Vorbis decoding to PCM or WAV. | `Archive`, `Entry` |
| `dsl` | Parse DSL article markup into a tree, extract data from it and render it as text or HTML. | `Document`, `Node`, `HTMLOptions` |
| `lengua` | Discover the files of an installation, link them together and provide multi-dictionary operations. | `Library`, `DictionaryInfo`, `Result` |

The low-level packages are useful on their own, for example to convert one
`.lsd` file or to dump a sound archive. Applications normally use only
`lengua` and `dsl`.

## Data flow

### Opening a library

`lengua.Open(dir)` does no decoding. It:

1. Walks `dir` and classifies files by extension:
   - `*.lsd`: dictionaries;
   - `*_abrv.lsd`: abbreviation lists;
   - `Abbrev.lsd`: the shared abbreviation list;
   - `*.lod`: metadata;
   - `*.lsa`: sound archives.
2. Reads only the header of every `.lsd` with `lsd.ReadInfo` (about 1 KB per
   file) to get the name, the languages and the heading count. Files that
   fail are reported by `Skipped()` instead of failing the whole library.
3. Links each dictionary to:
   - its `_abrv` file, by file stem;
   - its `.lod` file, by matching the `<name>` element to the LSD header
     name, since `.lod` file names differ from `.lsd` ones;
   - its `.lsa` archive, through the `<soundName>` element of the `.lod`.
4. Creates one lazy slot per file. A slot opens its file on first use.

Opening the full X3 collection takes about 0.2 s.

### Looking up a word

```
Library.Lookup(ctx, word, opts)
  ├─ selectDicts(opts)         filter by language / name
  └─ each(...)                 parallel, bounded by GOMAXPROCS, cancellable
       └─ per dictionary:
            lazy open → lsd.Open (decode all headings once)
            Dictionary.Lookup(word)  → binary search on FoldKey
            Dictionary.Article(h)    → decode one article on demand
  → results in catalog order, de-duplicated by article,
    plus errors.Join of per-dictionary failures (partial results)
```

`Result.Article` is raw DSL markup. `Result.Document()` parses it with the
`dsl` package. The library doesn't render anything by itself, so the caller
chooses text, HTML, structured extraction or the raw markup.

### Media

`Library.Media(dict, name)` first looks for `name` in the dictionary's sound
archive. There it is decoded from Ogg Vorbis and wrapped in a WAV header.
Otherwise the file comes from the media embedded in the `.lsd`, mostly
pictures, which are zlib-decompressed.

## The lsd package

Opening a dictionary:

1. Reads and validates the header (`header.go`).
2. Reads the compression model (`codec.go`, `huffman.go`):
   - a shared prefix string;
   - two symbol tables;
   - four Huffman tables.
3. Decodes every leaf page of the heading B-tree (`dictionary.go`,
   `heading.go`) into a compact in-memory index (`index.go`):
   - all heading strings live in one shared buffer;
   - each heading costs a 12-byte record plus a 4-byte sort entry;
   - a permutation sorted by `FoldKey` allows binary search.
4. Reads the media index (`media.go`).

Articles aren't decoded when the dictionary is opened:
- `Article(h)` decodes one article at a time from the file with `ReadAt`.
- The end of an article is found by binary search in the sorted list of
  article offsets.

Design choices:
- **Sticky-error bit reader.** Every read past the end returns zero and
  records `errTruncated`. Decoding loops check the error once per step
  instead of after every field, and corrupt input can't cause a panic.
- **Bounded work on corrupt input.**
  - Allocations are capped: at most 64K units of article preallocation,
    `count ≤ 2^24` for Huffman tables and `≤ 2^20` media entries.
  - Article decoding is limited to `size + input bits` steps.
- **Two compression schemes.** System (`0x131001`) and user (`0x132001`)
  dictionaries differ only in how a symbol is classified (literal,
  prefix copy or self copy). This is isolated in `codec.classify`.
- **Headings bound to their dictionary.** A `Heading` carries a pointer to
  its owner, and `Article` rejects foreign headings with
  `ErrForeignHeading` instead of returning another dictionary's text.

## The dsl package

`Parse` never fails. Real dictionaries contain malformed markup, so the
parser:
- treats unknown tags and stray brackets as literal text;
- repairs misnested tags by closing and reopening them, like Lingvo does;
- removes empty elements and merges adjacent text nodes.

The tree is small and plain: `Node{Tag, Attr, Level, Content, Children}`.
Paragraph tags `[m0]` to `[m9]` become one tag `m` with a `Level`.

Renderers:
- `Text()` indents each `[mN]` paragraph by 2·N spaces.
- `HTML()` uses CSS classes `dsl-*`; `DefaultCSS` gives a Lingvo-like look.
- `String()` re-serializes to DSL markup.

HTML output is safe for untrusted input:
- all text and attributes are escaped;
- colors are validated;
- `[url]` accepts only `http`, `https` and `mailto` links.

Extractors (`Translations`, `Examples`, `References`, `Media`, …) walk the
tree and return plain Go values.

The parser is fuzz-tested (`FuzzParse`). The fuzz test checks that parsing
never panics and that `String()` round-trips stably.

## The lsa package

A sound archive is a clip index followed by a single Ogg Vorbis stream
that contains all clips back to back. A clip is a sample range
`[offset, offset+samples)` of that stream.

`oggvorbis.Reader.SetPosition` finds the target page by scanning from the
start of the stream. Deep in a 100 MB archive that takes about 100 ms.
To avoid this, the archive builds a page index once
(`scanPages`, ~20 ms, on the first decode). Each decoder reads through a
`rebasedReader` whose origin is moved to the last page at or before the
clip start, so `SetPosition` scans only a page or two.

Decoders are pooled with `sync.Pool`, so concurrent `PCM`/`WAV` calls
don't share state. A decoder that failed is dropped rather than reused.
Opening an archive takes about 10 ms, and decoding a clip about 2 ms.

## Concurrency

- `lsd.Dictionary`, `lsa.Archive`, `dsl.Document` (read-only use) and
  `lengua.Library` are safe for concurrent use.
- Files are read with `ReadAt` only, so there is no shared file position.
- The decoded state (codec, heading index, media index) is immutable after
  `Open`.
- `Library` creates all lazy slots in `Open`, so its maps are never
  written afterwards. Each slot opens its file exactly once using
  `sync.Once` semantics and caches the error as well.
- Multi-dictionary operations run in parallel, bounded by `GOMAXPROCS`,
  and stop starting new work when the context is cancelled.

## Memory and performance

Measured on an Apple M-series laptop with the full X3 collection (216
dictionaries, 7.6 million headings):

| Operation | Time | Memory |
|---|---|---|
| `lengua.Open` (headers only) | ~0.2 s | negligible |
| Open the largest dictionary on first lookup | ~0.1–0.3 s | proportional to its headings |
| `Library.Preload` (all dictionaries) | ~1.8 s | ~360 MB heap |
| Lookup of a word in all En→Ru dictionaries (warm) | milliseconds | – |
| Decode a sound clip | ~2 ms | clip size |

Most applications use a few dictionaries and should rely on lazy opening.
Servers that need predictable latency can call `Preload` at start-up.

## Error handling

- Errors are exported sentinels wrapped with context, to be tested with
  `errors.Is`: `lsd.ErrNotLSD`, `lsd.ErrUnsupportedVersion`,
  `lsd.ErrMediaNotFound`, `lsa.ErrNotLSA`, `lsa.ErrNotFound`,
  `lengua.ErrNoDictionaries`, `ErrUnknownDictionary`, `ErrNotFound` and
  `ErrClosed`.
- Multi-dictionary operations return partial results together with
  `errors.Join` of the failures, so one damaged file doesn't hide the
  others.

## Testing

| Level | What | Data |
|---|---|---|
| Unit | bit reader, heading assembly, key folding, language parsing, LSA index, WAV header, seeking helpers, DSL parser and renderers | synthetic, always run |
| Fuzz | `dsl.FuzzParse` | generated |
| Integration | lookup, article decoding, DSL export, sounds, catalog linking, abbreviations, closing | real installation via `LINGVO_DATA`, skipped otherwise |

During development, the decoder output for the entire collection was also
compared with the reference implementations (see README).

## Extending

Each item below fits into one place:

- **Morphology** (`.amd`/`.amm`): a new package that maps word forms to
  lemmas before `Library.Lookup` searches.
- **Full-text search** (`Index/*.trs`): a new package used by a new
  `Library` method.
- **Other Lingvo versions**: further `articleScheme` values in
  `lsd/codec.go` plus header variants in `lsd/header.go`.
