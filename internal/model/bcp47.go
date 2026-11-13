package model

import (
	"fmt"
	"strings"
)

// BCP 47 language tags, checked for well-formedness against the RFC 5646 §2.1
// ABNF, by hand, with no registry:
//
//	Language-Tag  = langtag / privateuse / grandfathered
//	langtag       = language ["-" script] ["-" region] *("-" variant)
//	                *("-" extension) ["-" privateuse]
//	language      = 2*3ALPHA ["-" extlang] / 4ALPHA / 5*8ALPHA
//	extlang       = 3ALPHA *2("-" 3ALPHA)
//	script        = 4ALPHA
//	region        = 2ALPHA / 3DIGIT
//	variant       = 5*8alphanum / (DIGIT 3alphanum)
//	extension     = singleton 1*("-" (2*8alphanum))
//	singleton     = DIGIT / %x41-57 / %x59-5A / %x61-77 / %x79-7A  ; not x
//	privateuse    = "x" 1*("-" (1*8alphanum))

// grandfathered tags, irregular and regular (RFC 5646 §2.1), in the case the
// registry records them.
var grandfathered = []string{
	"en-GB-oed", "i-ami", "i-bnn", "i-default", "i-enochian", "i-hak", "i-klingon",
	"i-lux", "i-mingo", "i-navajo", "i-pwn", "i-tao", "i-tay", "i-tsu",
	"sgn-BE-FR", "sgn-BE-NL", "sgn-CH-DE",
	"art-lojban", "cel-gaulish", "no-bok", "no-nyn", "zh-guoyu", "zh-hakka",
	"zh-min", "zh-min-nan", "zh-xiang",
}

func isLetter(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isAlpha(s string) bool {
	for _, c := range s {
		if !isLetter(c) {
			return false
		}
	}
	return s != ""
}

func isDigit(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func isAlnum(s string) bool {
	for _, c := range s {
		if !isLetter(c) && (c < '0' || c > '9') {
			return false
		}
	}
	return s != ""
}

func isGrandfathered(tag string) bool {
	for _, g := range grandfathered {
		if strings.EqualFold(tag, g) {
			return true
		}
	}
	return false
}

// WellFormedLanguage reports whether tag is a well-formed BCP 47 tag.
func WellFormedLanguage(tag string) bool {
	if isGrandfathered(tag) {
		return true
	}
	parts := strings.Split(tag, "-")
	for _, p := range parts {
		if !isAlnum(p) || len(p) > 8 {
			return false
		}
	}
	i := 0
	if strings.EqualFold(parts[0], "x") {
		return privateUse(parts)
	}
	// language
	l := parts[0]
	switch {
	case isAlpha(l) && len(l) >= 2 && len(l) <= 3:
		i = 1
		// up to three extlangs
		for n := 0; n < 3 && i < len(parts) && len(parts[i]) == 3 && isAlpha(parts[i]); n++ {
			i++
		}
	case isAlpha(l) && len(l) >= 4 && len(l) <= 8:
		i = 1
	default:
		return false
	}
	// script
	if i < len(parts) && len(parts[i]) == 4 && isAlpha(parts[i]) {
		i++
	}
	// region
	if i < len(parts) && (len(parts[i]) == 2 && isAlpha(parts[i]) || len(parts[i]) == 3 && isDigit(parts[i])) {
		i++
	}
	// variants
	for i < len(parts) {
		p := parts[i]
		if len(p) >= 5 && len(p) <= 8 || len(p) == 4 && p[0] >= '0' && p[0] <= '9' {
			i++
			continue
		}
		break
	}
	// extensions
	for i < len(parts) && len(parts[i]) == 1 && !strings.EqualFold(parts[i], "x") {
		i++
		n := 0
		for i < len(parts) && len(parts[i]) >= 2 {
			i++
			n++
		}
		if n == 0 {
			return false
		}
	}
	if i < len(parts) && strings.EqualFold(parts[i], "x") {
		return privateUse(parts[i:])
	}
	return i == len(parts)
}

func privateUse(parts []string) bool {
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts[1:] {
		if len(p) < 1 || len(p) > 8 {
			return false
		}
	}
	return true
}

// CanonicalLanguageCase returns tag in the case RFC 5646 §2.1.1 recommends:
// lowercase throughout, except that a four-letter subtag is titlecase and a
// two-letter subtag uppercase when it is not the first subtag and follows no
// singleton.
func CanonicalLanguageCase(tag string) string {
	parts := strings.Split(strings.ToLower(tag), "-")
	afterSingleton := false
	for i, p := range parts {
		if i == 0 {
			if len(p) == 1 {
				afterSingleton = true
			}
			continue
		}
		if afterSingleton {
			continue
		}
		switch {
		case len(p) == 1:
			afterSingleton = true
		case len(p) == 2 && isAlpha(p):
			parts[i] = strings.ToUpper(p)
		case len(p) == 4 && isAlpha(p):
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "-")
}

// SameLanguage compares tags without regard to case.
func SameLanguage(a, b string) bool { return strings.EqualFold(a, b) }

// LanguageProblem is why tag cannot be content.language: not well-formed, or
// und. A tag in the wrong case is not a problem for a reader.
func LanguageProblem(tag string) error {
	if strings.EqualFold(tag, "und") {
		return fmt.Errorf("language %q (undetermined) must not be written: leave out a language that is not known", tag)
	}
	if !WellFormedLanguage(tag) {
		return fmt.Errorf("language %q is not a well-formed BCP 47 tag (for example en, en-AU, zh-Hant-TW, or mul for several)", tag)
	}
	// The ABNF admits a primary language subtag of four to eight letters, but
	// four letters are reserved and none of five to eight is registered, so
	// such a tag (english) cannot name a language. This is the one place the
	// check goes beyond the grammar; see SPEC-FEEDBACK.md.
	if primary, _, _ := strings.Cut(tag, "-"); len(primary) >= 4 && isAlpha(primary) && !isGrandfathered(tag) {
		return fmt.Errorf("language %q has a primary subtag of %d letters, which no language has; use a two- or three-letter code (for example en)", tag, len(primary))
	}
	return nil
}
