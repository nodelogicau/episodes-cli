package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/store"
)

const (
	ada    = "https://example.com/ada"
	jane   = "https://example.com/jane"
	claude = "https://claude.ai"
	mel    = "Australia/Melbourne"
)

func TestMain(m *testing.M) {
	// A fixed process offset, so "the local offset" is deterministic.
	time.Local = time.FixedZone("test", 11*3600)
	for _, k := range []string{"EPISODES_WORKSPACE", "EPISODES_AUTHOR", "EPISODES_HARNESS", "EPISODES_MODEL", "EPISODES_NOW", "EPISODES_ZONE"} {
		_ = os.Unsetenv(k)
	}
	os.Exit(m.Run())
}

type env struct {
	t   *testing.T
	dir string
}

type result struct {
	out, err string
	code     int
}

func (r result) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(r.out), &m); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr: %s", err, r.out, r.err)
	}
	return m
}

func (r result) errCode(t *testing.T) string {
	t.Helper()
	var m map[string]map[string]string
	if err := json.Unmarshal([]byte(r.err), &m); err != nil {
		t.Fatalf("stderr is not a JSON error: %v\n%s", err, r.err)
	}
	return m["error"]["code"]
}

// newEnv initialises a workspace for Ada.
func newEnv(t *testing.T, extra ...string) *env {
	t.Helper()
	dir := t.TempDir()
	args := append([]string{"init", dir, "--author", ada}, extra...)
	if r := execute(t, "", args...); r.code != 0 {
		t.Fatalf("init: %+v", r)
	}
	return &env{t: t, dir: dir}
}

func execute(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(args, strings.NewReader(stdin), &out, &errb, func() bool { return false })
	return result{out.String(), errb.String(), code}
}

// run executes a verb in the workspace with --json.
func (e *env) run(args ...string) result {
	e.t.Helper()
	return execute(e.t, "", append(args, "--workspace", e.dir, "--json")...)
}

func (e *env) runIn(stdin string, args ...string) result {
	e.t.Helper()
	return execute(e.t, stdin, append(args, "--workspace", e.dir, "--json")...)
}

// ok runs a verb that must succeed and returns its JSON.
func (e *env) ok(args ...string) map[string]any {
	e.t.Helper()
	r := e.run(args...)
	if r.code != 0 {
		e.t.Fatalf("%v: exit %d\n%s%s", args, r.code, r.out, r.err)
	}
	if r.err != "" {
		e.t.Fatalf("%v: stderr not empty: %s", args, r.err)
	}
	return r.json(e.t)
}

// fails runs a verb that must fail with code, leaving the workspace
// unchanged; it returns the JSON error code and message.
func (e *env) fails(code int, args ...string) (string, string) {
	e.t.Helper()
	before := e.snapshot()
	r := e.run(args...)
	if r.code != code {
		e.t.Fatalf("%v: exit %d, want %d\n%s%s", args, r.code, code, r.out, r.err)
	}
	if r.out != "" {
		e.t.Fatalf("%v: stdout not empty on failure: %s", args, r.out)
	}
	if after := e.snapshot(); after != before {
		e.t.Fatalf("%v: workspace changed by a failed command", args)
	}
	var m map[string]map[string]string
	if err := json.Unmarshal([]byte(r.err), &m); err != nil {
		e.t.Fatalf("stderr is not a JSON error: %s", r.err)
	}
	return m["error"]["code"], m["error"]["message"]
}

// snapshot is every file's path and bytes, for "nothing changed".
func (e *env) snapshot() string {
	var b strings.Builder
	_ = filepath.Walk(e.dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(p)
		b.WriteString(p + "\x00" + string(data) + "\x00")
		return nil
	})
	return b.String()
}

func (e *env) file(id string) string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.dir, filepath.FromSlash(store.RelPath(id))))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

