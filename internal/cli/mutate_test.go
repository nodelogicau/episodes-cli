package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/store"
)

func (e *env) doc(now string, extra ...string) string {
	e.t.Helper()
	args := append([]string{"record", "ref", "--kind", "document", "--standpoint", ada, "--ref", "drive:doc", "--occurred", "2026-11-07", "--basis", "stated", "--now", now}, extra...)
	return e.ok(args...)["id"].(string)
}

// --- source-status -----------------------------------------------------------

func TestObserveStatedState(t *testing.T) {
	e := newEnv(t)
	id := e.doc("2026-11-07T03:00:00Z")
	before := e.file(id)
	m := e.ok("observe", id, "--state", "unreachable", "--now", "2026-11-08T00:00:00Z")
	if m["changed"] != true {
		t.Fatal(m)
	}
	after := e.file(id)
	want := before + "source_status:\n  state: unreachable\n  verified: 2026-11-08T00:00:00Z\n  detected: 2026-11-08T00:00:00Z\n"
	if after != want {
		t.Fatalf("got\n%s\nwant\n%s", after, want)
	}
}

func TestObserveInlineRefused(t *testing.T) {
	e := newEnv(t)
	e.fails(2, "observe", e.note("2026-11-07T03:00:00Z"), "--state", "gone")
	c, _ := e.start("2026-11-07T03:01:00Z")
	e.fails(2, "observe", c, "--state", "gone")
	e.fails(3, "observe", "epi_nope", "--state", "gone")
}

func TestObserveEditedDocument(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	orig, now := filepath.Join(dir, "orig.md"), filepath.Join(dir, "now.md")
	_ = os.WriteFile(orig, []byte("v1\n"), 0o644)
	_ = os.WriteFile(now, []byte("v2\n"), 0o644)
	id := e.doc("2026-11-07T03:00:00Z", "--hash-file", orig)
	if e.ok("observe", id, "--content-file", now, "--now", "2026-11-08T00:00:00Z")["source_status"].(map[string]any)["state"] != "changed" {
		t.Fatal(e.file(id))
	}
	if e.ok("observe", id, "--content-file", orig, "--now", "2026-11-09T00:00:00Z")["source_status"].(map[string]any)["state"] != "available" {
		t.Fatal(e.file(id))
	}
	// Version compared when there is no hash.
	v := e.doc("2026-11-07T03:01:00Z", "--version", "3")
	if e.ok("observe", v, "--version", "4", "--now", "2026-11-08T00:00:00Z")["source_status"].(map[string]any)["state"] != "changed" {
		t.Fatal(e.file(v))
	}
}

func TestObserveNothingToCompare(t *testing.T) {
	e := newEnv(t)
	f := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	e.fails(2, "observe", e.doc("2026-11-07T03:00:00Z"), "--content-file", f)
}

func TestRepeatedObservation(t *testing.T) {
	e := newEnv(t)
	id := e.doc("2026-11-07T03:00:00Z")
	e.ok("observe", id, "--state", "gone", "--now", "2026-11-01T00:00:00Z")
	e.ok("observe", id, "--state", "gone", "--now", "2026-11-05T00:00:00Z")
	f := e.file(id)
	if !strings.Contains(f, "verified: 2026-11-05T00:00:00Z\n  detected: 2026-11-01T00:00:00Z\n") {
		t.Fatal(f)
	}
	e.ok("observe", id, "--state", "available", "--now", "2026-11-06T00:00:00Z")
	if !strings.Contains(e.file(id), "detected: 2026-11-06T00:00:00Z") {
		t.Fatal(e.file(id))
	}
}

func TestSourceGoneNoBackfill(t *testing.T) {
	e := newEnv(t)
	id := e.doc("2026-11-07T03:00:00Z", "--version", "7", "--hash", "sha256:"+strings.Repeat("ab", 32))
	content := func() string {
		f := e.file(id)
		return f[strings.Index(f, "content:"):strings.Index(f, "source:")]
	}
	before := content()
	e.ok("observe", id, "--state", "gone", "--now", "2026-11-08T00:00:00Z")
	if content() != before || strings.Contains(e.file(id), "redacted") {
		t.Fatal(e.file(id))
	}
}

// --- redaction ---------------------------------------------------------------

func TestWithdrawTurn(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	turn := e.turn(c, ada, "2026-11-07T03:01:00Z", "--language", "en")["id"].(string)
	m := e.ok("redact", turn, "--fields", "content.text", "--reason", "No longer wanted", "--now", "2026-11-07T06:30:00Z")
	f := e.file(turn)
	if strings.Contains(f, "text:") {
		t.Fatal(f)
	}
	if !strings.HasSuffix(f, "redacted:\n  timestamp: 2026-11-07T06:30:00Z\n  reason: No longer wanted\n  source:\n    author: https://example.com/ada\n  fields: [content.text]\n") {
		t.Fatal(f)
	}
	// upstream content-language: language stays unless listed.
	if !strings.Contains(f, "language: en") {
		t.Fatal(f)
	}
	if n, ok := m["notice"].(string); !ok || !strings.Contains(n, "history") {
		t.Fatal(m)
	}
	if rep, code := e.validate("2026-11-08T00:00:00Z"); code != 0 {
		t.Fatal(rep)
	}
}

