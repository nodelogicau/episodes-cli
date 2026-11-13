package model

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func decodeFile(t *testing.T, name string) (*Episode, []byte) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	e, probs, err := Decode(b)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(probs) > 0 {
		t.Fatalf("%s: decode problems %v", name, probs)
	}
	return e, b
}

func codes(ps Problems, sev Severity) []string {
	var out []string
	for _, p := range ps {
		if sev == "" || p.Severity == sev {
			out = append(out, p.Code)
		}
	}
	sort.Strings(out)
	return out
}

func has(ps Problems, code string, sev Severity) bool {
	for _, p := range ps {
		if p.Code == code && (sev == "" || p.Severity == sev) {
			return true
		}
	}
	return false
}

// object-format: the spec's examples round-trip byte-identically.
func TestGoldenRoundTrip(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.yaml")
	if len(files) < 6 {
		t.Fatalf("golden files missing: %v", files)
	}
	for _, f := range files {
		name := filepath.Base(f)
		e, in := decodeFile(t, name)
		out, err := Encode(e)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(in, out) {
			t.Errorf("%s does not round-trip:\n--- in\n%s--- out\n%s", name, in, out)
		}
		if errs := Check(e, ForValidate).Errors(); len(errs) > 0 {
			t.Errorf("%s: %v", name, errs)
		}
	}
}

// object-format: reordered input re-emits canonically.
func TestReorderedInput(t *testing.T) {
	_, in := decodeFile(t, "readme-message.yaml")
	// Reverse the top-level blocks.
	var blocks []string
	for _, line := range strings.SplitAfter(string(in), "\n") {
		if line == "" {
			continue
		}
		if line[0] != ' ' || len(blocks) == 0 {
			blocks = append(blocks, line)
		} else {
			blocks[len(blocks)-1] += line
		}
	}
	for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}
	rev := strings.Join(blocks, "")
	if rev == string(in) {
		t.Fatal("reversal did nothing")
	}
	e, probs, err := Decode([]byte(rev))
	if err != nil || len(probs) > 0 {
		t.Fatal(err, probs)
	}
	out, _ := Encode(e)
	if !bytes.Equal(out, in) {
		t.Errorf("reordered input not re-emitted canonically:\n%s", out)
	}
}

// object-format: a leftover occasion is kept and given no meaning.
func TestExtrasKept(t *testing.T) {
	e, in := decodeFile(t, "redacted-turn.yaml")
	if len(e.Extra) != 1 || e.Extra[0].Key != "occasion" {
		t.Fatalf("extras %v", e.Extra.Keys())
	}
	out, _ := Encode(e)
	if !bytes.Contains(out, []byte("\noccasion: int_X\nredacted:")) || !bytes.Equal(in, out) {
		t.Errorf("occasion not kept before redacted:\n%s", out)
	}
}

func TestDecodeRejects(t *testing.T) {
	for name, doc := range map[string]string{
		"duplicate key":        "id: epi_a\nid: epi_b\n",
		"duplicate nested key": "content:\n  mode: inline\n  mode: ref\n",
		"not a mapping":        "- a\n- b\n",
		"scalar":               "hello\n",
		"bad yaml":             "a: [\n",
		"two docs":             "a: 1\n---\nb: 2\n",
		"empty":                "",
	} {
		if _, _, err := Decode([]byte(doc)); !errors.Is(err, ErrUnparseable) {
			t.Errorf("%s: err = %v, want ErrUnparseable", name, err)
		}
	}
	_, probs, err := Decode([]byte("ordinal: seven\nparticipants: x\n"))
	if err != nil || len(probs) != 2 {
		t.Errorf("type problems: %v %v", err, probs)
	}
}

func TestEncodeDeterministic(t *testing.T) {
	e, _ := decodeFile(t, "readme-act.yaml")
	e.Participants = []string{"b", "a", "b"}
	e.Links = append(e.Links, Link{Rel: "about", Href: "https://example.com"}, e.Links[0])
	a, _ := Encode(e)
	b, _ := Encode(e.Clone())
	if !bytes.Equal(a, b) {
		t.Fatal("two encodes differ")
	}
	s := string(a)
	if !strings.Contains(s, "participants:\n  - a\n  - b\nlabel") {
		t.Errorf("participants not sorted and de-duplicated:\n%s", s)
	}
	if strings.Index(s, "rel: about") > strings.Index(s, "rel: via") || strings.Count(s, "rel: via") != 1 {
		t.Errorf("links not sorted and de-duplicated:\n%s", s)
	}
	if strings.HasPrefix(s, "---") || strings.Contains(s, "\n...") {
		t.Error("document markers written")
	}
}

