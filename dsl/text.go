package dsl

import "strings"

// Text renders the document as plain text: markup is removed, media file
// names are dropped and each [mN] paragraph is indented by N*2 spaces.
// Line breaks of the article are preserved.
func (d *Document) Text() string {
	var sb strings.Builder
	for _, n := range d.Nodes {
		writePlain(&sb, n)
	}
	return sb.String()
}

func writePlain(sb *strings.Builder, n *Node) {
	switch {
	case n.IsText():
		sb.WriteString(n.Content)
		return
	case n.Tag == TagMedia:
		return
	case n.Tag == TagM:
		sb.WriteString(strings.Repeat("  ", n.Level))
	}
	for _, c := range n.Children {
		writePlain(sb, c)
	}
}
