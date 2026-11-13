package cli

// The upstream acceptance suite: one subtest per scenario in
// nodelogicau/episodes openspec/specs/* (7 capabilities, 40 requirements, 78
// scenarios), each named after its capability and scenario. Scenarios that
// state a rule for writers' conduct, or for the specification's own text,
// are tested as far as the binary can enforce them, and say so.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

const okNow = "2026-11-09T00:00:00Z"

// body edits a hand-made episode: each pair replaces old with new.
func body(id string, pairs ...string) string {
	b := handMade(id, "")
	for i := 0; i+1 < len(pairs); i += 2 {
		b = strings.Replace(b, pairs[i], pairs[i+1], 1)
	}
	return b
}

const inlineBlock = "  mode: inline\n  media_type: text/markdown\n  derivation: verbatim\n  text: hi\n"

func (e *env) expectFinding(code, sev, field string) map[string]any {
	e.t.Helper()
	rep, exit := e.validate(okNow)
	f := finding(rep, code)
	if f == nil || f["severity"] != sev || (field != "" && f["field"] != field) {
		e.t.Fatalf("want %s %s (%s): %v", sev, code, field, rep)
	}
	if (sev == "error") != (exit == 4) {
		e.t.Fatalf("exit %d with %s %s", exit, sev, code)
	}
	return f
}

func (e *env) expectValid() map[string]any {
	e.t.Helper()
	rep, exit := e.validate(okNow)
	if exit != 0 || rep["errors"] != 0.0 {
		e.t.Fatalf("want valid: %v", rep)
	}
	return rep
}

func (e *env) indexBytes() string {
	b, _ := os.ReadFile(filepath.Join(e.dir, "index.yaml"))
	return string(b)
}