func TestQuotedOccurredInFlow(t *testing.T) {
	n := OccurredNode(Occurred{Start: "2026-11-07T14:24:00+11:00[Australia/Melbourne]", Basis: "recorded"}, true)
	out, _ := EncodeNode(n)
	if string(out) != "{start: \"2026-11-07T14:24:00+11:00[Australia/Melbourne]\", basis: recorded}\n" {
		t.Fatalf("%q", out)
	}
	// occurred-time (upstream): a YAML parser reads start as one string.
	e, probs, err := Decode([]byte("occurred: " + string(out)))
	if err != nil || len(probs) > 0 || e.Occurred.Start != "2026-11-07T14:24:00+11:00[Australia/Melbourne]" {
		t.Fatal(e, probs, err)
	}
}

func base() *Episode {
	text := "hello\n"
	return &Episode{
		ID: "epi_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37", Type: TypeEpisode, Kind: "message",
		Occurred:   Occurred{Start: "2026-11-07T14:24:00+11:00[Australia/Melbourne]", Basis: BasisRecorded},
		Standpoint: "https://github.com/benfortuna",
		Content:    Content{Mode: ModeInline, MediaType: DefaultMediaType, Derivation: DerivationVerbatim, Text: &text},
		Source:     Source{Author: "https://github.com/benfortuna"},
		Timestamp:  "2026-11-07T03:24:40Z",
	}
}