func TestRedactIndexFieldRefused(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "redact", e.note("2026-11-07T03:00:00Z"), "--fields", "standpoint", "--reason", "r")
	if !strings.Contains(msg, "closed set") {
		t.Fatal(msg)
	}
}

func TestRedactAbsentFieldRefused(t *testing.T) {
	e := newEnv(t)
	e.fails(2, "redact", e.note("2026-11-07T03:00:00Z"), "--fields", "place", "--reason", "r")
}

func TestRedactAgain(t *testing.T) {
	e := newEnv(t)
	id := e.note("2026-11-07T03:00:00Z", "--label", "Private")
	e.ok("redact", id, "--fields", "label", "--reason", "first", "--now", "2026-11-07T04:00:00Z")
	e.ok("redact", id, "--fields", "content.text", "--reason", "second", "--now", "2026-11-07T05:00:00Z", "--harness", "claude")
	f := e.file(id)
	if strings.Count(f, "redacted:") != 1 || !strings.Contains(f, "fields: [content.text, label]") || !strings.Contains(f, "reason: second") || !strings.Contains(f, "timestamp: 2026-11-07T05:00:00Z\n  reason") {
		t.Fatal(f)
	}
	// Listing an already-redacted path again is accepted and changes only the block.
	e.ok("redact", id, "--fields", "label", "--reason", "third", "--now", "2026-11-07T06:00:00Z")
}

func TestRedactionContainerUntouched(t *testing.T) {
	e := newEnv(t)
	c, first := e.start("2026-11-07T03:00:00Z")
	second := e.turn(c, claude, "2026-11-07T03:01:00Z")["id"].(string)
	third := e.turn(c, ada, "2026-11-07T03:02:00Z")["id"].(string)
	before := map[string]string{c: e.file(c), first: e.file(first), third: e.file(third)}
	index, _ := os.ReadFile(filepath.Join(e.dir, "index.yaml"))
	e.ok("redact", second, "--fields", "content.text", "--reason", "r", "--now", "2026-11-07T04:00:00Z")
	for id, b := range before {
		if e.file(id) != b {
			t.Fatalf("%s changed", id)
		}
	}
	after, _ := os.ReadFile(filepath.Join(e.dir, "index.yaml"))
	if string(after) != string(index) {
		t.Fatal("redaction changed the index")
	}
}

func TestTombstoneValid(t *testing.T) {
	e := newEnv(t)
	id := e.email(ada, "a@b", "2026-11-08T00:00:00Z").json(t)["id"].(string)
	e.ok("redact", id, "--fields", "content.ref,content.uid", "--reason", "moved", "--now", "2026-11-08T01:00:00Z")
	if rep, code := e.validate("2026-11-09T00:00:00Z"); code != 0 {
		t.Fatal(rep)
	}
	items := e.ok("list")["episodes"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["tombstone"] != true {
		t.Fatal(items)
	}
}

func TestRedactLinksWhole(t *testing.T) {
	e := newEnv(t)
	a := e.note("2026-11-07T03:00:00Z")
	b := e.note("2026-11-07T03:01:00Z", "--link", "via="+a, "--link", "related=https://private.example/x")
	e.ok("redact", b, "--fields", "links", "--reason", "private URL", "--now", "2026-11-07T04:00:00Z")
	if strings.Contains(e.file(b), "rel:") {
		t.Fatal(e.file(b))
	}
}

// --- accumulation ----------------------------------------------------------

func TestBetterTranscriptIsNewEpisode(t *testing.T) {
	e := newEnv(t)
	a := e.note("2026-11-07T03:00:00Z", "--derivation", "transcribed")
	before := e.file(a)
	b := e.note("2026-11-07T03:01:00Z", "--derivation", "transcribed", "--link", "predecessor-version="+a)
	if a == b || e.file(a) != before {
		t.Fatal("the earlier transcript changed")
	}
}

// accumulation: a create that finds a file at its derived path fails with
// exit 1 (the store test forces the collision; this checks the mapping).
func TestPathCollisionExit1(t *testing.T) {
	a := &app{}
	err := a.run(func(*cobra.Command, []string) error {
		return fmt.Errorf("episodes/2026-11/x.yaml: %w", store.ErrAlreadyExists)
	})(nil, nil)
	if ee := classify(err); ee.Code != 1 {
		t.Fatalf("exit %d", ee.Code)
	}
}

// --- redaction completeness (#12) --------------------------------------------

// cacheText is everything the uid cache holds on disk.
func cacheText(t *testing.T, dir string) string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, "episodes", "uid", "*.json"))
	var b strings.Builder
	for _, p := range m {
		data, _ := os.ReadFile(p)
		b.Write(data)
	}
	return b.String()
}

