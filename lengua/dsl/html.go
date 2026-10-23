package dsl

import (
	"html"
	"path"
	"strconv"
	"strings"
)

// HTMLOptions customize [Document.HTML]. The zero value renders a
// self-contained fragment without links or media.
type HTMLOptions struct {
	// RefURL returns the link target of a cross-reference to word, where
	// dict is the target dictionary name or empty for the current one.
	// If nil, references are rendered as plain styled text.
	RefURL func(word, dict string) string

	// MediaURL returns the URL of an embedded media file (a sound in an
	// .lsa archive or a picture in the .lsd file). If nil or if it returns
	// "", the media element is omitted.
	MediaURL func(name string) string

	// LabelTitle returns the expansion of a label abbreviation such as
	// "adj." (see lengua.Library.Abbreviation). It is used as the tooltip.
	LabelTitle func(label string) string

	// HideSecondary omits [*] content, matching Lingvo's compact view.
	HideSecondary bool
}

// DefaultCSS styles the classes produced by [Document.HTML] similarly to
// the Lingvo desktop application.
const DefaultCSS = `.dsl-m { margin: 0; }
.dsl-trn { }
.dsl-ex { color: #555; }
.dsl-com { color: #777; }
.dsl-p { color: green; font-style: italic; text-decoration: none; cursor: help; }
.dsl-t { font-family: "Lucida Sans Unicode", "Arial Unicode MS", sans-serif; }
.dsl-stress { text-decoration: underline; }
.dsl-ref { color: #0645ad; }
.dsl-sound { text-decoration: none; }
.dsl-img { max-width: 100%; }
`

// HTML renders the document as an HTML fragment. Elements get CSS classes
// of the form "dsl-<tag>" (see [DefaultCSS]); all text and attributes are
// escaped, so the output is safe to embed.
func (d *Document) HTML(opts HTMLOptions) string {
	r := htmlRenderer{opts: opts}
	r.children(d.Nodes)
	return r.sb.String()
}

type htmlRenderer struct {
	opts HTMLOptions
	sb   strings.Builder
}

func (r *htmlRenderer) children(nodes []*Node) {
	for i, n := range nodes {
		if !n.IsText() {
			r.element(n)
			continue
		}
		// Newlines next to paragraphs are implied by the block layout.
		text := n.Content
		if i > 0 && nodes[i-1].Tag == TagM {
			text = strings.TrimPrefix(text, "\n")
		}
		if i+1 < len(nodes) && nodes[i+1].Tag == TagM {
			text = strings.TrimSuffix(text, "\n")
		}
		r.sb.WriteString(strings.ReplaceAll(html.EscapeString(text), "\n", "<br>"))
	}
}

func (r *htmlRenderer) wrap(open, close string, n *Node) {
	r.sb.WriteString(open)
	r.children(n.Children)
	r.sb.WriteString(close)
}

func (r *htmlRenderer) element(n *Node) {
	switch n.Tag {
	case TagM:
		r.wrap(`<div class="dsl-m" style="margin-left:`+strconv.Itoa(n.Level)+`em">`, "</div>", n)
	case TagBold, TagItalic, TagUnderline, TagSub, TagSup:
		r.wrap("<"+n.Tag+">", "</"+n.Tag+">", n)
	case TagColor:
		r.wrap(`<span class="dsl-c" style="color:`+cssColor(n.Attr)+`">`, "</span>", n)
	case TagLabel:
		title := ""
		if r.opts.LabelTitle != nil {
			if t := r.opts.LabelTitle(n.Text()); t != "" {
				title = ` title="` + html.EscapeString(t) + `"`
			}
		}
		r.wrap(`<abbr class="dsl-p"`+title+`>`, "</abbr>", n)
	case TagSecondary:
		if !r.opts.HideSecondary {
			r.wrap(`<span class="dsl-opt">`, "</span>", n)
		}
	case TagRef:
		r.link("dsl-ref", r.refURL(n), n)
	case TagURL:
		r.link("dsl-url", safeURL(n.Text()), n)
	case TagMedia, "video":
		r.media(n.Text())
	default:
		r.wrap(`<span class="dsl-`+className(n.Tag)+`">`, "</span>", n)
	}
}

func (r *htmlRenderer) refURL(n *Node) string {
	if r.opts.RefURL == nil {
		return ""
	}
	dict, _ := n.Attribute("dict")
	return r.opts.RefURL(n.Text(), dict)
}

func (r *htmlRenderer) link(class, href string, n *Node) {
	if href == "" {
		r.wrap(`<span class="`+class+`">`, "</span>", n)
		return
	}
	r.wrap(`<a class="`+class+`" href="`+html.EscapeString(href)+`">`, "</a>", n)
}

func (r *htmlRenderer) media(name string) {
	if r.opts.MediaURL == nil {
		return
	}
	src := r.opts.MediaURL(name)
	if src == "" {
		return
	}
	src, alt := html.EscapeString(src), html.EscapeString(name)
	if IsImage(name) {
		r.sb.WriteString(`<img class="dsl-img" src="` + src + `" alt="` + alt + `">`)
		return
	}
	r.sb.WriteString(`<a class="dsl-sound" href="` + src + `" title="` + alt + `">&#128264;</a>`)
}

// IsImage reports whether a media file name refers to a picture rather
// than a sound.
func IsImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp":
		return true
	}
	return false
}

// className maps tags to CSS-safe class suffixes.
func className(tag string) string {
	switch tag {
	case TagNoSearch:
		return "notrs"
	case TagStress:
		return "stress"
	}
	return tag
}

// cssColor validates a DSL color name; [c] without a name is green.
func cssColor(name string) string {
	if name == "" {
		return "green"
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '#') {
			return "green"
		}
	}
	return name
}

// safeURL allows only web and mail links from [url] tags.
func safeURL(u string) string {
	u = strings.TrimSpace(u)
	lower := strings.ToLower(u)
	for _, scheme := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(lower, scheme) {
			return u
		}
	}
	if strings.HasPrefix(lower, "www.") {
		return "http://" + u
	}
	return ""
}
