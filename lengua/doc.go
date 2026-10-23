// Package lengua provides access to the dictionaries of an ABBYY Lingvo X3
// installation: lookup across dictionaries, article markup, pronunciation
// sounds, pictures and abbreviation lists.
//
//	lib, err := lengua.Open("/path/to/L-X3")
//	if err != nil { ... }
//	defer lib.Close()
//
//	results, err := lib.Lookup(ctx, "serene", lengua.LookupOptions{
//		Source: lsd.English, Target: lsd.Russian,
//	})
//	for _, r := range results {
//		doc := r.Document()
//		fmt.Println(r.Dictionary.Name, doc.Translations())
//		for _, file := range doc.Media() {
//			wav, err := lib.Media(r.Dictionary.Name, file)
//			...
//		}
//	}
//
// The package builds on lower-level packages that can be used on their
// own: lsd (compiled dictionaries), lsa (sound archives) and dsl (article
// markup).
package lengua