func TestRules(t *testing.T) {
	cases := []struct {
		name string
		mut  func(e *Episode)
		code string
		sev  Severity
		mode Mode
	}{
		{"unknown kind ok", func(e *Episode) { e.Kind = "voicememo" }, "", "", ForValidate},
		{"malformed kind", func(e *Episode) { e.Kind = "Phone Call" }, "invalid_kind", SevError, ForWrite},
		{"missing standpoint", func(e *Episode) { e.Standpoint = "" }, "missing_field", SevError, ForValidate},
		{"bad type", func(e *Episode) { e.Type = "claim" }, "invalid_type", SevError, ForValidate},
		{"planned basis", func(e *Episode) { e.Occurred.Basis = "planned" }, "invalid_basis", SevError, ForValidate},
		{"mixed forms", func(e *Episode) { e.Occurred.Start = "2026-11-03"; e.Occurred.End = "2026-11-03T10:00:00Z" }, "occurred_mixed_forms", SevError, ForValidate},
		{"tz beside datetime", func(e *Episode) { e.Occurred.TZ = "Australia/Melbourne" }, "tz_beside_datetime", SevError, ForValidate},
		{"critical suffix", func(e *Episode) { e.Occurred.Start = "2026-11-07T14:24:00+11:00[!Australia/Melbourne]" }, "critical_suffix", SevError, ForValidate},
		{"elective ignored on read", func(e *Episode) {
			e.Occurred.Start = "2026-11-07T14:24:00+11:00[Australia/Melbourne][u-ca=gregory]"
		}, "", "", ForValidate},
		{"elective refused on write", func(e *Episode) { e.Occurred.Start = "2026-11-07T14:24:00+11:00[u-ca=gregory]" }, "suffix_refused", SevError, ForWrite},
		{"zone mismatch warns", func(e *Episode) { e.Occurred.Start = "2026-11-07T14:24:00+10:00[Australia/Melbourne]" }, "zone_offset_mismatch", SevWarning, ForValidate},
		{"zone mismatch refused", func(e *Episode) { e.Occurred.Start = "2026-11-07T14:24:00+10:00[Australia/Melbourne]" }, "zone_offset_mismatch", SevError, ForWrite},
		{"after recording warns", func(e *Episode) { e.Occurred.End = "2026-11-07T15:24:00+11:00[Australia/Melbourne]" }, "occurred_after_recording", SevWarning, ForValidate},
		{"after recording refused", func(e *Episode) { e.Occurred.End = "2026-11-07T15:24:00+11:00[Australia/Melbourne]" }, "occurred_after_recording", SevError, ForWrite},
		{"inline without text", func(e *Episode) { e.Content.Text = nil }, "inline_without_text", SevError, ForValidate},
		{"uid on inline", func(e *Episode) { e.Content.UID = "mid:a@b" }, "uid_on_inline", SevError, ForValidate},
		{"text on ref", func(e *Episode) { e.Content.Mode = ModeRef; e.Content.Ref = "gmail:1"; e.Content.Derivation = "" }, "text_on_ref", SevError, ForValidate},
		{"ref is episode id", func(e *Episode) {
			e.Content = Content{Mode: ModeRef, Ref: "epi_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a38"}
		}, "ref_is_episode_id", SevError, ForValidate},
		{"uid without media type", func(e *Episode) { e.Content = Content{Mode: ModeRef, Ref: "gcal:1", UID: "x@google.com"} }, "uid_without_media_type", SevError, ForValidate},
		{"unknown hash algorithm", func(e *Episode) { e.Content = Content{Mode: ModeRef, Ref: "f", Hash: "blake3:ab12"} }, "hash_unverified", SevInfo, ForValidate},
		{"malformed hash", func(e *Episode) { e.Content = Content{Mode: ModeRef, Ref: "f", Hash: "nohash"} }, "hash_invalid", SevError, ForValidate},
		{"language und", func(e *Episode) { e.Content.Language = "und" }, "language_und", SevError, ForValidate},
		{"language english", func(e *Episode) { e.Content.Language = "english" }, "language_invalid", SevError, ForValidate},
		{"language on none", func(e *Episode) { e.Content = Content{Mode: ModeNone, Language: "en"} }, "language_on_none", SevError, ForValidate},
		{"bad derivation", func(e *Episode) { e.Content.Derivation = "translated" }, "invalid_derivation", SevError, ForValidate},
		{"bad mode", func(e *Episode) { e.Content.Mode = "embedded" }, "invalid_mode", SevError, ForValidate},
		{"no source", func(e *Episode) { e.Source = Source{Model: "x"} }, "source_missing", SevError, ForValidate},
		{"link shape", func(e *Episode) { e.Links = []Link{{Rel: "via"}} }, "link_shape", SevError, ForValidate},
		{"unregistered rel", func(e *Episode) { e.Links = []Link{{Rel: "answers", Href: "https://x.example"}} }, "link_rel_invalid", SevError, ForValidate},
		{"uri rel", func(e *Episode) {
			e.Links = []Link{{Rel: "https://www.w3.org/ns/activitystreams#inReplyTo", Href: "https://x.example"}}
		}, "", "", ForValidate},
		{"up to own container", func(e *Episode) {
			e.Container = "epi_01a1b4b1-8a20-7d43-b6e1-0c9f5a7d3e12"
			e.Links = []Link{{Rel: "up", Href: e.Container}}
		}, "link_restates_container", SevWarning, ForValidate},
		{"redaction path outside set", func(e *Episode) {
			e.Redacted = &Redacted{Timestamp: e.Timestamp, Reason: "r", Source: e.Source, Fields: []string{"standpoint"}}
		}, "redaction_path_invalid", SevError, ForValidate},
		{"redacted field still present", func(e *Episode) {
			e.Redacted = &Redacted{Timestamp: e.Timestamp, Reason: "r", Source: e.Source, Fields: []string{"content.text"}}
		}, "redaction_field_present", SevError, ForValidate},
		{"redacted text valid", func(e *Episode) {
			e.Content.Text = nil
			e.Redacted = &Redacted{Timestamp: e.Timestamp, Reason: "r", Source: e.Source, Fields: []string{"content.text"}}
		}, "", "", ForValidate},
		{"tombstone valid", func(e *Episode) {
			e.Content = Content{Mode: ModeRef}
			e.Redacted = &Redacted{Timestamp: e.Timestamp, Reason: "r", Source: e.Source, Fields: []string{"content.ref"}}
		}, "", "", ForValidate},
		{"non-v7 id info", func(e *Episode) { e.ID = "epi_import-42" }, "id_not_uuidv7", SevInfo, ForValidate},
		{"bad state", func(e *Episode) {
			e.Content = Content{Mode: ModeRef, Ref: "x"}
			e.SourceStatus = &SourceStatus{State: "lost", Verified: e.Timestamp, Detected: e.Timestamp}
		}, "invalid_state", SevError, ForValidate},
		{"date within a day", func(e *Episode) { e.Occurred.Start = "2026-11-08"; e.Occurred.TZ = "" }, "", "", ForValidate},
	}
	for _, c := range cases {
		e := base()
		c.mut(e)
		ps := Check(e, c.mode)
		if c.code == "" {
			if errs := codes(ps, SevError); len(errs) > 0 || len(codes(ps, SevWarning)) > 0 {
				t.Errorf("%s: unexpected %v", c.name, ps)
			}
			continue
		}
		if !has(ps, c.code, c.sev) {
			t.Errorf("%s: want %s %s, got %v", c.name, c.sev, c.code, ps)
		}
	}
}

