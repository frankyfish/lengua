// Package dsl parses and renders article markup in the ABBYY DSL format,
// as returned by lsd.Dictionary.Article.
//
// DSL markup uses bracketed tags such as [b]bold[/b], [trn]translation[/trn]
// or [m1]indented line[/m]. Lingvo dictionaries frequently close tags in the
// wrong order (e.g. "[p][i]adj.[/p][/i]"); [Parse] repairs such markup into a
// proper tree, so renderers and extractors never see overlapping elements.
//
//	doc := dsl.Parse(article)
//	fmt.Println(doc.Text())            // plain text
//	html := doc.HTML(dsl.HTMLOptions{}) // HTML fragment
//	for _, t := range doc.Translations() { ... }
//
// The most important tags are:
//
//	[m], [m0]..[m9]  paragraph with indentation level, closed by [/m]
//	[trn]            translation
//	[!trs]           text excluded from full-text search
//	[ex]             example usage
//	[com]            comment
//	[p]              label (abbreviation such as "adj."), see *_abrv.lsd
//	[t]              phonetic transcription
//	[s]              media file name (sound or picture)
//	[ref]            cross-reference to another heading, optional dict="..."
//	[url]            external link
//	[lang]           language of the enclosed text: id=1033 or name="English"
//	[c]              colored text, optional color name: [c red]
//	[*]              secondary content, shown only in the full view
//	[']              stressed vowel
//	[b] [i] [u] [sub] [sup]  formatting
package dsl