// put writes a hand-made episode file at the path its id derives.
func (e *env) put(id, body string) {
	e.t.Helper()
	p := filepath.Join(e.dir, filepath.FromSlash(store.RelPath(id)))
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// note records an inline note and returns its id.
func (e *env) note(now string, extra ...string) string {
	e.t.Helper()
	args := append([]string{"record", "inline", "--kind", "note", "--standpoint", ada, "--text", "a note", "--now", now}, extra...)
	return e.ok(args...)["id"].(string)
}

// start begins a conversation and returns the container and first turn.
func (e *env) start(now string, extra ...string) (string, string) {
	e.t.Helper()
	args := append([]string{"conversation", "start", "--standpoint", ada, "--text", "hello", "--now", now}, extra...)
	m := e.ok(args...)
	return m["container"].(map[string]any)["id"].(string), m["turn"].(map[string]any)["id"].(string)
}

func (e *env) turn(container, standpoint, now string, extra ...string) map[string]any {
	e.t.Helper()
	args := append([]string{"conversation", "turn", container, "--standpoint", standpoint, "--text", "turn", "--now", now}, extra...)
	return e.ok(args...)
}

// email records a reference episode for a Message-ID.
func (e *env) email(standpoint, mid, now string, extra ...string) result {
	e.t.Helper()
	args := append([]string{"record", "ref", "--kind", "email", "--standpoint", standpoint, "--ref", "gmail:" + mid,
		"--message-id", "<" + mid + ">", "--media-type", "message/rfc822",
		"--occurred", "2026-11-07T14:24:00+11:00", "--basis", "stated", "--now", now}, extra...)
	return e.run(args...)
}

func (e *env) validate(now string) (map[string]any, int) {
	e.t.Helper()
	r := e.run("validate", "--now", now)
	return r.json(e.t), r.code
}

// finding returns the first finding with code, or nil.
func finding(report map[string]any, code string) map[string]any {
	for _, f := range report["findings"].([]any) {
		m := f.(map[string]any)
		if m["code"] == code {
			return m
		}
	}
	return nil
}

func findings(report map[string]any, sev string) []string {
	var out []string
	for _, f := range report["findings"].([]any) {
		m := f.(map[string]any)
		if m["severity"] == sev {
			out = append(out, m["code"].(string))
		}
	}
	return out
}

// idAt mints a UUIDv7 episode id at t, for hand-made files.
func idAt(t time.Time) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	ms := uint64(t.UnixMilli())
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (40 - 8*i))
	}
	b[6] = 0x70 | b[6]&0x0f
	b[8] = 0x80 | b[8]&0x3f
	h := hex.EncodeToString(b[:])
	return "epi_" + h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// handMade is a valid episode body for id, in the given form.
func handMade(id, extra string) string {
	return "id: " + id + `
type: episode
kind: note
occurred:
  start: "2026-11-07T14:24:00+11:00"
  basis: stated
standpoint: https://example.com/ada
content:
  mode: inline
  media_type: text/markdown
  derivation: verbatim
  text: hi
source:
  author: https://example.com/ada
timestamp: 2026-11-07T03:24:40Z
` + extra
}

// --- cli-interface -----------------------------------------------------

func TestTurnTextFromStdin(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	r := e.runIn("piped bytes\n", "conversation", "turn", c, "--standpoint", ada, "--text-file", "-", "--now", "2026-11-07T03:01:00Z")
	if r.code != 0 {
		t.Fatal(r)
	}
	if !strings.Contains(e.file(r.json(t)["id"].(string)), "text: |\n    piped bytes\n") {
		t.Fatal(e.file(r.json(t)["id"].(string)))
	}
	// A terminal on stdin is refused, never read.
	var out, errb bytes.Buffer
	code := Execute([]string{"conversation", "turn", c, "--standpoint", ada, "--text-file", "-", "--workspace", e.dir}, strings.NewReader(""), &out, &errb, func() bool { return true })
	if code != 2 || !strings.Contains(errb.String(), "terminal") {
		t.Fatalf("%d %s", code, errb.String())
	}
}

func TestMissingStandpoint(t *testing.T) {
	e := newEnv(t)
	_, msg := e.fails(2, "record", "inline", "--kind", "note", "--text", "hi")
	if !strings.Contains(msg, "standpoint") {
		t.Fatal(msg)
	}
}

func TestJSONOutputMode(t *testing.T) {
	e := newEnv(t)
	id := e.note("2026-11-07T03:00:00Z")
	for _, args := range [][]string{{"version"}, {"workspace"}, {"show", id}, {"list"}, {"validate"}, {"index"}, {"index", "--check"}} {
		r := e.run(args...)
		if r.code != 0 || r.err != "" {
			t.Fatalf("%v: %+v", args, r)
		}
		r.json(t)
	}
	r := e.run("show", "epi_does-not-exist")
	if r.code != 3 || r.out != "" || r.errCode(t) != "not_found" {
		t.Fatalf("%+v", r)
	}
}

func TestExitCodes(t *testing.T) {
	e := newEnv(t)
	e.fails(3, "show", "epi_does-not-exist")
	code, msg := e.fails(2, "record", "inline", "--kind", "Phone Call", "--standpoint", ada, "--text", "hi")
	if code != "refused" || !strings.Contains(msg, "invalid_kind") {
		t.Fatal(code, msg)
	}
	t.Chdir(t.TempDir())
	r := execute(t, "", "list", "--json")
	if r.code != 5 || r.errCode(t) != "no_workspace" {
		t.Fatalf("%+v", r)
	}
}

func TestWorkspaceFlagWinsOverEnv(t *testing.T) {
	a, b := newEnv(t), newEnv(t)
	t.Setenv("EPISODES_WORKSPACE", b.dir)
	m := a.ok("workspace")
	if m["root"] != a.dir && !strings.HasSuffix(m["root"].(string), filepath.Base(a.dir)) {
		t.Fatalf("%v", m["root"])
	}
	if m["found_by"] != "flag" {
		t.Fatal(m["found_by"])
	}
}

func TestHarnessOnDefaultAuthorsBehalf(t *testing.T) {
	e := newEnv(t)
	id := e.note("2026-11-07T03:00:00Z", "--harness", "claude", "--model", "claude-opus-5-5")
	if !strings.Contains(e.file(id), "source:\n  author: https://example.com/ada\n  harness: claude\n  model: claude-opus-5-5\n") {
		t.Fatal(e.file(id))
	}
}

