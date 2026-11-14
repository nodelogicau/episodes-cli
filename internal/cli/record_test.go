package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

// --- recording -------------------------------------------------------------

func TestTypedNote(t *testing.T) {
	e := newEnv(t)
	m := e.ok("record", "inline", "--kind", "note", "--standpoint", ada, "--text", "Renewed the cert", "--now", "2026-11-07T03:00:00Z")
	if !strings.HasPrefix(m["path"].(string), "episodes/2026-11/") || m["id"] == nil {
		t.Fatal(m)
	}
	f := e.file(m["id"].(string))
	if !strings.Contains(f, "basis: recorded") || !strings.Contains(f, "start: \"2026-11-07T14:00:00+11:00\"") {
		t.Fatal(f)
	}
}

func TestCalendarArrangement(t *testing.T) {
	e := newEnv(t)
	m := e.ok("record", "ref", "--kind", "event", "--standpoint", ada, "--ref", "gcal:evt_7q", "--uid", "7q2m@google.com",
		"--media-type", "text/calendar", "--version", "3", "--occurred", "2026-11-07T16:02:00+11:00[Australia/Melbourne]",
		"--basis", "stated", "--label", "Budget review", "--now", "2026-11-08T00:00:00Z")
	f := e.file(m["id"].(string))
	for _, want := range []string{"kind: event", "ref: gcal:evt_7q", "uid: 7q2m@google.com", "version: \"3\"", "media_type: text/calendar", "label: Budget review", "basis: stated"} {
		if !strings.Contains(f, want) {
			t.Fatalf("missing %q:\n%s", want, f)
		}
	}
	if strings.Contains(f, "text:") {
		t.Fatal("reference episode has text")
	}
}

func TestContainerAndFirstTurnTogether(t *testing.T) {
	e := newEnv(t)
	c, turn := e.start("2026-11-07T03:00:00Z")
	if !strings.Contains(e.file(c), "content:\n  mode: none\n") || !strings.Contains(e.file(c), "kind: conversation") {
		t.Fatal(e.file(c))
	}
	if !strings.Contains(e.file(turn), "container: "+c+"\nordinal: 1\n") {
		t.Fatal(e.file(turn))
	}
	if c >= turn {
		t.Fatal("container not minted first")
	}
	// The container's standpoint defaults to the author; it can be given.
	c2, _ := e.start("2026-11-07T03:01:00Z", "--container-standpoint", jane)
	if !strings.Contains(e.file(c2), "standpoint: "+jane) {
		t.Fatal(e.file(c2))
	}
}

func TestAlternatingSpeakers(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	a := e.turn(c, ada, "2026-11-07T03:01:00Z")
	b := e.turn(c, claude, "2026-11-07T03:02:00Z")
	if a["ordinal"] != 2.0 || b["ordinal"] != 3.0 {
		t.Fatal(a["ordinal"], b["ordinal"])
	}
	if !strings.Contains(e.file(b["id"].(string)), "standpoint: "+claude) {
		t.Fatal(e.file(b["id"].(string)))
	}
	// The standpoint is required on every turn.
	e.fails(2, "conversation", "turn", c, "--text", "x")
}

func TestConcurrentTurns(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ords []int
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.run("conversation", "turn", c, "--standpoint", ada, "--text", "x")
			if r.code != 0 {
				t.Error(r)
				return
			}
			mu.Lock()
			ords = append(ords, int(r.json(t)["ordinal"].(float64)))
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Ints(ords)
	for i, o := range ords {
		if o != i+2 {
			t.Fatalf("ordinals not distinct and dense: %v", ords)
		}
	}
	if r := e.run("index", "--check"); r.code != 0 {
		t.Fatalf("torn index: %s", r.out)
	}
}

func TestUnknownContainer(t *testing.T) {
	e := newEnv(t)
	e.fails(3, "conversation", "turn", "epi_nope", "--standpoint", ada, "--text", "x")
	e.fails(3, "record", "inline", "--kind", "attachment", "--standpoint", ada, "--text", "x", "--container", "epi_01a1b4b1-8a20-7d43-b6e1-0c9f5a7d3e12")
}

