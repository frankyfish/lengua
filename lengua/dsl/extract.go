package dsl

import "strings"

// Example is a usage example from an [ex] element.
type Example struct {
	// Text is the whole example, e.g. "serene weather — ясная погода".
	Text string
	// Original is the part marked with [lang], usually the source-language
	// phrase ("serene weather"). It is empty if the example has no [lang].
	Original string
	// Translation is the rest of the text after Original, with the leading
	// dash removed ("ясная погода").
	Translation string
}

// Reference is a cross-reference from a [ref] element.
type Reference struct {
	Word       string // heading to look up
	Dictionary string // target dictionary name; empty means the same one
}

// Translations returns the text of all [trn] elements.
func (d *Document) Translations() []string { return d.texts(TagTranslation) }

// Transcriptions returns the phonetic transcriptions ([t] elements).
func (d *Document) Transcriptions() []string { return d.texts(TagTranscript) }

// Labels returns the grammatical and usage labels ([p] elements), such as
// "adj." or "colloq.". The abbreviation dictionaries expand them.
func (d *Document) Labels() []string { return d.texts(TagLabel) }

// Media returns the names of referenced media files ([s] elements), e.g.
// "serene.wav". Use [IsImage] to distinguish pictures from sounds.
func (d *Document) Media() []string { return d.texts(TagMedia) }

// Examples returns the usage examples ([ex] elements).
func (d *Document) Examples() []Example {
	var out []Example
	for _, n := range d.Find(TagExample) {
		ex := Example{Text: strings.TrimSpace(n.Text())}
		var lang *Node
		n.Walk(func(c *Node) bool {
			if lang == nil && c.Tag == TagLang {
				lang = c
			}
			return lang == nil
		})
		if lang != nil {
			ex.Original = strings.TrimSpace(lang.Text())
			if rest, ok := strings.CutPrefix(ex.Text, ex.Original); ok {
				ex.Translation = strings.TrimSpace(strings.TrimLeft(rest, " -—–"))
			}
		}
		out = append(out, ex)
	}
	return out
}

// References returns the cross-references ([ref] elements).
func (d *Document) References() []Reference {
	var out []Reference
	for _, n := range d.Find(TagRef) {
		dict, _ := n.Attribute("dict")
		out = append(out, Reference{Word: strings.TrimSpace(n.Text()), Dictionary: dict})
	}
	return out
}

// texts returns the trimmed, non-empty text of the outermost elements
// with the given tag.
func (d *Document) texts(tag string) []string {
	var out []string
	for _, n := range d.Find(tag) {
		var s string
		if tag == TagMedia {
			s = mediaName(n)
		} else {
			s = n.Text()
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// mediaName returns the file name in an [s] element. Node.Text skips
// nested media, so the text is collected directly.
func mediaName(n *Node) string {
	var sb strings.Builder
	n.Walk(func(c *Node) bool {
		sb.WriteString(c.Content)
		return true
	})
	return sb.String()
}