func TestRedactRefTakesCompanions(t *testing.T) {
	cache := t.TempDir()
	t.Setenv(store.EnvCacheDir, cache)
	e := newEnv(t)
	note := e.note("2026-11-08T00:00:00Z")
	id := e.email(ada, "secret@b", "2026-11-08T00:01:00Z", "--version", "v3",
		"--hash", "sha256:"+strings.Repeat("ab", 32), "--link", "related="+note).json(t)["id"].(string)
	if !strings.Contains(cacheText(t, cache), "secret@b") {
		t.Fatal("cache not primed")
	}
	m := e.ok("redact", id, "--fields", "content.ref", "--reason", "moved", "--now", "2026-11-08T01:00:00Z")
	added := fmt.Sprint(m["added"])
	if added != "[content.hash content.uid content.version links]" {
		t.Fatal(m)
	}
	f := e.file(id)
	for _, gone := range []string{"gmail:", "secret@b", "v3", "sha256:", note} {
		if strings.Contains(f, gone) {
			t.Fatalf("%q left behind:\n%s", gone, f)
		}
	}
	if rep, code := e.validate("2026-11-09T00:00:00Z"); code != 0 {
		t.Fatal(rep)
	}
	// The derived value is deleted, not just ignored.
	if strings.Contains(cacheText(t, cache), "secret@b") {
		t.Fatal("redacted uid still in the cache")
	}
}

func TestRedactRefAlone(t *testing.T) {
	e := newEnv(t)
	id := e.doc("2026-11-08T00:00:00Z")
	m := e.ok("redact", id, "--fields", "content.ref", "--reason", "moved", "--now", "2026-11-08T01:00:00Z")
	if _, ok := m["added"]; ok || fmt.Sprint(m["redacted"].(map[string]any)["fields"]) != "[content.ref]" {
		t.Fatal(m)
	}
}

// A file redacted before the rule, still carrying uid and hash: validate
// names each field left behind, and a second redaction repairs it.
func TestIncompleteRefRedactionRepaired(t *testing.T) {
	e := newEnv(t)
	id := e.email(ada, "old@b", "2026-11-08T00:00:00Z", "--hash", "sha256:"+strings.Repeat("cd", 32)).json(t)["id"].(string)
	f := e.file(id)
	i := strings.Index(f, "  ref: ")
	f = f[:i] + f[i+strings.Index(f[i:], "\n")+1:]
	f += "redacted:\n  timestamp: \"2026-11-08T01:00:00Z\"\n  reason: moved\n  source:\n    author: " + ada + "\n  fields:\n    - content.ref\n"
	e.put(id, f)
	rep, code := e.validate("2026-11-09T00:00:00Z")
	var left []string
	for _, x := range rep["findings"].([]any) {
		if fd := x.(map[string]any); fd["code"] == "redaction_incomplete" {
			left = append(left, fd["field"].(string))
		}
	}
	if code != 4 || strings.Join(left, ",") != "content.hash,content.uid" {
		t.Fatal(rep)
	}
	e.ok("redact", id, "--fields", "content.uid", "--reason", "complete the earlier redaction", "--now", "2026-11-08T02:00:00Z")
	if rep, code := e.validate("2026-11-09T00:00:00Z"); code != 0 {
		t.Fatal(rep)
	}
}

// A redaction merged in from another clone: the cache still holds the uid,
// the next lookup finds the file no longer carries it, and deletes it.
func TestMergedRedactionClearsCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv(store.EnvCacheDir, cache)
	e := newEnv(t)
	id := e.email(ada, "merged@b", "2026-11-08T00:00:00Z").json(t)["id"].(string)
	f := e.file(id)
	for _, key := range []string{"  ref: ", "  uid: "} {
		i := strings.Index(f, key)
		f = f[:i] + f[i+strings.Index(f[i:], "\n")+1:]
	}
	f += "redacted:\n  timestamp: \"2026-11-08T01:00:00Z\"\n  reason: moved\n  source:\n    author: " + ada + "\n  fields:\n    - content.ref\n    - content.uid\n"
	e.put(id, f)
	if !strings.Contains(cacheText(t, cache), "merged@b") {
		t.Fatal("cache should still hold the uid before the next lookup")
	}
	if r := e.email(ada, "merged@b", "2026-11-08T02:00:00Z"); r.code != 0 {
		t.Fatal(r)
	}
	// Only the new episode's uid remains.
	if n := strings.Count(cacheText(t, cache), "merged@b"); n != 1 {
		t.Fatalf("cache holds the uid %d times", n)
	}
}
