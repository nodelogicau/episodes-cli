// Recall by query: the model-free baseline (change recall-by-query, #16).

package query

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/cases"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// Resolutions, in result order.
const (
	ResolutionUID     = "uid"
	ResolutionLexical = "lexical"
)

// fold is Unicode full case folding, so ß matches SS (design D1). It is the
// one place to change if the format turns out to mean simple folding.
func fold(s string) string { return cases.Fold().String(s) }

func isWhiteSpace(r rune) bool { return unicode.Is(unicode.White_Space, r) }

// trimPunct strips leading and trailing runes of general category P.
func trimPunct(s string) string { return strings.TrimFunc(s, unicode.IsPunct) }

// Terms normalises a query (design D1). whole is the query with White_Space
// trimmed from its edges and then category P; terms are its runs of
// non-White_Space with category P trimmed from their edges, empty ones
// dropped; raw[i] is terms[i] as written, before that trim. Nothing is
// folded. A query whose whole and every term are empty resolves nothing.
func Terms(q string) (whole string, terms, raw []string) {
	whole = trimPunct(strings.TrimFunc(q, isWhiteSpace))
	for _, r := range strings.FieldsFunc(q, isWhiteSpace) {
		if t := trimPunct(r); t != "" {
			terms = append(terms, t)
			raw = append(raw, r)
		}
	}
	return whole, terms, raw
}

// Matcher is one query, normalised and folded, to match against episodes.
type Matcher struct {
	whole string // folded
	// groups are the distinct folded terms, each with the forms (raw and
	// trimmed, unfolded) that may equal a uid.
	groups []termGroup
}

type termGroup struct {
	folded string
	uids   []string
}

// NewMatcher normalises q by Terms and folds it.
func NewMatcher(q string) *Matcher {
	whole, terms, raw := Terms(q)
	m := &Matcher{whole: fold(whole)}
	at := map[string]int{}
	for i, t := range terms {
		f := fold(t)
		g, ok := at[f]
		if !ok {
			g = len(m.groups)
			at[f] = g
			m.groups = append(m.groups, termGroup{folded: f})
		}
		m.groups[g].uids = append(m.groups[g].uids, raw[i], t)
	}
	return m
}

// Empty reports a query that resolves nothing.
func (m *Matcher) Empty() bool { return m.whole == "" && len(m.groups) == 0 }

// Match reports how ep resolves (design D2): ResolutionUID when a term,
// before or after trimming, equals its content.uid exactly;
// ResolutionLexical when the folded whole query or a folded term occurs in
// its folded label or, if inline, its content.text; "" when neither. matched
// is the number of distinct folded terms that hit either way. Only fields
// the decoded episode carries are searched, so a redacted one never
// matches.
func (m *Matcher) Match(ep *model.Episode) (resolution string, matched int) {
	if m.Empty() {
		return "", 0
	}
	var texts []string
	if ep.Label != "" {
		texts = append(texts, fold(ep.Label))
	}
	if ep.Content.Mode == model.ModeInline && ep.Content.Text != nil {
		texts = append(texts, fold(*ep.Content.Text))
	}
	in := func(s string) bool {
		for _, t := range texts {
			if strings.Contains(t, s) {
				return true
			}
		}
		return false
	}
	lexical := m.whole != "" && in(m.whole)
	uid := false
	for _, g := range m.groups {
		hit := false
		if ep.Content.UID != "" {
			for _, u := range g.uids {
				if u == ep.Content.UID {
					hit, uid = true, true
					break
				}
			}
		}
		if in(g.folded) {
			hit, lexical = true, true
		}
		if hit {
			matched++
		}
	}
	switch {
	case uid:
		return ResolutionUID, matched
	case lexical:
		return ResolutionLexical, matched
	}
	return "", 0
}

// Result is one episode a query resolved.
type Result struct {
	Episode    *model.Episode
	Resolution string
	Matched    int
}

// SortResults orders results by resolution (uid, then lexical), then by
// distinct terms matched, most first, then by id descending by code point
// (design D4). Go compares strings bytewise, which for UTF-8 is code-point
// order.
func SortResults(rs []Result) {
	rank := func(r string) int {
		if r == ResolutionUID {
			return 0
		}
		return 1
	}
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		switch {
		case rank(a.Resolution) != rank(b.Resolution):
			return rank(a.Resolution) < rank(b.Resolution)
		case a.Matched != b.Matched:
			return a.Matched > b.Matched
		}
		return a.Episode.ID > b.Episode.ID
	})
}