func TestReplayedTurn(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	held := ""
	for i := 0; i < 3; i++ {
		held = e.turn(c, ada, "2026-11-07T03:01:00Z")["id"].(string)
	}
	code, msg := e.fails(2, "conversation", "turn", c, "--standpoint", ada, "--text", "again", "--ordinal", "4", "--now", "2026-11-07T03:05:00Z")
	if code != "duplicate" || !strings.Contains(msg, held) {
		t.Fatal(code, msg)
	}
}

func TestSameEmailTwice(t *testing.T) {
	e := newEnv(t)
	first := e.email(ada, "a@b", "2026-11-08T00:00:00Z")
	if first.code != 0 {
		t.Fatal(first)
	}
	r := e.email(ada, "a@b", "2026-11-08T00:01:00Z")
	if r.code != 2 || r.errCode(t) != "duplicate" || !strings.Contains(r.err, first.json(t)["id"].(string)) {
		t.Fatalf("%+v", r)
	}
	if r := e.email(ada, "a@b", "2026-11-08T00:02:00Z", "--allow-duplicate"); r.code != 0 {
		t.Fatal(r)
	}
}

func TestTwoPeoplesCopies(t *testing.T) {
	e := newEnv(t)
	if r := e.email(ada, "a@b", "2026-11-08T00:00:00Z"); r.code != 0 {
		t.Fatal(r)
	}
	if r := e.email(jane, "a@b", "2026-11-08T00:01:00Z"); r.code != 0 {
		t.Fatal(r)
	}
}

// --- content -----------------------------------------------------------------

func TestTranscriptDerivation(t *testing.T) {
	e := newEnv(t)
	tf := filepath.Join(t.TempDir(), "t.txt")
	_ = os.WriteFile(tf, []byte("line one\nline two\n"), 0o644)
	id := e.ok("record", "inline", "--kind", "transcript", "--standpoint", ada, "--derivation", "transcribed", "--text-file", tf, "--now", "2026-11-07T03:00:00Z")["id"].(string)
	f := e.file(id)
	if !strings.Contains(f, "derivation: transcribed") || !strings.Contains(f, "text: |\n    line one\n    line two\n") {
		t.Fatal(f)
	}
}

func TestInlineWithUIDRefused(t *testing.T) {
	e := newEnv(t)
	e.fails(2, "record", "inline", "--kind", "note", "--standpoint", ada, "--text", "x", "--uid", "mid:a@b")
}

func TestRefNamingEpisodeRefused(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "record", "ref", "--kind", "x", "--standpoint", ada, "--ref", "epi_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37", "--occurred", "2026-11-07", "--basis", "stated")
	if !strings.Contains(msg, "--link") {
		t.Fatal(msg)
	}
}

func TestTextOnReferenceRefused(t *testing.T) {
	e := newEnv(t)
	e.fails(2, "record", "ref", "--kind", "email", "--standpoint", ada, "--ref", "gmail:1", "--text", "body", "--occurred", "2026-11-07", "--basis", "stated")
}

func TestMessageIDConversion(t *testing.T) {
	e := newEnv(t)
	id := e.ok("record", "ref", "--kind", "email", "--standpoint", jane, "--ref", "gmail:msg_1", "--message-id", "<CAF3x9q@mail.example.com>",
		"--media-type", "message/rfc822", "--occurred", "2026-11-07T14:24:00+11:00", "--basis", "stated", "--now", "2026-11-08T00:00:00Z")["id"].(string)
	f := e.file(id)
	if !strings.Contains(f, "uid: mid:CAF3x9q@mail.example.com\n") {
		t.Fatal(f)
	}
	// The source keeps who recorded it apart from whose it is.
	if !strings.Contains(f, "standpoint: "+jane) || !strings.Contains(f, "author: "+ada) {
		t.Fatal(f)
	}
	e.fails(2, "record", "ref", "--kind", "email", "--standpoint", jane, "--ref", "gmail:msg_2", "--message-id", "<x@y>", "--uid", "x@y",
		"--media-type", "message/rfc822", "--occurred", "2026-11-07T14:24:00+11:00", "--basis", "stated")
}

