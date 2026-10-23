
## decompiler

https://rcebits.com/lsd2dsl/index.html


## Summary 

The Go library is ready in  lingvo/  (module  github.com/ffish/lingvo ), and all tests pass, including with the race detector, against your installation.

• Packages:
•  lingvo : high-level API for lookup, suggestions, sounds and pictures, and abbreviation expansion.
•  lsd : dictionary reader, including export back to DSL source.
•  dsl : parses article markup and renders it as text or HTML.
•  lsa : sound archives decoded to WAV.
•  cmd/lingvo : a command-line tool.
• Checked against all your data: all 7.6M headwords match the existing open-source decoders, every article decodes, and all 77,908 sound clips decode.
• Docs:  README.md ,  docs/ARCHITECTURE.md  and  docs/FORMATS.md .

Required files:

• Needed:  Data/*/LingvoDic/*.lsd  (the dictionaries).
• Optional:
•  *.lod  (links dictionaries to their sounds)
•  Sound/*.lsa  (pronunciation audio)
•  *_abrv.lsd  and  ABBYY Lingvo x3/Abbrev.lsd  (abbreviation lists)
• Not used:  Index/*.trs ,  .amd / .amm  and the installers.

The main limits are that lookup has no word-form matching ("went" won't find "go"), and loading every dictionary at once takes about 1.8 s and 360 MB.

Temporary files from the earlier analysis are still in  /tmp  ( lsdall ,  lsdreader ,  lsd2dsl ,  lsdenv ); you can delete them.