func TestNoSourceAtAll(t *testing.T) {
	dir := t.TempDir()
	if r := execute(t, "", "init", dir); r.code != 0 {
		t.Fatal(r)
	}
	e := &env{t: t, dir: dir}
	_, msg := e.fails(2, "record", "inline", "--kind", "note", "--standpoint", ada, "--text", "hi")
	if !strings.Contains(msg, "source") {
		t.Fatal(msg)
	}
	// A harness alone is a valid source.
	e.ok("record", "inline", "--kind", "note", "--standpoint", ada, "--text", "hi", "--harness", "claude")
}

func TestTimestampFromNow(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	m := e.turn(c, ada, "2026-11-07T03:24:40Z")
	if !strings.Contains(e.file(m["id"].(string)), "timestamp: 2026-11-07T03:24:40Z\n") {
		t.Fatal(e.file(m["id"].(string)))
	}
	t.Setenv("EPISODES_NOW", "2026-11-07T04:00:00Z")
	id := e.ok("record", "inline", "--kind", "note", "--standpoint", ada, "--text", "x")["id"].(string)
	if !strings.Contains(e.file(id), "timestamp: 2026-11-07T04:00:00Z\n") {
		t.Fatal(e.file(id))
	}
}

func TestNoGitOperations(t *testing.T) {
	e := newEnv(t)
	gitDir := filepath.Join(e.dir, ".git")
	_ = os.MkdirAll(gitDir, 0o755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	before, _ := os.ReadDir(gitDir)
	e.note("2026-11-07T03:00:00Z")
	after, _ := os.ReadDir(gitDir)
	if len(before) != len(after) {
		t.Fatal(".git modified")
	}
}

func TestVersionJSON(t *testing.T) {
	m := result(execute(t, "", "version", "--json")).json(t)
	if m["format"] != "episodes/0.1" || m["version"] == nil || m["link_relations"] == nil {
		t.Fatal(m)
	}
}

// --- workspace -------------------------------------------------------------

func TestFreshInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	r := execute(t, "", "init", dir, "--author", ada, "--zone", mel, "--json")
	if r.code != 0 {
		t.Fatal(r)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "episodes.yaml"))
	for _, want := range []string{"format: episodes/0.1\n", "workspace:\n  id: ", "author: https://example.com/ada", "zone: Australia/Melbourne"} {
		if !strings.Contains(string(cfg), want) {
			t.Fatalf("missing %q in\n%s", want, cfg)
		}
	}
	idx, _ := os.ReadFile(filepath.Join(dir, "index.yaml"))
	if !strings.Contains(string(idx), "format: episodes/0.1") || !strings.Contains(string(idx), "entries: []") {
		t.Fatal(string(idx))
	}
	if fi, err := os.Stat(filepath.Join(dir, "episodes")); err != nil || !fi.IsDir() {
		t.Fatal("episodes/ missing")
	}
}

func TestInitOverExisting(t *testing.T) {
	e := newEnv(t)
	before := e.snapshot()
	r := execute(t, "", "init", e.dir, "--json")
	if r.code != 2 || e.snapshot() != before {
		t.Fatalf("%+v", r)
	}
}

func TestNewerFormatRefused(t *testing.T) {
	e := newEnv(t)
	_ = os.WriteFile(filepath.Join(e.dir, "episodes.yaml"), []byte("format: episodes/0.2\nworkspace:\n  id: x\n"), 0o644)
	r := e.run("list")
	if r.code != 1 || !strings.Contains(r.err, "episodes/0.2") {
		t.Fatalf("%+v", r)
	}
}

func TestPointerDiscovery(t *testing.T) {
	e := newEnv(t)
	app := t.TempDir()
	src := filepath.Join(app, "src")
	_ = os.MkdirAll(src, 0o755)
	r := execute(t, "", "workspace", "pointer", e.dir, "--at", app, "--json")
	if r.code != 0 {
		t.Fatal(r)
	}
	other := newEnv(t)
	if r := execute(t, "", "workspace", "pointer", other.dir, "--at", app, "--json"); r.code != 2 {
		t.Fatalf("overwrite without --force: %+v", r)
	}
	t.Chdir(src)
	m := result(execute(t, "", "workspace", "--json")).json(t)
	if m["found_by"] != "pointer" || !strings.HasSuffix(m["root"].(string), filepath.Base(e.dir)) {
		t.Fatal(m)
	}
}

func TestStrictEnvironmentVariable(t *testing.T) {
	t.Setenv("EPISODES_WORKSPACE", t.TempDir())
	if r := execute(t, "", "list", "--json"); r.code != 5 {
		t.Fatalf("%+v", r)
	}
}

func allCommands(c *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{c}
	for _, s := range c.Commands() {
		out = append(out, allCommands(s)...)
	}
	return out
}

// TestStandalone: the module imports nothing from its siblings.
func TestStandalone(t *testing.T) { standalone(t) }
