package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nodelogicau/episodes-cli/internal/store"
)

// --- recall ------------------------------------------------------------------

func TestShowWithChildren(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	e.turn(c, claude, "2026-11-07T03:01:00Z")
	e.turn(c, ada, "2026-11-07T03:02:00Z")
	m := e.ok("show", c, "--children")
	kids := m["children"].([]any)
	if len(kids) != 3 {
		t.Fatal(m)
	}
	for i, k := range kids {
		if k.(map[string]any)["ordinal"] != float64(i+1) {
			t.Fatalf("child %d: %v", i, k)
		}
	}
	r := execute(t, "", "show", c, "--children", "--workspace", e.dir)
	if strings.Count(r.out, "\n---\n") != 3 {
		t.Fatal(r.out)
	}
}

func TestWhatAdaSaidLastWeek(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-03T03:00:00Z")
	e.turn(c, claude, "2026-11-03T03:01:00Z")
	inRange := e.turn(c, ada, "2026-11-05T03:00:00Z")["id"].(string)
	e.turn(c, ada, "2026-11-10T03:00:00Z")
	old := e.ok("act", "--occurred", "2026-10-20T09:00:00+11:00", "--basis", "stated", "--now", "2026-11-03T05:00:00Z")["id"].(string)
	items := e.ok("list", "--standpoint", ada, "--since", "2026-11-02", "--until", "2026-11-09")["episodes"].([]any)
	var ids []string
	for _, it := range items {
		m := it.(map[string]any)
		if m["standpoint"] != ada {
			t.Fatal(m)
		}
		ids = append(ids, m["id"].(string))
	}
	if indexOf(ids, inRange) < 0 || indexOf(ids, old) >= 0 || len(ids) != 3 {
		t.Fatalf("%v", ids)
	}
	if ids[0] != c {
		t.Fatalf("not in occurred order: %v", ids)
	}
	// --by recorded orders by id.
	rec := e.ok("list", "--by", "recorded")["episodes"].([]any)
	for i := 1; i < len(rec); i++ {
		if rec[i-1].(map[string]any)["id"].(string) > rec[i].(map[string]any)["id"].(string) {
			t.Fatal("--by recorded is not id order")
		}
	}
}

func TestIndexLaggingAfterMerge(t *testing.T) {
	e := newEnv(t)
	e.note("2026-11-07T03:00:00Z")
	merged := idAt(at("2026-11-07T04:00:00Z"))
	e.put(merged, strings.Replace(handMade(merged, ""), "timestamp: 2026-11-07T03:24:40Z", "timestamp: 2026-11-07T04:00:00Z", 1))
	items := e.ok("list")["episodes"].([]any)
	if len(items) != 2 {
		t.Fatal(items)
	}
}

func TestThreadWithWithdrawnTurn(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	t2 := e.turn(c, claude, "2026-11-07T03:01:00Z", "--text", "secret words")["id"].(string)
	e.turn(c, ada, "2026-11-07T03:02:00Z")
	e.ok("redact", t2, "--fields", "content.text", "--reason", "r", "--now", "2026-11-07T04:00:00Z")
	m := e.ok("thread", c)
	kids := m["children"].([]any)
	if len(kids) != 3 {
		t.Fatal(m)
	}
	second := kids[1].(map[string]any)
	if second["withdrawn"] != true || second["text"] != nil || second["ordinal"] != 2.0 {
		t.Fatal(second)
	}
	if kids[0].(map[string]any)["text"] != "hello" {
		t.Fatal(kids[0])
	}
	r := execute(t, "", "thread", c, "--workspace", e.dir)
	if strings.Contains(r.out, "secret") || !strings.Contains(r.out, "[2] (withdrawn") {
		t.Fatal(r.out)
	}
}

