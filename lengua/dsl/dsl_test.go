package dsl

import (
	"reflect"
	"strings"
	"testing"
)

// serene is a shortened real article from LingvoUniversal (En-Ru).
const serene = `\[[t]sɪ'riːn[/t]\] [s]serene.wav[/s]
[b]1.[/b] [p]прил.[/p]
[m1]1) [trn]ясный; безоблачный[/trn][/m]
[m2][*][ex][lang id=1033]serene weather[/lang] — безветренная, ясная погода[/ex][/*][/m]
[m2][*][ref]calm[/ref] 1., [ref dict="LingvoGrammar (En-Ru)"]clear[/ref][/*][/m]
[b]2.[/b] [p][i][c][com]сущ.[/i][/p]; [p][i]поэт.[/com][/c][/i][/p]`

func TestParseRepairsNesting(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"[b]x[/b]", "[b]x[/b]"},
		{"[p][i]adj.[/p] x[/i]", "[p][i]adj.[/i][/p][i] x[/i]"},
		{"[b]unclosed", "[b]unclosed[/b]"},
		{"stray[/b] close", "stray close"},
		{"[c red]x[/c]", "[c red]x[/c]"},
		{"[m1]a[/m]\n[m2]b[/m]", "[m1]a[/m]\n[m2]b[/m]"},
		{"[m1][b]a\n[m2]b[/b][/m]", "[m1][b]a\n[/b][/m][m2][b]b[/b][/m]"},
		{"[unknown]x", `\[unknown\]x`},
		{`\[esc\] \\ [b]\[[/b]`, `\[esc\] \\ [b]\[[/b]`},
		{"[open bracket", `\[open bracket`},
		{"[b][/b]", ""},
	}
	for _, tt := range tests {
		if got := Parse(tt.in).String(); got != tt.want {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRoundTripIsStable(t *testing.T) {
	once := Parse(serene)
	twice := Parse(once.String())
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("round trip changed the document:\n%s\n%s", once, twice)
	}
}

func TestParagraphLevel(t *testing.T) {
	doc := Parse("[m3]x[/m]")
	if n := doc.Nodes[0]; n.Tag != TagM || n.Level != 3 {
		t.Errorf("got tag %q level %d, want m 3", n.Tag, n.Level)
	}
}

func TestAttribute(t *testing.T) {
	tests := []struct {
		attr, key, want string
		ok              bool
	}{
		{"id=1033", "id", "1033", true},
		{`dict="LingvoGrammar (En-Ru)"`, "dict", "LingvoGrammar (En-Ru)", true},
		{`name="English" id=1`, "id", "1", true},
		{"red", "id", "", false},
		{"", "id", "", false},
	}
	for _, tt := range tests {
		n := &Node{Tag: "x", Attr: tt.attr}
		got, ok := n.Attribute(tt.key)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Attribute(%q) on %q = %q, %v; want %q, %v", tt.key, tt.attr, got, ok, tt.want, tt.ok)
		}
	}
}

func TestText(t *testing.T) {
	got := Parse(serene).Text()
	for _, want := range []string{"[sɪ'riːn]", "  1) ясный; безоблачный", "    serene weather — безветренная"} {
		if !strings.Contains(got, want) {
			t.Errorf("Text() = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "serene.wav") {
		t.Errorf("Text() contains a media file name: %q", got)
	}
}

func TestExtractors(t *testing.T) {
	doc := Parse(serene)
	if got, want := doc.Translations(), []string{"ясный; безоблачный"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Translations() = %q, want %q", got, want)
	}
	if got, want := doc.Transcriptions(), []string{"sɪ'riːn"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Transcriptions() = %q, want %q", got, want)
	}
	if got, want := doc.Media(), []string{"serene.wav"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Media() = %q, want %q", got, want)
	}
	if got, want := doc.Labels(), []string{"прил.", "сущ.", "поэт."}; !reflect.DeepEqual(got, want) {
		t.Errorf("Labels() = %q, want %q", got, want)
	}
	wantEx := []Example{{
		Text:        "serene weather — безветренная, ясная погода",
		Original:    "serene weather",
		Translation: "безветренная, ясная погода",
	}}
	if got := doc.Examples(); !reflect.DeepEqual(got, wantEx) {
		t.Errorf("Examples() = %+v, want %+v", got, wantEx)
	}
	wantRefs := []Reference{{Word: "calm"}, {Word: "clear", Dictionary: "LingvoGrammar (En-Ru)"}}
	if got := doc.References(); !reflect.DeepEqual(got, wantRefs) {
		t.Errorf("References() = %+v, want %+v", got, wantRefs)
	}
}

func TestHTML(t *testing.T) {
	opts := HTMLOptions{
		RefURL:   func(word, dict string) string { return "/w/" + word },
		MediaURL: func(name string) string { return "/m/" + name },
		LabelTitle: func(label string) string {
			return map[string]string{"прил.": "прилагательное"}[label]
		},
	}
	got := Parse(serene).HTML(opts)
	for _, want := range []string{
		`<div class="dsl-m" style="margin-left:1em">1) <span class="dsl-trn">ясный; безоблачный</span></div>`,
		`<a class="dsl-sound" href="/m/serene.wav" title="serene.wav">`,
		`<a class="dsl-ref" href="/w/calm">calm</a>`,
		`<abbr class="dsl-p" title="прилагательное">прил.</abbr>`,
		`<span class="dsl-t">sɪ&#39;riːn</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML() missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "</div><br>") {
		t.Errorf("HTML() has line breaks between paragraphs:\n%s", got)
	}
}

func TestHTMLEscapesUntrustedContent(t *testing.T) {
	got := Parse(`<script>x</script> [c red;background:url(x)]a[/c] [url]javascript:alert(1)[/url]`).HTML(HTMLOptions{})
	for _, bad := range []string{"<script>", "background", `href="javascript`} {
		if strings.Contains(got, bad) {
			t.Errorf("HTML() contains %q:\n%s", bad, got)
		}
	}
}

func TestHideSecondary(t *testing.T) {
	got := Parse("a[*]b[/*]").HTML(HTMLOptions{HideSecondary: true})
	if got != "a" {
		t.Errorf("HTML() = %q, want %q", got, "a")
	}
}

func FuzzParse(f *testing.F) {
	f.Add(serene)
	f.Add("[m1][b]a\n[m2]b[/b][/m]")
	f.Fuzz(func(t *testing.T, s string) {
		once := Parse(s)
		if twice := Parse(once.String()); !reflect.DeepEqual(once, twice) {
			t.Errorf("unstable round trip for %q: %q vs %q", s, once, twice)
		}
		_ = once.HTML(HTMLOptions{})
		_ = once.Text()
	})
}
