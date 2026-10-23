package dsl

import (
	"strconv"
	"strings"
)

// Tag names used by Lingvo dictionaries. Paragraph tags [m0]..[m9] are
// all represented by TagM with the level stored in Node.Level.
const (
	TagM           = "m"
	TagTranslation = "trn"
	TagNoSearch    = "!trs"
	TagExample     = "ex"
	TagComment     = "com"
	TagLabel       = "p"
	TagTranscript  = "t"
	TagMedia       = "s"
	TagRef         = "ref"
	TagURL         = "url"
	TagLang        = "lang"
	TagColor       = "c"
	TagSecondary   = "*"
	TagStress      = "'"
	TagBold        = "b"
	TagItalic      = "i"
	TagUnderline   = "u"
	TagSub         = "sub"
	TagSup         = "sup"
)

// knownTags lists the tags the parser recognizes. Anything else in square
// brackets is kept as literal text, since Lingvo itself does the same.
var knownTags = map[string]bool{
	TagM: true, TagTranslation: true, TagNoSearch: true, TagExample: true,
	TagComment: true, TagLabel: true, TagTranscript: true, TagMedia: true,
	TagRef: true, TagURL: true, TagLang: true, TagColor: true,
	TagSecondary: true, TagStress: true, TagBold: true, TagItalic: true,
	TagUnderline: true, TagSub: true, TagSup: true,
	"trn1": true, "video": true, "preview": true,
}

// Node is an element or a text run in a parsed article.
type Node struct {
	// Tag is the element name, e.g. "trn" or "m". It is empty for text.
	Tag string
	// Attr is the raw attribute string of the opening tag, e.g. "id=1033"
	// for [lang id=1033] or "red" for [c red]. Use Attribute to parse it.
	Attr string
	// Level is the indentation level of an [mN] paragraph (0 for [m]).
	Level int
	// Content holds the unescaped content of a text node.
	Content string
	// Children are the nested nodes of an element.
	Children []*Node
}

// IsText reports whether n is a text node.
func (n *Node) IsText() bool { return n.Tag == "" }

// Attribute returns the value of key in the node attributes, so that
// Attribute("dict") is "Some Dict" for [ref dict="Some Dict"]. The quotes
// are optional in DSL. ok is false if the key is absent.
func (n *Node) Attribute(key string) (value string, ok bool) {
	rest := n.Attr
	for rest != "" {
		rest = strings.TrimLeft(rest, " ")
		k, after, found := strings.Cut(rest, "=")
		if !found {
			return "", false
		}
		k = strings.TrimSpace(k)
		var v string
		if strings.HasPrefix(after, `"`) {
			v, rest, _ = strings.Cut(after[1:], `"`)
		} else {
			v, rest, _ = strings.Cut(after, " ")
		}
		if k == key {
			return v, true
		}
	}
	return "", false
}

// Text returns the plain text content of the node and its descendants.
// Media file names ([s] elements) nested in the node are omitted.
func (n *Node) Text() string {
	if n.IsText() {
		return n.Content
	}
	var sb strings.Builder
	for _, c := range n.Children {
		c.writeText(&sb)
	}
	return sb.String()
}

func (n *Node) writeText(sb *strings.Builder) {
	switch {
	case n.IsText():
		sb.WriteString(n.Content)
	case n.Tag == TagMedia:
		// file names aren't readable content
	default:
		for _, c := range n.Children {
			c.writeText(sb)
		}
	}
}

// Walk calls fn for n and its descendants in document order. If fn returns
// false, the children of that node are skipped.
func (n *Node) Walk(fn func(*Node) bool) {
	if !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// openTag returns the DSL opening tag, e.g. "[m2]" or "[lang id=1033]".
func (n *Node) openTag() string {
	name := n.Tag
	if n.Tag == TagM && n.Level > 0 {
		name += strconv.Itoa(n.Level)
	}
	if n.Attr != "" {
		name += " " + n.Attr
	}
	return "[" + name + "]"
}

// shallowCopy returns an element with the same tag and attributes but no
// children; used when the parser reopens a tag closed out of order.
func (n *Node) shallowCopy() *Node {
	return &Node{Tag: n.Tag, Attr: n.Attr, Level: n.Level}
}
