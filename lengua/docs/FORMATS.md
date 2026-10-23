# File formats

This file describes the ABBYY Lingvo X3 formats as implemented by this
library. The LSD description comes from the reverse engineering work in
[lsd2dsl](https://github.com/nongeneric/lsd2dsl) and
[lsdreader](https://github.com/sv99/lsdreader), checked against the full
X3 collection. The LSA and LOD descriptions were worked out for this
library.

Conventions: `u8/u16/u32` are little-endian unless stated otherwise. A
*bit stream* is read MSB-first, and its multi-bit fields are big-endian.

## Installation layout

```
L-X3/
├── ABBYY Lingvo x3/Abbrev.lsd         shared abbreviation list (Ru-Ru)
└── Data/<product-id>/
    ├── LingvoDic/
    │   ├── <Dict>.lsd                 compiled dictionary
    │   ├── <Dict>_abrv.lsd            its abbreviation list (optional)
    │   └── <Dict_(Xx-Yy)>.lod         XML metadata (name differs from .lsd!)
    ├── Sound/<name>.lsa               sound archives
    ├── Index/*.trs                    full-text index   (not used)
    └── *.amd, *.amm                   morphology        (not used)
```

## LSD: compiled dictionary

### Fixed header (52 bytes)

| Offset | Type | Field |
|---|---|---|
| 0x00 | 8 bytes | magic `"LingVo"` + 2 zero bytes |
| 0x08 | u32 | version: `0x131001` (system) or `0x132001` (user-compiled) |
| 0x0C | u32 | unknown |
| 0x10 | u32 | checksum |
| 0x14 | u32 | heading count |
| 0x18 | u32 | annotation offset |
| 0x1C | u32 | codec ("encoder") offset |
| 0x20 | u32 | articles offset |
| 0x24 | u32 | heading pages offset |
| 0x28 | u32 | unknown |
| 0x2C | u16, u16 | last page, unknown |
| 0x30 | u16 | source language |
| 0x32 | u16 | target language |

The section offsets are absolute and must not decrease in this order:
annotation, codec, articles, pages.

### Variable header (from 0x34)

| Type | Field |
|---|---|
| u8 n + n×UTF-16LE | dictionary name |
| u8 n + n×UTF-16LE | first heading |
| u8 n + n×UTF-16LE | last heading |
| u32 n + n×UTF-16LE | alphabet |
| u16 n + n bytes | icon (BMP) |
| u32 | pages end = start of the media index |
| u32 | overlay offset (unused in X3) |

### File sections

```
┌──────────┬────────────┬───────┬──────────┬──────────────┬─────────────┐
│ header   │ annotation │ codec │ articles │ heading pages│ media index │
│          │ (article)  │       │          │ 512 B each   │ + zlib data │
└──────────┴────────────┴───────┴──────────┴──────────────┴─────────────┘
```

### Codec (bit stream at the codec offset)

1. `u32 n`, then `n` × 16-bit UTF-16 units: the **prefix**, a shared
   dictionary of frequent substrings.
2. Two **symbol tables**, for articles and for headings. Each one is
   `u32 count`, `u8 width`, then `count` symbols of `width` bits.
3. Four **Huffman tables**: articles, headings, prefix lengths and
   postfix lengths. Each one is `u32 count`, `u8 bitsPerLen`, then
   `count` × (`symbolIndex` of bitLength(count) bits, `length` of
   bitsPerLen bits).
   - The tree is rebuilt by placing each symbol at the leftmost free slot
     of depth `length`.
   - A decoded Huffman leaf is an index into the symbol table.
4. `u32 a`, `u32 b`. The widths of page references and article
   references are bitLength(a) and bitLength(b).

### References

A reference starts with a 2-bit tag. Tag `3` means a full `u32` follows.
Otherwise the value is `tag << (bits-2) | next (bits-2) bits`.

### Heading pages

A heading B-tree of 512-byte pages. Each page header is:
- 1 bit: is a leaf;
- 4 × 16 bits: page number, previous, parent and next page;
- 16 bits: heading count;
- padding to a byte boundary.

Only leaf pages are needed, because they hold all headings in sort order.
Each heading is:

```
prefixLen   Huffman(prefix lengths)   chars shared with the previous heading
postfixLen  Huffman(postfix lengths)
postfix     postfixLen × Huffman(headings) → symbol table → UTF-16 unit
ref         reference(b)              article offset, relative to articles
1 bit       has unsorted chars
  [u8 n, n × (u8 position, u16 char)] unsorted chars to insert
```

How the stored characters map to a heading:
- *Sorted* characters are the searchable text, called `Heading.Text`.
- *Unsorted* characters are displayed but not searched. In DSL they are
  written in braces: `some{thing}`.
- A backslash escapes the following character, whether it is sorted or
  unsorted.
- Several headings may share one article reference. These are synonyms or
  variants, and the library groups them into one `Entry`.

### Articles

An article starts at `articlesOffset + ref` and ends at the next larger
distinct reference (or at the pages offset). Its bit stream is:

```
size   16 bits, or 0xFFFF followed by 32 bits      output length in UTF-16 units
repeat until size units are produced:
  sym = symbolTable[Huffman(articles)]
  classify(sym) →  literal char
                |  prefix copy (len)   + start: bitLength(len(prefix)) bits
                |  self copy (len)     + start: bitLength(size) bits
```

| Scheme | Literal | Prefix copy | Self copy |
|---|---|---|---|
| system `0x131001` | `sym ≥ 0x80`: char = sym−0x80 | `sym ≤ 0x3F`: len = sym+3 | otherwise: len = sym−0x3D |
| user `0x132001` | `sym < 0x10000`: char = sym | `0x10000 ≤ sym < 0x10040`: len = sym−0xFFFD | `sym ≥ 0x10040`: len = sym−0x1003D |

Copies are clamped to the source bounds. Because copies expand the output,
an article can be much longer than its encoded size. The result is DSL
markup (see below). The annotation uses the same encoding.

### Media index (at pages end)

```
u32 count
count × { u8 n, n × UTF-16LE name, u32 offset (absolute), u32 unknown,
          u32 uncompressed size, u32 compressed size }
```

Each file is a zlib stream. Most X3 dictionaries have no media. Picture
dictionaries embed JPG/BMP files referenced from articles as
`[s]picture.jpg[/s]`.

### Abbreviation lists

`*_abrv.lsd` and `Abbrev.lsd` are ordinary LSD files named "Abbrev".
- Headings are labels such as `прил.`.
- Articles are their expansions.

## LSA: sound archive

```
string  magic "L9SA"
u32     record count
records:
  string name
  if not the first record:
      u32 sample offset
      u8  marker: 0xFF = clip, 0x00 = group header (no sample count follows)
  u32 sample count                (clips only)
Ogg Vorbis stream (all clips concatenated; rate and channels in its header)
```

- A `string` is UTF-16LE terminated by a `0xFF` byte, which may be either
  the low or the high byte of a code unit.
- Names may have trailing spaces, which the library trims.
- The first clip starts at sample 0.
- A clip is the sample range `[offset, offset + count)` of the decoded
  stream.
- The X3 collection has 12 archives with 77,908 clips. `<soundName>` in
  `.lod` files links dictionaries to archives, for example `SoundEn` →
  `sounden.lsa` and `PhraseBookRuEn` → `phrasebookruen.lsa` (matched
  case-insensitively).

## LOD: dictionary metadata

An XML file next to each dictionary. Only these elements are used:

```xml
<id>…</id>              <!-- ABBYY product id -->
<name>…</name>          <!-- equals the name in the LSD header -->
<soundName>…</soundName><!-- stem of the .lsa sound archive, optional -->
```

LOD file names don't match LSD file names (`Accounting_(En-Ru).lod` vs
`AccountingEnRu.lsd`), so files are linked by `<name>`.

## Language codes

Most codes are Windows LCIDs, for example 1033 English, 1049 Russian and
1036 French. Other codes found in X3:

| Code | Language |
|---|---|
| 32775 | GermanNewSpelling, used by German dictionaries |
| 2070 | PortugueseStandard |
| 1540 | Latin (ABBYY private) |

`Language.Primary()` folds such variants into the base language.

## DSL: article markup

Articles use the markup of ABBYY's open DSL source format:

| Tag | Meaning |
|---|---|
| `[m0]`…`[m9]` `[/m]` | paragraph with left indent N |
| `[trn]` `[!trs]` | translation (`[!trs]`: excluded from indexing) |
| `[ex]` | example |
| `[com]` | comment |
| `[p]` | label/abbreviation (expand with the `_abrv` list) |
| `[t]` | transcription |
| `[s]` | media file name (sound or picture) |
| `[ref]` `<<…>>` | cross-reference to another heading |
| `[url]` | web link |
| `[lang id=N]` / `[lang name="…"]` | language of the enclosed text |
| `[*]` | secondary (expandable) content |
| `[b]` `[i]` `[u]` `[c color]` `[sub]` `[sup]` | formatting |
| `[']` | stress mark |

- Literal `[ ] { } ~ \ @` are escaped with a backslash.
- `~` stands for the heading. Lingvo substitutes it in DSL sources only;
  compiled articles already contain the expanded text.

`lsd.Dictionary.WriteDSL` writes a dictionary as a DSL source file:
- the `#NAME`, `#INDEX_LANGUAGE` and `#CONTENTS_LANGUAGE` headers;
- each entry's headings, one per line;
- the article lines indented by a tab.

The output is UTF-16LE with a BOM by default, or UTF-8.
