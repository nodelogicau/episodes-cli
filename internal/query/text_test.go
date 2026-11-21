package query

import (
	"reflect"
	"testing"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// recall-by-query 3.1, D1, and the upstream examples.
func TestTerms(t *testing.T) {
	cases := []struct {
		q, whole    string
		terms, raws []string
	}{
		{`"invoice?"  (Ada)`, `invoice?"  (Ada`, []string{"invoice", "Ada"}, []string{`"invoice?"`, "(Ada)"}},
		{"e-mail", "e-mail", []string{"e-mail"}, []string{"e-mail"}},
		{"timeout?", "timeout", []string{"timeout"}, []string{"timeout?"}},
		{"v1.2.3.", "v1.2.3", []string{"v1.2.3"}, []string{"v1.2.3."}},
		{"o'brien auth/session", "o'brien auth/session", []string{"o'brien", "auth/session"}, []string{"o'brien", "auth/session"}},
		// NBSP, U+2028 and U+3000 are White_Space.
		{" ada lovelace　", "ada lovelace", []string{"ada", "lovelace"}, []string{"ada", "lovelace"}},
		{"  ?! ", "", nil, nil},
		{"", "", nil, nil},
		{"invoice ?! receipt", "invoice ?! receipt", []string{"invoice", "receipt"}, []string{"invoice", "receipt"}},
	}
	for _, c := range cases {
		whole, terms, raws := Terms(c.q)
		if whole != c.whole || !reflect.DeepEqual(terms, c.terms) || !reflect.DeepEqual(raws, c.raws) {
			t.Errorf("Terms(%q) = %q %q %q, want %q %q %q", c.q, whole, terms, raws, c.whole, c.terms, c.raws)
		}
	}
	if !NewMatcher("  ?! ").Empty() || NewMatcher("a").Empty() {
		t.Fatal("Empty")
	}
}

func inline(id, label, text string) *model.Episode {
	return &model.Episode{ID: id, Label: label, Content: model.Content{Mode: model.ModeInline, Text: &text}}
}

func ref(id, label, uid string) *model.Episode {
	return &model.Episode{ID: id, Label: label, Content: model.Content{Mode: model.ModeRef, Ref: "gmail:1", UID: uid}}
}

// recall-by-query 3.2, D2.
func TestMatch(t *testing.T) {
	text := "the text of a ref"
	refWithText := ref("epi_r", "", "")
	refWithText.Content.Text = &text
	cases := []struct {
		name, q string
		ep      *model.Episode
		res     string
		matched int
	}{
		{"case-folded substring", "invoice", inline("epi_1", "", "Paid the INVOICE today"), ResolutionLexical, 1},
		{"full folding", "STRASSE", inline("epi_1", "", "die Straße"), ResolutionLexical, 1},
		{"full folding back", "straße", inline("epi_1", "", "STRASSE 5"), ResolutionLexical, 1},
		{"label", "invoice", ref("epi_1", "Invoice from Ada", ""), ResolutionLexical, 1},
		{"any term", "invoice receipt", inline("epi_1", "", "a receipt"), ResolutionLexical, 1},
		{"both terms", "invoice receipt", inline("epi_1", "invoice", "a receipt"), ResolutionLexical, 2},
		{"distinct under folding", "timeout Timeout timeout?", inline("epi_1", "", "the session timeout"), ResolutionLexical, 1},
		{"interior punctuation kept", "e-mail", inline("epi_1", "", "an email"), "", 0},
		{"no stemming", "dog's", inline("epi_1", "", "the dog barked"), "", 0},
		{"substring, not tokens", "auth/session", inline("epi_1", "", "see auth/session.go"), ResolutionLexical, 1},
		{"ref content not searched", "text", refWithText, "", 0},
		{"none mode text not searched", "x", &model.Episode{ID: "epi_n", Content: model.Content{Mode: model.ModeNone}}, "", 0},
		{"uid exact", "<abc@example.com>", ref("epi_1", "", "<abc@example.com>"), ResolutionUID, 1},
		{"uid after trimming", "mid:abc@example.com?", ref("epi_1", "", "mid:abc@example.com"), ResolutionUID, 1},
		{"uid not folded", "MID:ABC@EXAMPLE.COM", ref("epi_1", "", "mid:abc@example.com"), "", 0},
		{"uid not substring", "mid:abc", ref("epi_1", "", "mid:abc@example.com"), "", 0},
		{"uid wins over lexical", "mid:a@x invoice", ref("epi_1", "invoice", "mid:a@x"), ResolutionUID, 2},
		{"redacted label absent", "invoice", inline("epi_1", "", "nothing"), "", 0},
		{"empty", "?!", inline("epi_1", "?!", "?!"), "", 0},
	}
	for _, c := range cases {
		res, n := NewMatcher(c.q).Match(c.ep)
		if res != c.res || n != c.matched {
			t.Errorf("%s: Match(%q) = %q %d, want %q %d", c.name, c.q, res, n, c.res, c.matched)
		}
	}
}

// recall-by-query 3.3, D4.
func TestSortResults(t *testing.T) {
	r := func(id, res string, n int) Result {
		return Result{Episode: &model.Episode{ID: id}, Resolution: res, Matched: n}
	}
	rs := []Result{
		r("epi_1", ResolutionLexical, 1),
		r("epi_4", ResolutionLexical, 1),
		r("epi_2", ResolutionLexical, 2),
		r("epi_0", ResolutionUID, 1),
		r("epi_3", ResolutionUID, 1),
	}
	SortResults(rs)
	var got []string
	for _, x := range rs {
		got = append(got, x.Episode.ID)
	}
	if want := []string{"epi_3", "epi_0", "epi_2", "epi_4", "epi_1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}