func TestUIDWithoutMediaType(t *testing.T) {
	e := newEnv(t)
	code, msg := e.fails(2, "record", "ref", "--kind", "event", "--standpoint", ada, "--ref", "gcal:1", "--uid", "7q2m4k9d0s1a@google.com", "--occurred", "2026-11-07", "--basis", "stated")
	if code != "refused" || !strings.Contains(msg, "uid_without_media_type") {
		t.Fatal(code, msg)
	}
}

func TestHashFile(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	lf, crlf := filepath.Join(dir, "lf"), filepath.Join(dir, "crlf")
	_ = os.WriteFile(lf, []byte("a\nb\n"), 0o644)
	_ = os.WriteFile(crlf, []byte("a\r\nb\r\n"), 0o644)
	hash := func(f, now string) string {
		id := e.ok("record", "ref", "--kind", "file", "--standpoint", ada, "--ref", "drive:"+now, "--hash-file", f, "--occurred", "2026-11-07", "--basis", "stated", "--now", now)["id"].(string)
		for _, line := range strings.Split(e.file(id), "\n") {
			if strings.HasPrefix(line, "  hash: ") {
				return line
			}
		}
		t.Fatal("no hash")
		return ""
	}
	if hash(lf, "2026-11-08T00:00:00Z") != hash(crlf, "2026-11-08T00:01:00Z") {
		t.Fatal("CRLF not normalised")
	}
	e.fails(2, "record", "ref", "--kind", "file", "--standpoint", ada, "--ref", "drive:x", "--hash", "blake3:ab12", "--occurred", "2026-11-07", "--basis", "stated")
	e.fails(2, "record", "ref", "--kind", "file", "--standpoint", ada, "--ref", "drive:x", "--hash", "SHA256:"+strings.Repeat("A", 64), "--occurred", "2026-11-07", "--basis", "stated")
}

func TestLanguage(t *testing.T) {
	e := newEnv(t)
	id := e.note("2026-11-07T03:00:00Z", "--language", "EN-au")
	if !strings.Contains(e.file(id), "language: en-AU") {
		t.Fatal(e.file(id))
	}
	_, msg := e.fails(2, "record", "inline", "--kind", "note", "--standpoint", ada, "--text", "x", "--language", "und")
	if !strings.Contains(msg, "leave out") {
		t.Fatal(msg)
	}
	e.fails(2, "record", "inline", "--kind", "note", "--standpoint", ada, "--text", "x", "--language", "english")
	e.fails(2, "act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated", "--language", "en")
}

// --- links -------------------------------------------------------------------

func TestTranscriptViaRecording(t *testing.T) {
	e := newEnv(t)
	memo := e.ok("record", "ref", "--kind", "voicememo", "--standpoint", ada, "--ref", "drive:memo", "--occurred", "2026-11-07", "--basis", "stated", "--now", "2026-11-07T03:00:00Z")["id"].(string)
	id := e.note("2026-11-07T03:01:00Z", "--derivation", "transcribed", "--link", "via="+memo)
	if !strings.Contains(e.file(id), "links:\n  - rel: via\n    href: "+memo+"\n") {
		t.Fatal(e.file(id))
	}
}

func TestUnregisteredRelRefused(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "record", "inline", "--kind", "note", "--standpoint", ada, "--text", "x", "--link", "fulfils=https://example.com/int_X")
	if !strings.Contains(msg, "fulfils") || !strings.Contains(msg, "absolute URI") {
		t.Fatal(msg)
	}
}

func TestLinkToMissingEpisode(t *testing.T) {
	e := newEnv(t)
	e.fails(3, "record", "inline", "--kind", "note", "--standpoint", ada, "--text", "x", "--link", "predecessor-version=epi_nope")
}