func TestWithdrawnExcluded(t *testing.T) {
	e := newEnv(t)
	id := e.note("2026-11-07T03:00:00Z")
	e.ok("redact", id, "--fields", "content.text", "--reason", "r", "--now", "2026-11-07T04:00:00Z")
	if n := e.ok("list", "--kind", "note")["count"]; n != 0.0 {
		t.Fatal(n)
	}
	items := e.ok("list", "--kind", "note", "--include-withdrawn")["episodes"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["withdrawn"] != true {
		t.Fatal(items)
	}
}

func TestEmptyInterval(t *testing.T) {
	e := newEnv(t)
	e.note("2026-11-07T03:00:00Z")
	m := e.ok("list", "--since", "2025-01-01", "--until", "2025-01-31")
	if m["count"] != 0.0 || len(m["episodes"].([]any)) != 0 || len(m) != 2 {
		t.Fatal(m)
	}
	r := execute(t, "", "list", "--since", "2025-01-01", "--until", "2025-01-31", "--workspace", e.dir)
	if strings.TrimSpace(r.out) != "No episodes found." {
		t.Fatal(r.out)
	}
}

// --- sharding ----------------------------------------------------------------

func TestRecordedLongAfterItHappened(t *testing.T) {
	e := newEnv(t)
	m := e.ok("record", "ref", "--kind", "email", "--standpoint", jane, "--ref", "gmail:old", "--occurred", "2025-03-01T10:00:00Z", "--basis", "stated", "--now", "2026-11-07T03:00:00Z")
	if !strings.HasPrefix(m["path"].(string), "episodes/2026-11/") {
		t.Fatal(m)
	}
}

func TestMisplacedFile(t *testing.T) {
	e := newEnv(t)
	id := e.note("2026-11-07T03:00:00Z")
	from := filepath.Join(e.dir, filepath.FromSlash(store.RelPath(id)))
	to := filepath.Join(e.dir, "episodes", "2026-10", id+".yaml")
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	_ = os.Rename(from, to)
	rep, code := e.validate("2026-11-08T00:00:00Z")
	f := finding(rep, "misplaced")
	if code != 4 || f == nil || !strings.Contains(f["message"].(string), "episodes/2026-10/") || !strings.Contains(f["message"].(string), "episodes/2026-11/") {
		t.Fatal(rep)
	}
}

func TestGraceMonth(t *testing.T) {
	e := newEnv(t)
	e.note("2026-10-05T00:00:00Z")
	e.note("2026-11-05T00:00:00Z")
	e.note("2026-12-05T00:00:00Z")
	root := e.ok("index")
	_ = root
	idx, _ := os.ReadFile(filepath.Join(e.dir, "index.yaml"))
	if !strings.Contains(string(idx), "path: index/2026-10.yaml\n    count: 1") {
		t.Fatal(string(idx))
	}
	// November is open.
	e.note("2026-11-20T00:00:00Z")
}

func TestClockBehindSealedMonth(t *testing.T) {
	e := newEnv(t)
	e.note("2027-01-05T00:00:00Z")
	for _, args := range [][]string{
		{"record", "inline", "--kind", "note", "--standpoint", ada, "--text", "x"},
		{"act", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated"},
		{"conversation", "start", "--standpoint", ada, "--text", "x"},
	} {
		_, msg := e.fails(2, append(args, "--now", "2026-11-07T03:00:00Z")...)
		if !strings.Contains(msg, "2026-11") || !strings.Contains(msg, "clock") {
			t.Fatal(msg)
		}
	}
}

func TestLateMergeValidate(t *testing.T) {
	e := newEnv(t)
	e.note("2026-09-10T00:00:00Z")
	e.note("2026-11-07T00:00:00Z")
	late := idAt(at("2026-09-28T00:00:00Z"))
	e.put(late, handMade(late, ""))
	rep, code := e.validate("2026-11-08T00:00:00Z")
	f := finding(rep, "late_merge")
	if code != 4 || f == nil || !strings.Contains(f["message"].(string), late) || !strings.Contains(f["message"].(string), "re-mint") {
		t.Fatal(rep)
	}
	r := e.run("index")
	if r.code != 4 || !strings.Contains(r.out, late) {
		t.Fatal(r)
	}
}

func TestBrokenClocks(t *testing.T) {
	e := newEnv(t)
	far := idAt(at("2027-03-10T00:00:00Z"))
	e.put(far, handMade(far, ""))
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 4 || finding(rep, "id_far_future") == nil {
		t.Fatal(rep)
	}
	e2 := newEnv(t)
	soon := idAt(at("2026-11-09T00:00:00Z"))
	e2.put(soon, strings.Replace(handMade(soon, ""), "timestamp: 2026-11-07T03:24:40Z", "timestamp: 2026-11-09T00:00:00Z", 1))
	rep, code = e2.validate("2026-11-08T00:00:00Z")
	if code != 0 || finding(rep, "id_from_future") == nil || finding(rep, "timestamp_from_future") == nil {
		t.Fatal(rep)
	}
}

// --- index -------------------------------------------------------------------

func TestFirstEpisodeOfNewMonth(t *testing.T) {
	e := newEnv(t)
	e.note("2026-10-03T00:00:00Z")
	e.note("2026-10-20T00:00:00Z")
	e.note("2026-11-07T00:00:00Z")
	jan := e.note("2027-01-02T00:00:00Z")
	idx, _ := os.ReadFile(filepath.Join(e.dir, "index.yaml"))
	s := string(idx)
	if !strings.Contains(s, "  - path: index/2026-10.yaml\n    count: 2\n  - path: index/2026-11.yaml\n    count: 1\n") || strings.Contains(s, "2026-12") || strings.Count(s, "- id: ") != 1 || !strings.Contains(s, jan) {
		t.Fatal(s)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "index", "2026-12.yaml")); !os.IsNotExist(err) {
		t.Fatal("December segment written")
	}
}

func TestTurnEntryShape(t *testing.T) {
	e := newEnv(t)
	c, turn := e.start("2026-11-07T03:00:00Z", "--label", "x", "--link", "related=https://example.com")
	_ = c
	idx, _ := os.ReadFile(filepath.Join(e.dir, "index.yaml"))
	s := string(idx)
	i := strings.Index(s, "- id: "+turn)
	entry := s[i:]
	for _, bad := range []string{"label", "content", "links", "participants", "source"} {
		if strings.Contains(entry, bad+":") {
			t.Fatalf("entry carries %s:\n%s", bad, entry)
		}
	}
	if !strings.Contains(entry, `occurred: {start: "`) || !strings.Contains(entry, "ordinal: 1") {
		t.Fatal(entry)
	}
}

func TestIndexConflictMarkersAndDrift(t *testing.T) {
	e := newEnv(t)
	e.note("2026-11-07T03:00:00Z")
	merged := idAt(at("2026-11-07T04:00:00Z"))
	e.put(merged, handMade(merged, ""))
	r := e.run("index", "--check")
	if r.code != 4 || !strings.Contains(r.out, merged) {
		t.Fatal(r)
	}
	_ = os.WriteFile(filepath.Join(e.dir, "index.yaml"), []byte("<<<<<<< HEAD\nentries: []\n=======\n>>>>>>> branch\n"), 0o644)
	e.ok("index")
	if r := e.run("index", "--check"); r.code != 0 {
		t.Fatal(r)
	}
}

func TestRebuildLeavesSegmentsAlone(t *testing.T) {
	e := newEnv(t)
	e.note("2026-09-07T03:00:00Z")
	e.note("2026-11-07T03:00:00Z")
	seg := filepath.Join(e.dir, "index", "2026-09.yaml")
	before, err := os.ReadFile(seg)
	if err != nil {
		t.Fatal(err)
	}
	e.ok("index")
	after, _ := os.ReadFile(seg)
	if string(before) != string(after) {
		t.Fatal("segment rewritten")
	}
}

func TestEditedOccurredIsRevised(t *testing.T) {
	e := newEnv(t)
	id := e.note("2026-11-07T03:00:00Z")
	p := filepath.Join(e.dir, filepath.FromSlash(store.RelPath(id)))
	data, _ := os.ReadFile(p)
	_ = os.WriteFile(p, []byte(strings.Replace(string(data), `start: "2026-11-07T14:00:00+11:00"`, `start: "2026-11-07T13:00:00+11:00"`, 1)), 0o644)
	rep, code := e.validate("2026-11-08T00:00:00Z")
	f := finding(rep, "revised")
	if code != 4 || f == nil || f["id"] != id || f["field"] != "occurred" {
		t.Fatal(rep)
	}
}

// --- validation --------------------------------------------------------------

func TestCleanWorkspace(t *testing.T) {
	e := newEnv(t, "--zone", mel)
	c, turn := e.start("2026-11-07T03:00:00Z")
	e.turn(c, claude, "2026-11-07T03:01:00Z")
	e.ok("act", "--occurred", "2026-11-07T13:00:00+11:00[Australia/Melbourne]", "--basis", "stated", "--via", turn, "--harness", "claude", "--now", "2026-11-07T03:02:00Z")
	if r := e.email(ada, "a@b", "2026-11-07T04:00:00Z"); r.code != 0 {
		t.Fatal(r)
	}
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 0 || rep["errors"] != 0.0 || len(rep["findings"].([]any)) != 0 {
		t.Fatal(rep)
	}
}

func TestSeveralErrorsTogether(t *testing.T) {
	e := newEnv(t)
	ref := idAt(at("2026-11-07T03:00:00Z"))
	e.put(ref, strings.Replace(handMade(ref, ""), "  mode: inline\n  media_type: text/markdown\n  derivation: verbatim\n", "  mode: ref\n  ref: gmail:1\n", 1))
	id := e.note("2026-11-07T04:00:00Z")
	from := filepath.Join(e.dir, filepath.FromSlash(store.RelPath(id)))
	_ = os.MkdirAll(filepath.Join(e.dir, "episodes", "2026-10"), 0o755)
	_ = os.Rename(from, filepath.Join(e.dir, "episodes", "2026-10", id+".yaml"))
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 4 || finding(rep, "text_on_ref") == nil || finding(rep, "misplaced") == nil {
		t.Fatal(rep)
	}
}

func TestOrdinalRepeatedWarning(t *testing.T) {
	e := newEnv(t)
	c, _ := e.start("2026-11-07T03:00:00Z")
	a := e.turn(c, ada, "2026-11-07T03:01:00Z", "--ordinal", "5")["id"].(string)
	b := idAt(at("2026-11-07T03:02:00Z"))
	body := strings.Replace(e.file(a), a, b, 1)
	e.put(b, body)
	rep, code := e.validate("2026-11-08T00:00:00Z")
	f := finding(rep, "ordinal_repeated")
	if code != 0 || f == nil || f["severity"] != "warning" || !strings.Contains(f["message"].(string), a) || !strings.Contains(f["message"].(string), b) {
		t.Fatal(rep)
	}
}

func TestPossibleDuplicateInfo(t *testing.T) {
	e := newEnv(t)
	first := e.email(ada, "a@b", "2026-11-07T04:00:00Z").json(t)["id"].(string)
	second := e.email(ada, "a@b", "2026-11-07T04:01:00Z", "--allow-duplicate").json(t)["id"].(string)
	rep, code := e.validate("2026-11-08T00:00:00Z")
	f := finding(rep, "possible_duplicate")
	if code != 0 || f == nil || f["severity"] != "info" || !strings.Contains(f["message"].(string), first) || !strings.Contains(f["message"].(string), second) {
		t.Fatal(rep)
	}
}

func TestReferenceToDKFIDOpaque(t *testing.T) {
	e := newEnv(t)
	e.note("2026-11-07T03:00:00Z", "--participant", "clm_01a1b4b2-3c1d-7e5a-9f02-6b8c4d2e1a37")
	rep, code := e.validate("2026-11-08T00:00:00Z")
	if code != 0 || len(rep["findings"].([]any)) != 0 {
		t.Fatal(rep)
	}
}

func TestLinkTargetDropped(t *testing.T) {
	e := newEnv(t)
	a := e.note("2026-11-07T03:00:00Z")
	b := e.note("2026-11-07T03:01:00Z", "--link", "predecessor-version="+a)
	_ = os.Remove(filepath.Join(e.dir, filepath.FromSlash(store.RelPath(a))))
	e.ok("index")
	rep, code := e.validate("2026-11-08T00:00:00Z")
	f := finding(rep, "link_unresolved")
	if code != 0 || f == nil || !strings.Contains(f["message"].(string), a) || !strings.Contains(f["message"].(string), b) {
		t.Fatal(rep)
	}
}

func TestLeftoverOccasion(t *testing.T) {
	e := newEnv(t)
	id := idAt(at("2026-11-07T03:00:00Z"))
	body := strings.Replace(handMade(id, "occasion: int_X\n"), "  mode: inline\n  media_type: text/markdown\n  derivation: verbatim\n  text: hi\n", "  mode: ref\n  ref: drive:x\n", 1)
	e.put(id, body)
	if rep, code := e.validate("2026-11-08T00:00:00Z"); code != 0 {
		t.Fatal(rep)
	}
	e.ok("observe", id, "--state", "gone", "--now", "2026-11-08T00:00:00Z")
	if !strings.Contains(e.file(id), "\noccasion: int_X\nsource_status:") {
		t.Fatal(e.file(id))
	}
}

func TestReorderedInputReEmittedByObserve(t *testing.T) {
	e := newEnv(t)
	id := idAt(at("2026-11-07T03:00:00Z"))
	body := "timestamp: 2026-11-07T03:24:40Z\nsource:\n  author: https://example.com/ada\ncontent:\n  ref: drive:x\n  mode: ref\nstandpoint: https://example.com/ada\noccurred:\n  basis: stated\n  start: \"2026-11-07T14:24:00+11:00\"\nkind: note\ntype: episode\nid: " + id + "\n"
	e.put(id, body)
	e.ok("observe", id, "--state", "available", "--now", "2026-11-08T00:00:00Z")
	if !strings.HasPrefix(e.file(id), "id: "+id+"\ntype: episode\nkind: note\noccurred:\n  start:") {
		t.Fatal(e.file(id))
	}
}
