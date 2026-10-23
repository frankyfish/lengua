package dsl

import (
	"strings"
)

// Document is a parsed article: a well-formed tree of nodes.
type Document struct {
	Nodes []*Node
}

// Parse parses DSL article markup. It never fails: unknown tags and
// unbalanced brackets are kept as literal text, unclosed tags are closed
// at the end, and tags closed out of order are split and reopened so that
// the result is a proper tree. For example "[p][i]adj.[/p] x[/i]" yields
// p(i("adj.")) followed by i(" x").
//
// Backslash escapes (\[, \], \\ and others) are resolved into plain text.
func Parse(markup string) *Document {
	p := parser{root: &Node{}}
	p.stack = []*Node{p.root}
	p.run(markup)
	p.flush()
	nodes := normalize(p.root.Children)
	return &Document{Nodes: nodes}
}

type parser struct {
	root  *Node
	stack []*Node // open elements; stack[0] is the root
	text  strings.Builder
}

func (p *parser) top() *Node { return p.stack[len(p.stack)-1] }

func (p *parser) run(s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			if i+1 < len(s) {
				i++
			}
			p.text.WriteByte(s[i])
		case '[':
			end := tagEnd(s, i)
			if end < 0 || !p.tag(s[i+1:end]) {
				p.text.WriteByte(c)
				continue
			}
			i = end
		default:
			p.text.WriteByte(c)
		}
	}
}

// tagEnd returns the index of the ']' closing the tag that starts at
// s[start], or -1 if the bracket doesn't start a tag.
func tagEnd(s string, start int) int {
	for j := start + 1; j < len(s); j++ {
		switch s[j] {
		case ']':
			return j
		case '[', '\n', '\\':
			return -1
		}
	}
	return -1
}

// tag handles the content of a [...] tag and reports whether it was
// recognized. Unrecognized tags are left to the caller as literal text.
func (p *parser) tag(content string) bool {
	closing := strings.HasPrefix(content, "/")
	content = strings.TrimPrefix(content, "/")
	name, attr, _ := strings.Cut(content, " ")
	level := 0
	if len(name) == 2 && name[0] == 'm' && name[1] >= '0' && name[1] <= '9' {
		name, level = TagM, int(name[1]-'0')
	}
	if !knownTags[name] {
		return false
	}
	p.flush()
	if closing {
		p.close(name, false)
		return true
	}
	el := &Node{Tag: name, Attr: strings.TrimSpace(attr), Level: level}
	if name == TagM {
		// Paragraphs don't nest: a new one implicitly ends the previous
		// one, carrying over any inline formatting still open.
		reopen := p.close(TagM, true)
		p.push(el)
		for _, n := range reopen {
			p.push(n.shallowCopy())
		}
		return true
	}
	p.push(el)
	return true
}

func (p *parser) push(el *Node) {
	top := p.top()
	top.Children = append(top.Children, el)
	p.stack = append(p.stack, el)
}

// close ends the innermost open element named tag. Elements opened after
// it are closed too and, unless deferReopen is set, reopened right away;
// otherwise they are returned for the caller to reopen. Closing a tag that
// isn't open is a no-op.
func (p *parser) close(tag string, deferReopen bool) []*Node {
	j := len(p.stack) - 1
	for j > 0 && p.stack[j].Tag != tag {
		j--
	}
	if j == 0 {
		return nil
	}
	inner := append([]*Node(nil), p.stack[j+1:]...)
	p.stack = p.stack[:j]
	if deferReopen {
		return inner
	}
	for _, n := range inner {
		p.push(n.shallowCopy())
	}
	return nil
}

// flush appends pending text to the current element, merging adjacent
// text nodes.
func (p *parser) flush() {
	if p.text.Len() == 0 {
		return
	}
	text := p.text.String()
	p.text.Reset()
	top := p.top()
	if n := len(top.Children); n > 0 && top.Children[n-1].IsText() {
		top.Children[n-1].Content += text
		return
	}
	top.Children = append(top.Children, &Node{Content: text})
}

// normalize removes empty elements, which appear when reopened tags are
// closed immediately, and merges text nodes that become adjacent.
func normalize(nodes []*Node) []*Node {
	var out []*Node
	for _, n := range nodes {
		if !n.IsText() {
			n.Children = normalize(n.Children)
			if len(n.Children) == 0 {
				continue
			}
		} else if k := len(out); k > 0 && out[k-1].IsText() {
			out[k-1].Content += n.Content
			continue
		}
		out = append(out, n)
	}
	return out
}

// String returns the document as well-formed DSL markup with special
// characters escaped. Parsing the result yields an equal document.
func (d *Document) String() string {
	var sb strings.Builder
	for _, n := range d.Nodes {
		writeDSL(&sb, n)
	}
	return sb.String()
}

func writeDSL(sb *strings.Builder, n *Node) {
	if n.IsText() {
		sb.WriteString(Escape(n.Content))
		return
	}
	sb.WriteString(n.openTag())
	for _, c := range n.Children {
		writeDSL(sb, c)
	}
	sb.WriteString("[/" + n.Tag + "]")
}

// Escape escapes characters that have a special meaning in DSL text.
func Escape(s string) string {
	if !strings.ContainsAny(s, `[]\~@{}<>`) {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(`[]\~@{}<>`, s[i]) >= 0 {
			sb.WriteByte('\\')
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// Walk calls fn for every node in document order. If fn returns false, the
// children of that node are skipped.
func (d *Document) Walk(fn func(*Node) bool) {
	for _, n := range d.Nodes {
		n.Walk(fn)
	}
}

// Find returns the outermost elements with the given tag, in document
// order. Elements nested in a match are not returned separately.
func (d *Document) Find(tag string) []*Node {
	var out []*Node
	d.Walk(func(n *Node) bool {
		if n.Tag == tag {
			out = append(out, n)
			return false
		}
		return true
	})
	return out
}
