// Command lengua looks up words and exports data from ABBYY Lingvo X3
// dictionaries.
//
// Usage:
//
//	lengua [-data DIR] list [-from LANG] [-to LANG]
//	lengua [-data DIR] lookup [-from LANG] [-to LANG] [-dict NAME] [-format text|html|dsl] WORD
//	lengua [-data DIR] suggest [-from LANG] [-to LANG] [-dict NAME] [-n 20] PREFIX
//	lengua [-data DIR] export-dsl [-utf8] [-dict NAME] OUTDIR
//	lengua [-data DIR] media -dict NAME FILE > out
//	lengua export-sounds ARCHIVE.lsa OUTDIR
//
// The data directory defaults to $LINGVO_DATA. Languages are given as
// ISO codes (en), English names (English) or Lingvo codes (1033).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/frankyfish/lengua"
	"github.com/frankyfish/lengua/dsl"
	"github.com/frankyfish/lengua/lsa"
	"github.com/frankyfish/lengua/lsd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lengua:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	global := flag.NewFlagSet("lengua", flag.ExitOnError)
	data := global.String("data", os.Getenv("LINGVO_DATA"), "Lingvo data directory")
	global.Usage = usage
	global.Parse(args)
	if global.NArg() == 0 {
		usage()
		return errors.New("missing command")
	}
	cmd, rest := global.Arg(0), global.Args()[1:]

	if cmd == "export-sounds" {
		return exportSounds(rest)
	}
	commands := map[string]func(context.Context, *lengua.Library, []string) error{
		"list":       list,
		"lookup":     lookup,
		"suggest":    suggest,
		"export-dsl": exportDSL,
		"media":      media,
	}
	fn, ok := commands[cmd]
	if !ok {
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
	if *data == "" {
		return errors.New("no data directory: use -data or set LINGVO_DATA")
	}
	lib, err := lengua.Open(*data)
	if err != nil {
		return err
	}
	defer lib.Close()
	return fn(ctx, lib, rest)
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  lengua [-data DIR] list [-from LANG] [-to LANG]
  lengua [-data DIR] lookup [-from LANG] [-to LANG] [-dict NAME] [-format text|html|dsl] WORD
  lengua [-data DIR] suggest [-from LANG] [-to LANG] [-dict NAME] [-n 20] PREFIX
  lengua [-data DIR] export-dsl [-utf8] [-dict NAME] OUTDIR
  lengua [-data DIR] media -dict NAME FILE > out
  lengua export-sounds ARCHIVE.lsa OUTDIR

DIR defaults to $LINGVO_DATA.
`)
}

// selectionFlags registers -from, -to and -dict and returns a function
// that resolves them into lookup options.
func selectionFlags(fs *flag.FlagSet) func() (lengua.LookupOptions, error) {
	from := fs.String("from", "", "source language")
	to := fs.String("to", "", "target language")
	dict := fs.String("dict", "", "search only this dictionary")
	return func() (lengua.LookupOptions, error) {
		var opts lengua.LookupOptions
		var err error
		if *dict != "" {
			opts.Dictionaries = []string{*dict}
		}
		if opts.Source, err = parseLanguage(*from); err != nil {
			return opts, err
		}
		opts.Target, err = parseLanguage(*to)
		return opts, err
	}
}

func parseLanguage(s string) (lsd.Language, error) {
	if s == "" {
		return 0, nil
	}
	l, ok := lsd.ParseLanguage(s)
	if !ok {
		return 0, fmt.Errorf("unknown language %q", s)
	}
	return l, nil
}

func list(_ context.Context, lib *lengua.Library, args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	selection := selectionFlags(fs)
	fs.Parse(args)
	opts, err := selection()
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tFROM\tTO\tHEADINGS\tSOUND\tFILE")
	infos := lib.Find(opts.Source, opts.Target)
	if len(opts.Dictionaries) > 0 {
		info, err := lib.Info(opts.Dictionaries[0])
		if err != nil {
			return err
		}
		infos = []lengua.DictionaryInfo{info}
	}
	for _, d := range infos {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", d.Name, d.Source, d.Target,
			d.HeadingCount, fileStem(d.SoundPath), fileStem(d.Path))
	}
	return tw.Flush()
}

func lookup(ctx context.Context, lib *lengua.Library, args []string) error {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	selection := selectionFlags(fs)
	format := fs.String("format", "text", "output format: text, html or dsl")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("lookup: missing word")
	}
	opts, err := selection()
	if err != nil {
		return err
	}
	results, err := lib.Lookup(ctx, strings.Join(fs.Args(), " "), opts)
	for _, r := range results {
		fmt.Printf("=== %s — %s\n", r.Dictionary.Name, r.Heading.Display())
		switch *format {
		case "dsl":
			fmt.Println(r.Article)
		case "html":
			fmt.Println(r.Document().HTML(dsl.HTMLOptions{}))
		default:
			fmt.Println(r.Document().Text())
		}
		fmt.Println()
	}
	if len(results) == 0 && err == nil {
		return errors.New("not found")
	}
	return err
}

func suggest(ctx context.Context, lib *lengua.Library, args []string) error {
	fs := flag.NewFlagSet("suggest", flag.ExitOnError)
	selection := selectionFlags(fs)
	n := fs.Int("n", 20, "maximum number of suggestions")
	fs.Parse(args)
	opts, err := selection()
	if err != nil {
		return err
	}
	words, err := lib.Suggest(ctx, strings.Join(fs.Args(), " "), *n, opts)
	for _, w := range words {
		fmt.Println(w)
	}
	return err
}

func exportDSL(ctx context.Context, lib *lengua.Library, args []string) error {
	fs := flag.NewFlagSet("export-dsl", flag.ExitOnError)
	utf8 := fs.Bool("utf8", false, "write UTF-8 instead of UTF-16LE")
	dict := fs.String("dict", "", "export only this dictionary")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return errors.New("export-dsl: missing output directory")
	}
	outDir := fs.Arg(0)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	infos := lib.Dictionaries()
	if *dict != "" {
		info, err := lib.Info(*dict)
		if err != nil {
			return err
		}
		infos = []lengua.DictionaryInfo{info}
	}
	opts := lsd.ExportOptions{}
	if *utf8 {
		opts.Encoding = lsd.UTF8
	}
	for _, info := range infos {
		if err := ctx.Err(); err != nil {
			return err
		}
		d, err := lib.Dictionary(info.Name)
		if err != nil {
			return err
		}
		stem := fileStem(info.Path)
		fmt.Fprintf(os.Stderr, "%s -> %s.dsl\n", info.Name, stem)
		if err := writeDictionary(d, filepath.Join(outDir, stem), opts); err != nil {
			return err
		}
	}
	return nil
}

// writeDictionary writes stem.dsl plus the icon, annotation and embedded
// media files, mirroring the layout of ABBYY DSL Compiler sources.
func writeDictionary(d *lsd.Dictionary, stem string, opts lsd.ExportOptions) error {
	if icon := d.Icon(); len(icon) > 0 {
		opts.IconFile = filepath.Base(stem) + ".bmp"
		if err := os.WriteFile(stem+".bmp", icon, 0o644); err != nil {
			return err
		}
	}
	if ann, err := d.Annotation(); err != nil {
		return err
	} else if ann != "" {
		if err := os.WriteFile(stem+".ann", []byte(ann), 0o644); err != nil {
			return err
		}
	}
	for _, m := range d.Media() {
		data, err := d.ReadMedia(m.Name)
		if err != nil {
			return err
		}
		dir := stem + ".files"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(m.Name)), data, 0o644); err != nil {
			return err
		}
	}
	f, err := os.Create(stem + ".dsl")
	if err != nil {
		return err
	}
	if err := d.WriteDSL(f, opts); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func media(_ context.Context, lib *lengua.Library, args []string) error {
	fs := flag.NewFlagSet("media", flag.ExitOnError)
	dict := fs.String("dict", "", "dictionary that references the file")
	fs.Parse(args)
	if *dict == "" || fs.NArg() != 1 {
		return errors.New("media: usage: media -dict NAME FILE")
	}
	data, err := lib.Media(*dict, fs.Arg(0))
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

func exportSounds(args []string) error {
	if len(args) != 2 {
		return errors.New("export-sounds: usage: export-sounds ARCHIVE.lsa OUTDIR")
	}
	a, err := lsa.Open(args[0])
	if err != nil {
		return err
	}
	defer a.Close()
	if err := os.MkdirAll(args[1], 0o755); err != nil {
		return err
	}
	for i, e := range a.Entries() {
		wav, err := a.WAV(e.Name)
		if err != nil {
			return err
		}
		name := strings.NewReplacer("/", "_", `\`, "_").Replace(e.Name)
		if err := os.WriteFile(filepath.Join(args[1], name), wav, 0o644); err != nil {
			return err
		}
		if (i+1)%1000 == 0 {
			fmt.Fprintf(os.Stderr, "%d/%d\n", i+1, a.Len())
		}
	}
	return nil
}

func fileStem(path string) string {
	if path == "" {
		return "-"
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