// links: no verb adds a link to an existing episode. The verbs that take an
// existing episode and change its file are observe and redact, and neither
// accepts --link; conversation turn names a container and writes a new turn.
func TestNoLinkVerb(t *testing.T) {
	var walk func(c *cobra.Command)
	var withLink []string
	walk = func(c *cobra.Command) {
		if c.Flags().Lookup("link") != nil {
			withLink = append(withLink, c.CommandPath())
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk((&app{}).rootCmd())
	sort.Strings(withLink)
	want := []string{"episodes act", "episodes conversation start", "episodes conversation turn", "episodes record inline", "episodes record ref"}
	if strings.Join(withLink, ",") != strings.Join(want, ",") {
		t.Fatalf("verbs with --link: %v", withLink)
	}
	for _, name := range []string{"edit", "delete", "retire"} {
		if c, _, err := (&app{}).rootCmd().Find([]string{name}); err == nil && c.Name() == name {
			t.Errorf("verb %s exists", name)
		}
	}
}

func TestUpToOwnContainerRefused(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	e.fails(2, "conversation", "turn", c, "--standpoint", ada, "--text", "x", "--link", "up="+c)
}

func TestUnknownRelOnRead(t *testing.T) {
	e := newEnv(t)
	id := idAt(at("2026-11-07T03:00:00Z"))
	e.put(id, handMade(id, "")+"")
	body := strings.Replace(handMade(id, ""), "content:", "links:\n  - rel: fulfils\n    href: https://example.com/x\ncontent:", 1)
	e.put(id, body)
	if r := e.run("show", id); r.code != 0 || !strings.Contains(r.out, "fulfils") {
		t.Fatal(r)
	}
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 4 || finding(rep, "link_rel_invalid") == nil {
		t.Fatal(rep)
	}
}

// --- object-format ---------------------------------------------------------

func TestLegacyIDReadable(t *testing.T) {
	e := newEnv(t)
	e.put("epi_import-42", handMade("epi_import-42", ""))
	if r := e.run("show", "epi_import-42"); r.code != 0 {
		t.Fatal(r)
	}
	rep, _ := e.validate("2026-11-08T00:00:00Z")
	if f := finding(rep, "id_not_uuidv7"); f == nil || f["severity"] != "info" {
		t.Fatal(rep)
	}
}

func TestUnknownKindAndNonURIParticipant(t *testing.T) {
	e := newEnv(t)
	id := e.ok("record", "inline", "--kind", "voicememo", "--standpoint", ada, "--text", "x", "--participant", "Jane at accounts", "--participant", "clm_01abc", "--now", "2026-11-07T03:00:00Z")["id"].(string)
	if !strings.Contains(e.file(id), "  - Jane at accounts\n") {
		t.Fatal(e.file(id))
	}
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 0 || len(rep["findings"].([]any)) != 0 {
		t.Fatal(rep)
	}
}

func TestNoLabelSupplied(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	id := e.turn(c, ada, "2026-11-07T03:01:00Z")["id"].(string)
	if strings.Contains(e.file(id), "label:") {
		t.Fatal(e.file(id))
	}
}

// --- occurred-time -----------------------------------------------------------

func TestZonedValueQuoted(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	id := e.turn(c, ada, "2026-11-07T03:30:00Z", "--occurred", "2026-11-07T14:24:00+11:00[Australia/Melbourne]", "--basis", "recorded")["id"].(string)
	if !strings.Contains(e.file(id), `start: "2026-11-07T14:24:00+11:00[Australia/Melbourne]"`) {
		t.Fatal(e.file(id))
	}
}

func TestMixedFormsRefused(t *testing.T) {
	e := newEnv(t)
	e.fails(2, "act", "--occurred", "2026-11-03", "--end", "2026-11-03T10:00:00Z", "--basis", "stated")
}

func TestFallBackHourKeptApart(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-04-04T15:00:00Z")
	second := e.turn(c, ada, "2026-04-04T17:00:00Z", "--occurred", "2026-04-05T02:30:00+10:00[Australia/Melbourne]", "--basis", "recorded")["id"].(string)
	first := e.turn(c, ada, "2026-04-04T17:00:01Z", "--occurred", "2026-04-05T02:30:00+11:00[Australia/Melbourne]", "--basis", "recorded")["id"].(string)
	eps := e.ok("list", "--kind", "message")["episodes"].([]any)
	var order []string
	for _, x := range eps {
		order = append(order, x.(map[string]any)["id"].(string))
	}
	i, j := indexOf(order, first), indexOf(order, second)
	if i < 0 || j < 0 || i > j {
		t.Fatalf("first 02:30 should list before the second: %v", order)
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func TestWrongOffsetForZone(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "act", "--occurred", "2026-11-07T14:24:00+10:00[Australia/Melbourne]", "--basis", "stated", "--now", "2026-11-08T00:00:00Z")
	if !strings.Contains(msg, "+11:00") {
		t.Fatal(msg)
	}
}

func TestDefaultTimeNoZone(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	id := e.turn(c, ada, "2026-11-07T03:24:00Z")["id"].(string)
	if !strings.Contains(e.file(id), `start: "2026-11-07T14:24:00+11:00"`+"\n") {
		t.Fatal(e.file(id))
	}
	// With a configured zone, the zone is written.
	z := newEnv(t, "--zone", mel)
	c2, _ := z.start("2026-11-07T03:00:00Z")
	if !strings.Contains(z.file(c2), `[Australia/Melbourne]"`) {
		t.Fatal(z.file(c2))
	}
}

func TestSuffixesRefused(t *testing.T) {
	e := newEnv(t)
	e.fails(2, "act", "--occurred", "2026-11-07T14:24:00+11:00[u-ca=gregory]", "--basis", "stated", "--now", "2026-11-08T00:00:00Z")
	e.fails(2, "act", "--occurred", "2026-11-07T14:24:00+11:00[!Australia/Melbourne]", "--basis", "stated", "--now", "2026-11-08T00:00:00Z")
	e.fails(2, "act", "--occurred", "2026-11-07T14:24:00+11:00", "--tz", mel, "--basis", "stated", "--now", "2026-11-08T00:00:00Z")
}

func TestCriticalSuffixInvalidOnRead(t *testing.T) {
	e := newEnv(t)
	id := idAt(at("2026-11-07T03:00:00Z"))
	e.put(id, strings.Replace(handMade(id, ""), `"2026-11-07T14:24:00+11:00"`, `"2026-11-07T14:24:00+11:00[!Australia/Melbourne]"`, 1))
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 4 || finding(rep, "critical_suffix") == nil {
		t.Fatal(rep)
	}
}

func TestBasisRequiredAndPlannedRefused(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "record", "ref", "--kind", "email", "--standpoint", ada, "--ref", "g:1", "--occurred", "2026-11-07T16:02:00+11:00")
	if !strings.Contains(msg, "basis") {
		t.Fatal(msg)
	}
	_, msg = e.fails(2, "act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "planned")
	if !strings.Contains(msg, "plan") {
		t.Fatal(msg)
	}
}

func TestNinetyMinutesLoggedAtStart(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "act", "--occurred", "2026-11-08T09:00:00+11:00", "--end", "2026-11-08T10:30:00+11:00", "--basis", "stated", "--now", "2026-11-07T22:05:00Z")
	if !strings.Contains(msg, "85 minutes") {
		t.Fatal(msg)
	}
	// Open-ended recording succeeds.
	e.ok("act", "--occurred", "2026-11-08T09:00:00+11:00", "--basis", "stated", "--now", "2026-11-07T22:05:00Z")
}

// --- self-attested-acts ----------------------------------------------------

func TestDeepWorkLoggedAfterTheFact(t *testing.T) {
	e := newEnv(t, "--zone", mel)
	c, turn := e.start("2026-11-07T23:30:00Z")
	_ = c
	m := e.ok("act", "--occurred", "2026-11-08T09:00:00+11:00[Australia/Melbourne]", "--end", "2026-11-08T10:30:00+11:00[Australia/Melbourne]",
		"--basis", "stated", "--label", "Deep work on the budget draft", "--via", turn, "--now", "2026-11-07T23:31:00Z")
	if m["attestation"] != "via" {
		t.Fatal(m)
	}
	f := e.file(m["id"].(string))
	for _, want := range []string{"kind: activity", "content:\n  mode: none\n", "links:\n  - rel: via\n    href: " + turn, "standpoint: " + ada, "author: " + ada} {
		if !strings.Contains(f, want) {
			t.Fatalf("missing %q:\n%s", want, f)
		}
	}
}

func TestSomeoneElsesActRefused(t *testing.T) {
	e := newEnv(t)
	e.fails(2, "act", "--standpoint", jane, "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated")
}

func TestViaAssistantsTurnRefused(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	assistant := e.turn(c, claude, "2026-11-07T03:01:00Z")["id"].(string)
	e.fails(2, "act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated", "--via", assistant, "--now", "2026-11-07T03:02:00Z")
}

// self-attested-acts: the attestation is named.
func TestHarnessWithNeitherFlag(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated", "--harness", "claude", "--now", "2026-11-07T03:00:00Z")
	if !strings.Contains(msg, "--via") || !strings.Contains(msg, "--control") {
		t.Fatal(msg)
	}
}

func TestDefaultsHarnessWithControl(t *testing.T) {
	e := newEnv(t, "--harness", "claude")
	args := []string{"act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated", "--now", "2026-11-07T03:00:00Z"}
	e.fails(2, args...)
	m := e.ok(append(args, "--control")...)
	f := e.file(m["id"].(string))
	if m["attestation"] != "control" || strings.Contains(f, "links:") || !strings.Contains(f, "harness: claude") {
		t.Fatal(m, f)
	}
}

func TestConnectorWithControl(t *testing.T) {
	e := newEnv(t)
	m := e.ok("act", "--occurred", "2026-11-07T07:00:00+11:00", "--basis", "stated", "--kind", "run", "--harness", "run-logger", "--control", "--now", "2026-11-07T03:00:00Z")
	if !strings.Contains(e.file(m["id"].(string)), "  harness: run-logger\n") {
		t.Fatal(e.file(m["id"].(string)))
	}
}

func TestViaAndControlRefused(t *testing.T) {
	e := newEnv(t)
	_, turn := e.start("2026-11-07T03:00:00Z")
	code, _ := e.fails(2, "act", "--occurred", "2026-11-07T13:00:00+11:00", "--basis", "stated", "--via", turn, "--control", "--now", "2026-11-07T03:01:00Z")
	if code != "usage" {
		t.Fatal(code)
	}
}

func TestPersonsOwnAct(t *testing.T) {
	e := newEnv(t)
	m := e.ok("act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated", "--now", "2026-11-07T03:00:00Z")
	if m["attestation"] != "author" {
		t.Fatal(m)
	}
}

// self-attested-acts: a control writes no marker.
func TestControlWritesNoMarker(t *testing.T) {
	strip := func(f string) string {
		var out []string
		inSource := false
		for _, line := range strings.Split(f, "\n") {
			switch {
			case strings.HasPrefix(line, "id: "):
				continue
			case line == "source:":
				inSource = true
				continue
			case inSource && strings.HasPrefix(line, "  "):
				continue
			}
			inSource = false
			out = append(out, line)
		}
		return strings.Join(out, "\n")
	}
	args := []string{"act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated", "--label", "Run", "--now", "2026-11-07T03:00:00Z"}
	a := newEnv(t)
	plain := a.file(a.ok(args...)["id"].(string))
	b := newEnv(t)
	ctl := b.file(b.ok(append(args, "--harness", "run-logger", "--control")...)["id"].(string))
	if strip(plain) != strip(ctl) || strings.Contains(ctl, "links:") {
		t.Fatalf("--control changed the file:\n%s\n---\n%s", plain, ctl)
	}
}

func TestActWithMismatchedStandpointInvalid(t *testing.T) {
	e := newEnv(t)
	id := idAt(at("2026-11-07T03:00:00Z"))
	body := strings.Replace(handMade(id, ""), "content:\n  mode: inline\n  media_type: text/markdown\n  derivation: verbatim\n  text: hi\n", "content:\n  mode: none\n", 1)
	body = strings.Replace(body, "standpoint: "+ada, "standpoint: "+jane, 1)
	e.put(id, body)
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 4 || finding(rep, "act_standpoint_mismatch") == nil {
		t.Fatal(rep)
	}
}

func TestChildlessConversationContainer(t *testing.T) {
	e := newEnv(t)
	c, turn := e.start("2026-11-07T03:00:00Z")
	_ = os.Remove(filepath.Join(e.dir, "episodes", "2026-11", turn+".yaml"))
	_ = c
	rep, _ := e.validate("2026-11-08T00:00:00Z")
	f := finding(rep, "container_without_children")
	if f == nil || f["severity"] != "info" || !strings.Contains(f["message"].(string), "conversation turn") {
		t.Fatal(rep)
	}
	// The act rules apply: Ada's own container passes them.
	if finding(rep, "act_standpoint_mismatch") != nil {
		t.Fatal(rep)
	}
}