func TestUpstream(t *testing.T) {
	scenarios := map[string]func(t *testing.T){
		// --- content-identity ---------------------------------------------
		"content-identity/An email": func(t *testing.T) {
			e := newEnv(t)
			id := e.email(jane, "CAF3x9q@mail.example.com", okNow).json(t)["id"].(string)
			if !strings.Contains(e.file(id), "ref: gmail:CAF3x9q@mail.example.com\n  uid: mid:CAF3x9q@mail.example.com\n") {
				t.Fatal(e.file(id))
			}
		},
		"content-identity/A calendar event": func(t *testing.T) {
			e := newEnv(t)
			id := e.ok("record", "ref", "--kind", "event", "--standpoint", ada, "--ref", "gcal:evt_7q2m4k9d0s1a", "--uid", "7q2m4k9d0s1a@google.com",
				"--media-type", "text/calendar", "--occurred", "2026-11-07T16:02:00+11:00", "--basis", "stated", "--now", okNow)["id"].(string)
			if !strings.Contains(e.file(id), "uid: 7q2m4k9d0s1a@google.com\n") {
				t.Fatal(e.file(id))
			}
		},
		"content-identity/A source with no identifier": func(t *testing.T) {
			e := newEnv(t)
			id := e.doc("2026-11-08T00:00:00Z")
			if strings.Contains(e.file(id), "uid:") {
				t.Fatal(e.file(id))
			}
			e.expectValid()
		},
		"content-identity/`uid` without a media type": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, inlineBlock, "  mode: ref\n  ref: gcal:1\n  uid: x@google.com\n"))
			e.expectFinding("uid_without_media_type", "error", "content.media_type")
		},
		"content-identity/`uid` on an inline episode": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, "  text: hi\n", "  text: hi\n  uid: mid:a@b\n"))
			e.expectFinding("uid_on_inline", "error", "content.uid")
		},
		"content-identity/A mailbox migration": func(t *testing.T) {
			e := newEnv(t)
			msg := filepath.Join(t.TempDir(), "m.eml")
			_ = os.WriteFile(msg, []byte("Message-ID: <a@b>\r\n\r\nbody\r\n"), 0o644)
			id := e.email(ada, "a@b", "2026-11-08T00:00:00Z", "--hash-file", msg).json(t)["id"].(string)
			m := e.ok("observe", id, "--content-file", msg, "--now", okNow)
			if m["source_status"].(map[string]any)["state"] != "available" || !strings.Contains(e.file(id), "ref: gmail:a@b\n") {
				t.Fatal(e.file(id))
			}
		},
		"content-identity/One email recorded on two machines": func(t *testing.T) {
			e := newEnv(t)
			e.email(ada, "a@b", "2026-11-08T00:00:00Z")
			e.email(ada, "a@b", "2026-11-08T00:01:00Z", "--allow-duplicate")
			e.expectFinding("possible_duplicate", "info", "")
		},
		"content-identity/Different standpoints": func(t *testing.T) {
			e := newEnv(t)
			e.email(ada, "a@b", "2026-11-08T00:00:00Z")
			e.email(jane, "a@b", "2026-11-08T00:01:00Z")
			if rep := e.expectValid(); finding(rep, "possible_duplicate") != nil {
				t.Fatal(rep)
			}
		},
		"content-identity/Redacting a Message-ID": func(t *testing.T) {
			e := newEnv(t)
			id := e.email(ada, "a@b", "2026-11-08T00:00:00Z").json(t)["id"].(string)
			idx := e.indexBytes()
			e.ok("redact", id, "--fields", "content.uid", "--reason", "r", "--now", okNow)
			if strings.Contains(e.file(id), "uid:") || !strings.Contains(e.file(id), "fields: [content.uid]") || e.indexBytes() != idx {
				t.Fatal(e.file(id))
			}
		},

		// --- content-language ---------------------------------------------
		"content-language/A rendered summary": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-11-08T00:00:00Z", "--derivation", "rendered", "--language", "en-AU")
			if !strings.Contains(e.file(id), "language: en-AU") {
				t.Fatal(e.file(id))
			}
		},
		"content-language/A bilingual email": func(t *testing.T) {
			e := newEnv(t)
			id := e.email(ada, "a@b", "2026-11-08T00:00:00Z", "--language", "mul").json(t)["id"].(string)
			if !strings.Contains(e.file(id), "language: mul") {
				t.Fatal(e.file(id))
			}
		},
		"content-language/Undetermined": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, "  text: hi\n", "  language: und\n  text: hi\n"))
			e.expectFinding("language_und", "error", "content.language")
		},
		"content-language/An ill-formed tag": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, "  text: hi\n", "  language: english\n  text: hi\n"))
			e.expectFinding("language_invalid", "error", "content.language")
		},
		"content-language/A stated language": func(t *testing.T) {
			e := newEnv(t)
			id := e.email(ada, "a@b", "2026-11-08T00:00:00Z", "--language", "fr").json(t)["id"].(string)
			if !strings.Contains(e.file(id), "language: fr") {
				t.Fatal(e.file(id))
			}
		},
		"content-language/A detected language": func(t *testing.T) {
			// The binary never detects: without --language, none is written.
			e := newEnv(t)
			c, _ := e.start("2026-11-08T00:00:00Z")
			id := e.turn(c, ada, "2026-11-08T00:01:00Z", "--text", "bonjour tout le monde")["id"].(string)
			if strings.Contains(e.file(id), "language:") {
				t.Fatal(e.file(id))
			}
			e.expectValid()
		},
		"content-language/A calendar event with a stated language": func(t *testing.T) {
			e := newEnv(t)
			e.ok("record", "ref", "--kind", "event", "--standpoint", ada, "--ref", "gcal:1", "--language", "ja", "--occurred", "2026-11-07T16:02:00+11:00", "--basis", "stated", "--now", okNow)
			e.ok("record", "ref", "--kind", "event", "--standpoint", ada, "--ref", "gcal:2", "--language", "ja", "--media-type", "text/calendar", "--occurred", "2026-11-07T16:02:00+11:00", "--basis", "stated", "--now", okNow)
			e.expectValid()
		},
		"content-language/A transcript made with a set language": func(t *testing.T) {
			e := newEnv(t)
			memo := e.ok("record", "ref", "--kind", "voicememo", "--standpoint", ada, "--ref", "drive:memo", "--occurred", "2026-11-07", "--basis", "stated", "--now", "2026-11-08T00:00:00Z")["id"].(string)
			id := e.note("2026-11-08T00:01:00Z", "--kind", "transcript", "--derivation", "transcribed", "--language", "ja", "--link", "via="+memo)
			if !strings.Contains(e.file(id), "language: ja") {
				t.Fatal(e.file(id))
			}
		},
		"content-language/A transcript with an auto-detected language": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-11-08T00:01:00Z", "--derivation", "transcribed")
			if strings.Contains(e.file(id), "language:") {
				t.Fatal(e.file(id))
			}
			e.expectValid()
		},
		"content-language/A container with a language": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, inlineBlock, "  mode: none\n  language: en\n"))
			e.expectFinding("language_on_none", "error", "content.language")
		},
		"content-language/Case-insensitive comparison": func(t *testing.T) {
			if !model.SameLanguage("EN-au", "en-AU") {
				t.Fatal("tags differ only in case")
			}
		},
		"content-language/An unregistered subtag": func(t *testing.T) {
			e := newEnv(t)
			e.note("2026-11-08T00:00:00Z", "--language", "en-QQ")
			e.expectValid()
		},
		"content-language/Withdrawing an inline episode": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-11-08T00:00:00Z", "--language", "en")
			idx := e.indexBytes()
			e.ok("redact", id, "--fields", "content.text", "--reason", "r", "--now", okNow)
			if !strings.Contains(e.file(id), "language: en") || e.indexBytes() != idx {
				t.Fatal(e.file(id))
			}
		},

		// --- episode-evidence ---------------------------------------------
		"episode-evidence/Recording a meeting before it starts": func(t *testing.T) {
			e := newEnv(t)
			e.fails(2, "record", "inline", "--kind", "meeting", "--standpoint", ada, "--text", "x", "--occurred", "2026-11-07T14:00:00+11:00", "--basis", "stated", "--now", "2026-11-06T22:00:00Z")
		},
		"episode-evidence/Recording during a conversation": func(t *testing.T) {
			e := newEnv(t)
			c, turn := e.start("2026-11-08T00:00:00Z")
			if strings.Contains(e.file(c), "end:") || startOf(e.file(c)) != startOf(e.file(turn)) {
				t.Fatal(e.file(c))
			}
			e.expectValid()
		},
		"episode-evidence/An end in the future": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T10:00:00Z"))
			e.put(id, body(id, `  start: "2026-11-07T14:24:00+11:00"`, `  start: "2026-11-07T09:00:00Z"`+"\n"+`  end: "2026-11-07T11:00:00Z"`, "timestamp: 2026-11-07T03:24:40Z", "timestamp: 2026-11-07T10:00:00Z"))
			if f := e.expectFinding("occurred_after_recording", "warning", "occurred.end"); f["id"] != id {
				t.Fatal(f)
			}
		},
		"episode-evidence/The repeated hour": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-04-04T15:45:00Z"))
			e.put(id, body(id, `"2026-11-07T14:24:00+11:00"`, `"2026-04-05T02:30:00+10:00[Australia/Melbourne]"`, "timestamp: 2026-11-07T03:24:40Z", "timestamp: 2026-04-04T15:45:00Z"))
			e.expectFinding("occurred_after_recording", "warning", "occurred.start")
			e2 := newEnv(t)
			e2.put(id, body(id, `"2026-11-07T14:24:00+11:00"`, `"2026-04-05T02:30:00+11:00[Australia/Melbourne]"`, "timestamp: 2026-11-07T03:24:40Z", "timestamp: 2026-04-04T15:45:00Z"))
			if rep := e2.expectValid(); finding(rep, "occurred_after_recording") != nil {
				t.Fatal(rep)
			}
		},
		"episode-evidence/A date alone with no zone": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T22:00:00Z"))
			e.put(id, body(id, `"2026-11-07T14:24:00+11:00"`, `"2026-11-08"`, "timestamp: 2026-11-07T03:24:40Z", "timestamp: 2026-11-07T22:00:00Z"))
			if rep := e.expectValid(); len(findings(rep, "warning")) > 0 {
				t.Fatal(rep)
			}
		},
		"episode-evidence/A time copied from a schedule": func(t *testing.T) {
			// A writer rule; what the binary can enforce is that no basis
			// admits a plan.
			e := newEnv(t)
			_, msg := e.fails(2, "act", "--occurred", "2026-11-07T14:00:00+11:00", "--basis", "planned", "--now", okNow)
			if !strings.Contains(msg, "plan") {
				t.Fatal(msg)
			}
		},
		"episode-evidence/A time given by the person": func(t *testing.T) {
			e := newEnv(t)
			e.ok("act", "--occurred", "2026-11-07T16:00:00+11:00", "--basis", "inferred", "--now", okNow)
			e.expectValid()
		},
		"episode-evidence/Recording a future meeting on demand": func(t *testing.T) {
			e := newEnv(t)
			e.ok("record", "ref", "--kind", "event", "--standpoint", ada, "--ref", "gcal:next-tuesday", "--uid", "x@google.com", "--media-type", "text/calendar",
				"--occurred", "2026-11-07T16:02:00+11:00", "--basis", "stated", "--label", "Budget review", "--now", "2026-11-08T00:00:00Z")
			e.expectValid()
		},
		"episode-evidence/The calendar as evidence that the meeting took place": func(t *testing.T) {
			r := execute(t, "", "record", "ref", "--help")
			if !strings.Contains(r.out, "arranged") || !strings.Contains(r.out, "never its scheduled time") {
				t.Fatal(r.out)
			}
		},
		"episode-evidence/A gap in the record": func(t *testing.T) {
			e := newEnv(t)
			m := e.ok("list", "--since", "2026-10-01", "--until", "2026-10-31")
			if m["count"] != 0.0 || len(m) != 2 {
				t.Fatal(m)
			}
		},
		"episode-evidence/A retirement and a claim disagree": func(t *testing.T) {
			// Nothing in this implementation reads intentions or DKF data.
			e := newEnv(t)
			e.note("2026-11-08T00:00:00Z", "--participant", "int_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37")
			if rep := e.expectValid(); len(rep["findings"].([]any)) != 0 {
				t.Fatal(rep)
			}
		},
		"episode-evidence/Standalone conformance still holds": func(t *testing.T) {
			standalone(t)
		},

		// --- episode-links ------------------------------------------------
		"episode-links/A registered relation": func(t *testing.T) {
			e := newEnv(t)
			a := e.note("2026-11-08T00:00:00Z")
			e.note("2026-11-08T00:01:00Z", "--link", "via="+a)
			e.expectValid()
		},
		"episode-links/An extension relation": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-11-08T00:00:00Z", "--link", "https://www.w3.org/ns/activitystreams#inReplyTo=https://example.com/post")
			if r := e.run("show", id); r.code != 0 {
				t.Fatal(r)
			}
			e.expectValid()
		},
		"episode-links/A non-URI unregistered relation": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, "content:", "links:\n  - rel: answers\n    href: https://example.com/q\ncontent:"))
			e.expectFinding("link_rel_invalid", "error", "links[0].rel")
		},
		"episode-links/Adding a link later": func(t *testing.T) {
			orig := &model.Episode{ID: "epi_x", Type: "episode", Kind: "note"}
			next := orig.Clone()
			next.Links = []model.Link{{Rel: "via", Href: "https://example.com"}}
			if err := store.Guard(orig, next); !errors.Is(err, store.ErrRevision) {
				t.Fatalf("adding a link: %v", err)
			}
		},
		"episode-links/Redacting a link to a private URL": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-11-08T00:00:00Z", "--link", "related=https://private.example/x")
			idx := e.indexBytes()
			e.ok("redact", id, "--fields", "links", "--reason", "r", "--now", okNow)
			if strings.Contains(e.file(id), "links:") || !strings.Contains(e.file(id), "fields: [links]") || e.indexBytes() != idx {
				t.Fatal(e.file(id))
			}
		},
		"episode-links/The target was never copied": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, "content:", "links:\n  - rel: via\n    href: epi_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37\ncontent:"))
			e.ok("index")
			e.expectFinding("link_unresolved", "info", "")
		},
		"episode-links/A transcript of a voice memo": func(t *testing.T) {
			e := newEnv(t)
			a := e.ok("record", "ref", "--kind", "voicememo", "--standpoint", ada, "--ref", "drive:memo", "--occurred", "2026-11-07", "--basis", "stated", "--now", "2026-11-08T00:00:00Z")["id"].(string)
			b := e.note("2026-11-08T00:01:00Z", "--derivation", "transcribed", "--link", "via="+a)
			if !strings.Contains(e.file(b), "derivation: transcribed") || !strings.Contains(e.file(b), "  - rel: via\n    href: "+a) {
				t.Fatal(e.file(b))
			}
		},
		"episode-links/A better transcript": func(t *testing.T) {
			e := newEnv(t)
			a := e.ok("record", "ref", "--kind", "voicememo", "--standpoint", ada, "--ref", "drive:memo", "--occurred", "2026-11-07", "--basis", "stated", "--now", "2026-11-08T00:00:00Z")["id"].(string)
			b := e.note("2026-11-08T00:01:00Z", "--derivation", "transcribed", "--link", "via="+a)
			before := e.file(b)
			c := e.note("2026-11-08T00:02:00Z", "--derivation", "transcribed", "--link", "via="+a, "--link", "predecessor-version="+b)
			want := "links:\n  - rel: predecessor-version\n    href: " + b + "\n  - rel: via\n    href: " + a + "\n"
			if !strings.Contains(e.file(c), want) || e.file(b) != before {
				t.Fatal(e.file(c))
			}
		},
		"episode-links/A turn that links up to its conversation": func(t *testing.T) {
			e := newEnv(t)
			c, _ := e.start("2026-11-07T03:00:00Z")
			id := idAt(at("2026-11-07T03:30:00Z"))
			e.put(id, body(id, "content:", "container: "+c+"\nordinal: 2\nlinks:\n  - rel: up\n    href: "+c+"\ncontent:"))
			if f := e.expectFinding("link_restates_container", "warning", ""); f["id"] != id {
				t.Fatal(f)
			}
		},
		"episode-links/Reading the links section": func(t *testing.T) {
			// No relation this implementation gives meaning targets another
			// format's object, and none is registered for one.
			for _, rel := range []string{"fulfils", "fulfills", "realises", "cites"} {
				if model.IsRegisteredRel(rel) {
					t.Errorf("%s is treated as registered", rel)
				}
			}
		},

		// --- occurred-time ------------------------------------------------
		"occurred-time/A conversation turn": func(t *testing.T) {
			e := newEnv(t, "--zone", mel)
			c, _ := e.start("2026-11-07T03:00:00Z")
			id := e.turn(c, ada, "2026-11-07T03:24:00Z")["id"].(string)
			if !strings.Contains(e.file(id), `start: "2026-11-07T14:24:00+11:00[Australia/Melbourne]"`) {
				t.Fatal(e.file(id))
			}
		},
		"occurred-time/The repeated hour": func(t *testing.T) {
			e := newEnv(t)
			id := e.ok("act", "--occurred", "2026-04-05T02:30:00+10:00[Australia/Melbourne]", "--basis", "stated", "--now", "2026-04-04T17:00:00Z")["id"].(string)
			if !strings.Contains(e.file(id), `"2026-04-05T02:30:00+10:00[Australia/Melbourne]"`) {
				t.Fatal(e.file(id))
			}
		},
		"occurred-time/A mail header with an offset only": func(t *testing.T) {
			e := newEnv(t)
			id := e.email(ada, "a@b", "2026-11-08T00:00:00Z").json(t)["id"].(string)
			if !strings.Contains(e.file(id), `start: "2026-11-07T14:24:00+11:00"`+"\n") {
				t.Fatal(e.file(id))
			}
			e.expectValid()
		},
		"occurred-time/Inconsistent offset and zone": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, `"2026-11-07T14:24:00+11:00"`, `"2026-01-15T10:00:00+10:00[Australia/Melbourne]"`))
			if f := e.expectFinding("zone_offset_mismatch", "warning", "occurred.start"); f["id"] != id {
				t.Fatal(f)
			}
			// Readers take the instant from +10:00: 2026-01-15T00:00Z.
			later := e.ok("act", "--occurred", "2026-01-15T00:30:00Z", "--basis", "stated", "--now", "2026-11-07T04:00:00Z")["id"].(string)
			earlier := e.ok("act", "--occurred", "2026-01-14T23:30:00Z", "--basis", "stated", "--now", "2026-11-07T04:01:00Z")["id"].(string)
			var order []string
			for _, it := range e.ok("list")["episodes"].([]any) {
				order = append(order, it.(map[string]any)["id"].(string))
			}
			if indexOf(order, earlier) >= indexOf(order, id) || indexOf(order, id) >= indexOf(order, later) {
				t.Fatalf("not ordered by the offset's instant: %v", order)
			}
		},
		"occurred-time/An elective calendar tag": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T04:00:00Z"))
			e.put(id, body(id, `"2026-11-07T14:24:00+11:00"`, `"2026-11-07T14:24:00+11:00[Australia/Melbourne][u-ca=gregory]"`))
			if rep := e.expectValid(); len(findings(rep, "warning")) > 0 {
				t.Fatal(rep)
			}
		},
		"occurred-time/An inferred date": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-11-08T00:00:00Z", "--occurred", "2026-11-03", "--tz", mel, "--basis", "inferred")
			if !strings.Contains(e.file(id), "occurred:\n  start: \"2026-11-03\"\n  tz: Australia/Melbourne\n  basis: inferred\n") {
				t.Fatal(e.file(id))
			}
		},
		"occurred-time/`tz` beside a date-time": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, "  basis: stated\n", "  tz: Australia/Melbourne\n  basis: stated\n"))
			e.expectFinding("tz_beside_datetime", "error", "occurred.tz")
		},
		"occurred-time/Mixed forms": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, `  start: "2026-11-07T14:24:00+11:00"`, `  start: "2026-11-03"`+"\n"+`  end: "2026-11-03T10:00:00Z"`))
			e.expectFinding("occurred_mixed_forms", "error", "")
		},
		"occurred-time/The index in flow style": func(t *testing.T) {
			e := newEnv(t, "--zone", mel)
			e.start("2026-11-07T03:24:00Z")
			idx := e.indexBytes()
			if !strings.Contains(idx, `occurred: {start: "2026-11-07T14:24:00+11:00[Australia/Melbourne]", basis: recorded}`) {
				t.Fatal(idx)
			}
			var root struct {
				Entries []struct {
					Occurred struct {
						Start any `yaml:"start"`
					} `yaml:"occurred"`
				} `yaml:"entries"`
			}
			if err := yaml.Unmarshal([]byte(idx), &root); err != nil {
				t.Fatal(err)
			}
			if s, ok := root.Entries[0].Occurred.Start.(string); !ok || s != "2026-11-07T14:24:00+11:00[Australia/Melbourne]" {
				t.Fatalf("%#v", root.Entries[0].Occurred.Start)
			}
		},

		// --- self-attested-acts -------------------------------------------
		"self-attested-acts/Attested in conversation": func(t *testing.T) {
			e := newEnv(t, "--zone", mel)
			_, turn := e.start("2026-11-08T00:00:00Z", "--text", "log ninety minutes of deep work from nine")
			e.ok("act", "--occurred", "2026-11-08T09:00:00+11:00[Australia/Melbourne]", "--end", "2026-11-08T10:30:00+11:00[Australia/Melbourne]",
				"--basis", "stated", "--via", turn, "--harness", "claude", "--now", "2026-11-08T00:01:00Z")
			e.expectValid()
		},
		"self-attested-acts/Attested through a logging action": func(t *testing.T) {
			e := newEnv(t)
			id := e.ok("act", "--occurred", "2026-11-08T09:00:00+11:00", "--basis", "stated", "--now", okNow)["id"].(string)
			if strings.Contains(e.file(id), "links:") {
				t.Fatal(e.file(id))
			}
			e.expectValid()
		},
		"self-attested-acts/An episode id in `content.ref`": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, inlineBlock, "  mode: ref\n  ref: epi_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37\n"))
			e.expectFinding("ref_is_episode_id", "error", "content.ref")
		},
		"self-attested-acts/Recording another person's act": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, inlineBlock, "  mode: none\n", "standpoint: "+ada, "standpoint: "+jane))
			e.expectFinding("act_standpoint_mismatch", "error", "standpoint")
		},
		"self-attested-acts/Attestation by someone else": func(t *testing.T) {
			e := newEnv(t)
			mail := e.email(jane, "a@b", "2026-11-07T03:30:00Z").json(t)["id"].(string)
			id := idAt(at("2026-11-07T04:00:00Z"))
			e.put(id, body(id, inlineBlock, "  mode: none\n", "content:", "links:\n  - rel: via\n    href: "+mail+"\ncontent:"))
			e.expectFinding("act_via_standpoint", "error", "links")
		},
		"self-attested-acts/The attestation was dropped": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T04:00:00Z"))
			e.put(id, body(id, inlineBlock, "  mode: none\n", "content:", "links:\n  - rel: via\n    href: epi_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37\ncontent:"))
			e.expectFinding("link_unresolved", "info", "")
		},
		"self-attested-acts/A passing remark": func(t *testing.T) {
			// A turn stays a turn: recording one writes exactly one episode,
			// and only `act` writes an act.
			e := newEnv(t)
			c, _ := e.start("2026-11-08T00:00:00Z")
			before := e.ok("list")["count"].(float64)
			e.turn(c, ada, "2026-11-08T00:01:00Z", "--text", "finally got the budget done")
			if e.ok("list")["count"].(float64) != before+1 || e.ok("list", "--kind", "activity")["count"] != 0.0 {
				t.Fatal("a turn produced more than itself")
			}
		},
		"self-attested-acts/An unusual kind": func(t *testing.T) {
			e := newEnv(t)
			e.ok("act", "--kind", "workout", "--occurred", "2026-11-08T07:00:00+11:00", "--basis", "stated", "--now", okNow)
			e.expectValid()
		},
		"self-attested-acts/A container": func(t *testing.T) {
			e := newEnv(t)
			c, _ := e.start("2026-11-08T00:00:00Z", "--container-standpoint", jane)
			if !strings.Contains(e.file(c), "standpoint: "+jane) {
				t.Fatal(e.file(c))
			}
			e.expectValid()
		},
		"self-attested-acts/Logged before it ends": func(t *testing.T) {
			e := newEnv(t)
			e.fails(2, "act", "--occurred", "2026-11-08T09:00:00+11:00", "--end", "2026-11-08T10:30:00+11:00", "--basis", "stated", "--now", "2026-11-07T23:00:00Z")
			e.ok("act", "--occurred", "2026-11-08T09:00:00+11:00", "--basis", "stated", "--now", "2026-11-07T23:00:00Z")
		},

		// --- standalone-conformance ---------------------------------------
		"standalone-conformance/Validating with no sibling workspace": func(t *testing.T) {
			e := newEnv(t)
			e.note("2026-11-08T00:00:00Z", "--participant", "https://nowhere.invalid/x", "--place", "clm_01a1")
			for _, sib := range []string{"dkf.yaml", "intentions.yaml"} {
				if _, err := os.Stat(filepath.Join(e.dir, sib)); err == nil {
					t.Fatal(sib)
				}
			}
			e.expectValid()
		},
		"standalone-conformance/A draft file carries `occasion`": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, handMade(id, "occasion: int_X\n"))
			if rep := e.expectValid(); len(rep["findings"].([]any)) > 1 {
				t.Fatal(rep)
			}
			if r := e.run("show", id); r.code != 0 || !strings.Contains(r.out, "occasion") {
				t.Fatal(r)
			}
		},
		"standalone-conformance/A standpoint with no resolvable identity": func(t *testing.T) {
			e := newEnv(t)
			e.ok("record", "inline", "--kind", "note", "--standpoint", "Jane at accounts", "--text", "x", "--now", okNow)
			e.expectValid()
		},
		"standalone-conformance/Reading the definition": func(t *testing.T) {
			e := newEnv(t)
			e.ok("record", "inline", "--kind", "note", "--standpoint", "prt_01a1b4b2", "--participant", "prt_01a1b4b3", "--place", "prt_place", "--author", "prt_author", "--text", "x", "--now", okNow)
			e.expectValid()
		},
		"standalone-conformance/Line endings differ": func(t *testing.T) {
			e := newEnv(t)
			dir := t.TempDir()
			lf, crlf := filepath.Join(dir, "lf"), filepath.Join(dir, "crlf")
			_ = os.WriteFile(lf, []byte("a\nb\n"), 0o644)
			_ = os.WriteFile(crlf, []byte("a\r\nb\r\n"), 0o644)
			id := e.doc("2026-11-08T00:00:00Z", "--hash-file", lf)
			if e.ok("observe", id, "--content-file", crlf, "--now", okNow)["source_status"].(map[string]any)["state"] != "available" {
				t.Fatal(e.file(id))
			}
		},
		"standalone-conformance/Unknown algorithm": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, inlineBlock, "  mode: ref\n  ref: drive:x\n  hash: blake3:ab12cd34\n"))
			e.expectFinding("hash_unverified", "info", "content.hash")
		},
		"standalone-conformance/A redacted inline episode lacks text": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, handMade(id, "redacted:\n  timestamp: 2026-11-07T06:00:00Z\n  reason: r\n  source:\n    author: x\n  fields: [content.text]\n"))
			e.put(id, strings.Replace(e.file(id), "  text: hi\n", "", 1))
			e.expectValid()
		},
		"standalone-conformance/Text missing with no redaction": func(t *testing.T) {
			e := newEnv(t)
			id := idAt(at("2026-11-07T03:00:00Z"))
			e.put(id, body(id, "  text: hi\n", ""))
			e.expectFinding("inline_without_text", "error", "content.text")
		},
		"standalone-conformance/Redacting twice": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-11-08T00:00:00Z", "--label", "L")
			e.ok("redact", id, "--fields", "content.text", "--reason", "one", "--now", "2026-11-08T01:00:00Z")
			e.ok("redact", id, "--fields", "label", "--reason", "two", "--now", "2026-11-08T02:00:00Z", "--harness", "claude")
			f := e.file(id)
			if strings.Count(f, "redacted:") != 1 || !strings.Contains(f, "redacted:\n  timestamp: 2026-11-08T02:00:00Z\n  reason: two\n  source:\n    author: https://example.com/ada\n    harness: claude\n  fields: [content.text, label]\n") {
				t.Fatal(f)
			}
		},
		"standalone-conformance/Deriving a path": func(t *testing.T) {
			id := idAt(at("2026-11-07T12:00:00Z"))
			if store.RelPath(id) != "episodes/2026-11/"+id+".yaml" {
				t.Fatal(store.RelPath(id))
			}
		},
		"standalone-conformance/Changing a sealed month": func(t *testing.T) {
			e := newEnv(t)
			id := e.note("2026-09-07T03:00:00Z")
			e.note("2026-11-07T03:00:00Z")
			p := filepath.Join(e.dir, filepath.FromSlash(store.RelPath(id)))
			data, _ := os.ReadFile(p)
			_ = os.WriteFile(p, []byte(strings.Replace(string(data), "kind: note", "kind: call", 1)), 0o644)
			if f := e.expectFinding("revised", "error", "kind"); !strings.Contains(f["message"].(string), "index/2026-09.yaml") {
				t.Fatal(f)
			}
			// Rewriting source_status or adding to redacted is conformant.
			e2 := newEnv(t)
			old := e2.note("2026-09-07T03:00:00Z", "--label", "L")
			e2.note("2026-11-07T03:00:00Z")
			e2.ok("redact", old, "--fields", "label", "--reason", "r", "--now", "2026-11-07T04:00:00Z")
			e2.expectValid()
		},
		"standalone-conformance/A late merge": func(t *testing.T) {
			e := newEnv(t)
			e.note("2026-09-10T00:00:00Z")
			e.note("2026-11-07T00:00:00Z")
			late := idAt(at("2026-09-28T00:00:00Z"))
			e.put(late, handMade(late, ""))
			if f := e.expectFinding("late_merge", "error", ""); !strings.Contains(f["message"].(string), late) {
				t.Fatal(f)
			}
		},
		"standalone-conformance/An id far in the future": func(t *testing.T) {
			e := newEnv(t)
			far := idAt(at("2027-03-10T00:00:00Z"))
			e.put(far, handMade(far, ""))
			if f := e.expectFinding("id_far_future", "error", "id"); f["id"] != far {
				t.Fatal(f)
			}
		},
		"standalone-conformance/Implementing episodes alone": func(t *testing.T) {
			standalone(t)
		},
		"standalone-conformance/A sibling format is mentioned": func(t *testing.T) {
			// No flag of any verb takes an object of another format.
			for _, c := range allCommands((&app{}).rootCmd()) {
				for _, name := range []string{"claim", "intention", "occasion", "particular", "cite"} {
					if c.Flags().Lookup(name) != nil {
						t.Errorf("%s has --%s", c.CommandPath(), name)
					}
				}
			}
		},
	}
	if len(scenarios) != 78 {
		t.Fatalf("%d scenarios, want 78", len(scenarios))
	}
	for name, fn := range scenarios {
		t.Run(name, fn)
	}
}

func startOf(file string) string {
	for _, line := range strings.Split(file, "\n") {
		if strings.HasPrefix(line, "  start: ") {
			return line
		}
	}
	return ""
}

// standalone checks that the module imports nothing from particulars-cli or
// intentions-cli: the format needs no sibling's code (task 9.5).
func standalone(t *testing.T) {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "github.com/nodelogicau/episodes-cli/...").CombinedOutput()
	if err != nil {
		t.Skipf("go list unavailable: %v\n%s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.Contains(dep, "particulars-cli") || strings.Contains(dep, "intentions-cli") {
			t.Fatalf("imports %s", dep)
		}
	}
	if !strings.Contains(string(out), "github.com/nodelogicau/episodes-cli/internal/model") {
		t.Fatalf("go list did not list the module:\n%s", out)
	}
}
