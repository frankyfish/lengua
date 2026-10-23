package lsd

import (
	"strconv"
	"strings"
)

// Language is an ABBYY language code. For most languages it equals the
// Windows locale identifier (LCID), e.g. 1033 for English and 1049 for
// Russian; ABBYY added private codes for languages Windows doesn't know.
type Language uint16

// Language codes that occur in the Lingvo X3 dictionary collection.
//
// Note that German dictionaries mostly use GermanNewSpelling, Portuguese
// ones use PortugueseStandard and Latin ones use Latin (ABBYY's legacy code).
const (
	Chinese            Language = 1028
	German             Language = 1031
	English            Language = 1033
	Spanish            Language = 1034
	Finnish            Language = 1035
	French             Language = 1036
	Italian            Language = 1040
	Polish             Language = 1045
	Russian            Language = 1049
	Turkish            Language = 1055
	Ukrainian          Language = 1058
	Kazakh             Language = 1087
	Tatar              Language = 1092
	Latin              Language = 1540
	PortugueseStandard Language = 2070
	GermanNewSpelling  Language = 32775
)

// String returns the English name of the language (as used in DSL
// #INDEX_LANGUAGE headers) or the numeric code if the language is unknown.
func (l Language) String() string {
	if name, ok := languageNames[l]; ok {
		return name
	}
	return strconv.Itoa(int(l))
}

// Primary returns the language with spelling and domain variants folded,
// so that GermanNewSpelling (and its Law and Medical variants) become
// German and PortugueseStandard becomes Portuguese. Use it to compare
// languages: lsd.ParseLanguage("de") returns German, while German
// dictionaries are tagged GermanNewSpelling.
func (l Language) Primary() Language {
	switch l {
	case GermanNewSpelling, 35847, 37895:
		return German
	case PortugueseStandard:
		return 1046
	}
	return l
}

// isoCodes maps ISO 639-1 codes to the languages of Lingvo X3.
var isoCodes = map[string]Language{
	"zh": Chinese, "de": German, "en": English, "es": Spanish,
	"fi": Finnish, "fr": French, "it": Italian, "pl": Polish,
	"ru": Russian, "tr": Turkish, "uk": Ukrainian, "kk": Kazakh,
	"tt": Tatar, "la": Latin, "pt": 1046,
}

// ParseLanguage resolves a language by its decimal code ("1033"), its
// ISO 639-1 code ("en") or its English name ("english"), ignoring case.
// If several codes share a name, the lowest code is returned.
func ParseLanguage(s string) (Language, bool) {
	if n, err := strconv.ParseUint(s, 10, 16); err == nil {
		return Language(n), true
	}
	if l, ok := isoCodes[strings.ToLower(s)]; ok {
		return l, true
	}
	var found Language
	for code, name := range languageNames {
		if strings.EqualFold(name, s) && (found == 0 || code < found) {
			found = code
		}
	}
	return found, found != 0
}