func TestLanguage(t *testing.T) {
	good := []string{"en", "en-AU", "ja", "zh-Hant-TW", "mul", "sgn-BE-FR", "i-klingon", "x-private",
		"de-CH-1901", "sl-rozaj-biske", "en-US-u-ca-gregory", "zh-yue-HK", "es-419", "en-a-bbb-x-a-ccc"}
	bad := []string{"", "en-", "a", "en-US-", "x", "en-a", "toolongtag", "en_US", "en--US", "123"}
	for _, g := range good {
		if !WellFormedLanguage(g) {
			t.Errorf("%q should be well-formed", g)
		}
	}
	for _, b := range bad {
		if WellFormedLanguage(b) {
			t.Errorf("%q should not be well-formed", b)
		}
	}
	for in, want := range map[string]string{
		"EN-au": "en-AU", "ZH-HANT-tw": "zh-Hant-TW", "en-us-x-AB": "en-US-x-ab", "SGN-be-fr": "sgn-BE-FR",
		"i-KLINGON": "i-klingon", "en-a-bb-CD": "en-a-bb-cd", "MUL": "mul", "es-419": "es-419",
	} {
		if got := CanonicalLanguageCase(in); got != want {
			t.Errorf("case(%q) = %q, want %q", in, got, want)
		}
	}
	if !SameLanguage("EN-au", "en-AU") {
		t.Error("case-insensitive comparison")
	}
	if !WellFormedLanguage("english") || LanguageProblem("english") == nil {
		t.Error("english is well-formed under the ABNF and refused as content.language")
	}
	if err := LanguageProblem("und"); err == nil || !strings.Contains(err.Error(), "leave out") {
		t.Errorf("und: %v", err)
	}
}

// content: CRLF normalised, whitespace kept.
func TestHash(t *testing.T) {
	lf := HashBytes([]byte("a\nb\n"))
	crlf := HashBytes([]byte("a\r\nb\r\n"))
	space := HashBytes([]byte("a \nb\n"))
	if lf != crlf || lf == space {
		t.Fatalf("lf %s crlf %s space %s", lf, crlf, space)
	}
	if !strings.HasPrefix(lf, "sha256:") || len(lf) != 7+64 {
		t.Fatal(lf)
	}
	if HashBytes([]byte("a")) == HashBytes([]byte("a\n")) {
		t.Error("final newline must not be normalised")
	}
	if HashWriteProblem(lf) != nil || HashWriteProblem(strings.ToUpper(lf)) == nil || HashWriteProblem("sha512:ab") == nil {
		t.Error("write form")
	}
	if p := ParseHash("blake3:ab12"); p.Malformed || p.Implemented {
		t.Errorf("blake3: %+v", p)
	}
	if m, ok := HashMatches(lf, []byte("a\r\nb\r\n")); !m || !ok {
		t.Error("matches")
	}
}

func TestMid(t *testing.T) {
	for in, want := range map[string]string{
		"<CAF3x9q@mail.example.com>": "mid:CAF3x9q@mail.example.com",
		"  <a/b%c@x>  ":              "mid:a%2Fb%25c@x",
		"<a b#c@x>":                  "mid:a%20b%23c@x",
		"bare@x":                     "mid:bare@x",
	} {
		got, err := MidFromMessageID(in)
		if err != nil || got != want {
			t.Errorf("mid(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"<a@b", "<>", ""} {
		if _, err := MidFromMessageID(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestRels(t *testing.T) {
	for _, r := range []string{"via", "predecessor-version", "up", "collection", "duplicate"} {
		if RelProblem(r) != nil {
			t.Errorf("%s should be registered", r)
		}
	}
	for _, r := range []string{"fulfils", "Via", "answers"} {
		if err := RelProblem(r); err == nil || !strings.Contains(err.Error(), "absolute URI") {
			t.Errorf("%s: %v", r, err)
		}
	}
}

// object-format: burst minting sorts in creation order.
func TestBurstMinting(t *testing.T) {
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = NewID()
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("id %d out of order", i)
		}
		if !IsCanonicalID(ids[i]) {
			t.Fatalf("%s not canonical", ids[i])
		}
	}
	if _, ok := MonthOf("epi_import-42"); ok {
		t.Error("legacy id derived a month")
	}
	if m, ok := MonthOf("epi_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37"); !ok || m == "" {
		t.Error("v7 id derived no month")
	}
}

func TestRemovePath(t *testing.T) {
	e := base()
	e.Label = "x"
	if !e.CarriesPath("label") || e.CarriesPath("place") || e.Label != "x" {
		t.Fatal("CarriesPath")
	}
	e.RemovePath("content.text")
	if e.Content.Text != nil {
		t.Fatal("RemovePath")
	}
}
