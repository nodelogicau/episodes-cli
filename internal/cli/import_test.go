package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nodelogicau/episodes-cli/internal/store"
)

const importNow = "2026-11-21T01:00:00Z"

// importLines joins records as NDJSON.
func importLines(recs ...map[string]any) string {
	var b strings.Builder
	for _, r := range recs {
		data, _ := json.Marshal(r)
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func refRec(n int, extra map[string]any) map[string]any {
	r := map[string]any{"kind": "email", "standpoint": jane,
		"occurred": map[string]any{"start": "2026-11-07T14:24:00+11:00", "basis": "stated"},
		"content":  map[string]any{"mode": "ref", "ref": fmt.Sprintf("gmail:%d", n), "uid": fmt.Sprintf("mid:%d@example.com", n), "media_type": "message/rfc822"}}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

func noteRec(extra map[string]any) map[string]any {
	r := map[string]any{"kind": "note", "standpoint": ada, "content": map[string]any{"mode": "inline", "text": "a note"}}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

// importRun imports stdin with --json and returns the result and exit code.
func (e *env) importRun(stdin string, args ...string) (map[string]any, int) {
	e.t.Helper()
	r := e.runIn(stdin, append([]string{"import", "--now", importNow}, args...)...)
	if r.code != 0 && r.code != 2 {
		e.t.Fatalf("import: exit %d\n%s%s", r.code, r.out, r.err)
	}
	if r.out == "" {
		e.t.Fatalf("import: no result (exit %d): %s", r.code, r.err)
	}
	return r.json(e.t), r.code
}

func resultLines(m map[string]any) []map[string]any {
	var out []map[string]any
	for _, l := range m["lines"].([]any) {
		out = append(out, l.(map[string]any))
	}
	return out
}

func lineAt(m map[string]any, n int) map[string]any {
	for _, l := range resultLines(m) {
		if int(l["line"].(float64)) == n {
			return l
		}
	}
	return nil
}

func counts(m map[string]any) (w, s, f int) {
	return int(m["written"].(float64)), int(m["skipped"].(float64)), int(m["failed"].(float64))
}

func (e *env) episodeCount() int {
	e.t.Helper()
	n := 0
	_ = filepath.Walk(filepath.Join(e.dir, store.EpisodesDir), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".yaml") {
			n++
		}
		return nil
	})
	return n
}

// import spec: A batch of references.
func TestImportBatchOfRefs(t *testing.T) {
	e := newEnv(t)
	m, code := e.importRun(importLines(refRec(1, nil), refRec(2, nil), refRec(3, nil)))
	if code != 0 {
		t.Fatalf("exit %d: %v", code, m)
	}
	lines := resultLines(m)
	if w, _, _ := counts(m); w != 3 || len(lines) != 3 {
		t.Fatalf("%v", m)
	}
	prev := ""
	for i, l := range lines {
		id := l["id"].(string)
		if int(l["line"].(float64)) != i+1 || l["status"] != "written" || l["path"] != store.RelPath(id) || id <= prev {
			t.Fatalf("line %d: %v (prev %s)", i+1, l, prev)
		}
		prev = id
		f := e.file(id)
		if !strings.Contains(f, "timestamp: "+importNow+"\n") || !strings.Contains(f, "source:\n  author: "+ada+"\n") {
			t.Fatalf("%s", f)
		}
	}
	if r := e.run("index", "--check"); r.code != 0 {
		t.Fatalf("index drift: %s", r.out)
	}
}

// import spec: A copied episode refused, and the other keys import supplies
// or the format does not define.
func TestImportRefusesCopiedFields(t *testing.T) {
	e := newEnv(t)
	cases := map[string]map[string]any{
		"id":            noteRec(map[string]any{"id": idAt(at("2026-11-07T03:00:00Z"))}),
		"type":          noteRec(map[string]any{"type": "episode"}),
		"source":        noteRec(map[string]any{"source": map[string]any{"author": ada}}),
		"timestamp":     noteRec(map[string]any{"timestamp": importNow}),
		"source_status": refRec(1, map[string]any{"source_status": map[string]any{"state": "available"}}),
		"redacted":      noteRec(map[string]any{"redacted": map[string]any{"fields": []string{"label"}}}),
		"summary":       noteRec(map[string]any{"summary": "an undefined key"}),
		"content.body":  noteRec(map[string]any{"content": map[string]any{"mode": "inline", "text": "x", "body": "y"}}),
	}
	for key, rec := range cases {
		t.Run(key, func(t *testing.T) {
			before := e.snapshot()
			m, code := e.importRun(importLines(noteRec(nil), rec))
			l := lineAt(m, 2)
			if code != 2 || l["status"] != "failed" || l["code"] != "refused" || !strings.Contains(l["message"].(string), key) {
				t.Fatalf("exit %d: %v", code, m)
			}
			if lineAt(m, 1) != nil || e.snapshot() != before {
				t.Fatal("stop wrote or reported the good line")
			}
		})
	}
	m, _ := e.importRun("not json\n[1]\n")
	if lineAt(m, 1)["code"] != "invalid" || lineAt(m, 2)["code"] != "invalid" {
		t.Fatalf("%v", m)
	}
}

// import spec: An email and its attachments.
func TestImportEmailAndAttachments(t *testing.T) {
	e := newEnv(t)
	att := func(n int) map[string]any {
		return map[string]any{"kind": "file", "standpoint": jane, "container": "@m1", "ordinal": "auto",
			"occurred": map[string]any{"start": "2026-11-07T14:24:00+11:00", "basis": "stated"},
			"content":  map[string]any{"mode": "ref", "ref": fmt.Sprintf("gmail:1/att%d", n), "media_type": "application/pdf"}}
	}
	m, code := e.importRun(importLines(refRec(1, map[string]any{"@key": "m1"}), att(1), att(2)))
	if code != 0 {
		t.Fatalf("%v", m)
	}
	email := lineAt(m, 1)["id"].(string)
	if lineAt(m, 1)["key"] != "m1" {
		t.Fatalf("%v", m)
	}
	for n, want := range map[int]string{2: "ordinal: 1\n", 3: "ordinal: 2\n"} {
		f := e.file(lineAt(m, n)["id"].(string))
		if !strings.Contains(f, "container: "+email+"\n"+want) {
			t.Fatalf("line %d:\n%s", n, f)
		}
	}
	// A later batch counts the workspace's children too.
	m, _ = e.importRun(importLines(att(3)), "--on-duplicate", "skip")
	if l := lineAt(m, 1); l["code"] != "refused" || !strings.Contains(l["message"].(string), "@m1") {
		t.Fatalf("a key is local to its batch: %v", m)
	}
	m, _ = e.importRun(importLines(map[string]any{"kind": "file", "standpoint": jane, "container": email, "ordinal": "auto",
		"occurred": map[string]any{"start": "2026-11-07T14:24:00+11:00", "basis": "stated"},
		"content":  map[string]any{"mode": "ref", "ref": "gmail:1/att3"}}))
	if f := e.file(lineAt(m, 1)["id"].(string)); !strings.Contains(f, "ordinal: 3\n") {
		t.Fatalf("%s", f)
	}
}

// import spec: Forward reference, and the other key failures.
func TestImportKeys(t *testing.T) {
	e := newEnv(t)
	child := func(key string) map[string]any {
		return map[string]any{"kind": "message", "standpoint": ada, "container": "@" + key, "content": map[string]any{"mode": "inline", "text": "hi"}}
	}
	conv := func(key string) map[string]any {
		return map[string]any{"@key": key, "kind": "conversation", "standpoint": ada, "content": map[string]any{"mode": "none"}}
	}
	m, code := e.importRun(importLines(child("later"), conv("later"), child("later")))
	if l := lineAt(m, 1); code != 2 || l["code"] != "refused" || !strings.Contains(l["message"].(string), "@later") || !strings.Contains(l["message"].(string), "after it") {
		t.Fatalf("%v", m)
	}
	m, _ = e.importRun(importLines(child("nowhere")))
	if l := lineAt(m, 1); !strings.Contains(l["message"].(string), "@nowhere") || !strings.Contains(l["message"].(string), "no line") {
		t.Fatalf("%v", m)
	}
	m, _ = e.importRun(importLines(conv("c"), child("c"), conv("c"), child("c")))
	if l := lineAt(m, 3); l["code"] != "refused" || !strings.Contains(l["message"].(string), "already defined on line 1") {
		t.Fatalf("%v", m)
	}
	// A link may name a key too.
	m, code = e.importRun(importLines(noteRec(map[string]any{"@key": "n"}), noteRec(map[string]any{"links": []any{map[string]any{"rel": "related", "href": "@n"}}})))
	if code != 0 || !strings.Contains(e.file(lineAt(m, 2)["id"].(string)), "href: "+lineAt(m, 1)["id"].(string)) {
		t.Fatalf("%v", m)
	}
}

// import spec: One bad line stops the batch; Skip writes the good lines.
func TestImportStopAndSkip(t *testing.T) {
	e := newEnv(t)
	var recs []map[string]any
	for i := 1; i <= 10; i++ {
		r := noteRec(map[string]any{"label": fmt.Sprintf("note %d", i)})
		if i == 7 {
			r["occurred"] = map[string]any{"start": "last tuesday", "basis": "stated"}
		}
		recs = append(recs, r)
	}
	in := importLines(recs...)
	before := e.snapshot()
	m, code := e.importRun(in)
	if w, _, f := counts(m); code != 2 || w != 0 || f != 1 || lineAt(m, 7)["status"] != "failed" || len(resultLines(m)) != 1 {
		t.Fatalf("exit %d: %v", code, m)
	}
	if e.snapshot() != before {
		t.Fatal("stop wrote")
	}
	m, code = e.importRun(in, "--on-error", "skip")
	if w, _, f := counts(m); code != 2 || w != 9 || f != 1 || lineAt(m, 7)["status"] != "failed" || e.episodeCount() != 9 {
		t.Fatalf("exit %d: %v", code, m)
	}
	// dependency_failed follows @key references, recursively.
	bad := map[string]any{"@key": "c", "kind": "conversation", "standpoint": "", "content": map[string]any{"mode": "none"}}
	turn := map[string]any{"@key": "t", "kind": "message", "standpoint": ada, "container": "@c", "ordinal": "auto", "content": map[string]any{"mode": "inline", "text": "hi"}}
	reply := noteRec(map[string]any{"links": []any{map[string]any{"rel": "related", "href": "@t"}}})
	m, _ = e.importRun(importLines(bad, turn, reply, noteRec(nil)), "--on-error", "skip")
	if lineAt(m, 2)["code"] != "dependency_failed" || lineAt(m, 3)["code"] != "dependency_failed" || lineAt(m, 4)["status"] != "written" {
		t.Fatalf("%v", m)
	}
}

// import spec: Idempotent re-run, and refuse and allow.
func TestImportDuplicates(t *testing.T) {
	e := newEnv(t)
	var recs []map[string]any
	for i := 0; i < 100; i++ {
		recs = append(recs, refRec(i, nil))
	}
	in := importLines(recs...)
	first, code := e.importRun(in)
	if w, _, _ := counts(first); code != 0 || w != 100 {
		t.Fatalf("%v", first)
	}
	before := e.snapshot()
	m, code := e.importRun(in, "--on-duplicate", "skip")
	if w, s, f := counts(m); code != 0 || w != 0 || s != 100 || f != 0 {
		t.Fatalf("exit %d: %d %d %d", code, w, s, f)
	}
	for i, l := range resultLines(m) {
		if l["status"] != "skipped" || l["id"] != resultLines(first)[i]["id"] {
			t.Fatalf("line %d: %v", i+1, l)
		}
	}
	if e.snapshot() != before {
		t.Fatal("re-run wrote")
	}
	m, code = e.importRun(importLines(refRec(5, nil)))
	if l := lineAt(m, 1); code != 2 || l["code"] != "duplicate" || !strings.Contains(l["message"].(string), resultLines(first)[5]["id"].(string)) {
		t.Fatalf("%v", m)
	}
	if m, code = e.importRun(importLines(refRec(5, nil)), "--on-duplicate", "allow"); code != 0 || lineAt(m, 1)["status"] != "written" {
		t.Fatalf("%v", m)
	}
	// Within the batch, the earlier line is the existing one.
	m, _ = e.importRun(importLines(refRec(200, nil), refRec(200, map[string]any{"content": map[string]any{"mode": "ref", "ref": "imap:x", "uid": "mid:200@example.com", "media_type": "MESSAGE/RFC822"}})))
	if l := lineAt(m, 2); l["code"] != "duplicate" || !strings.Contains(l["message"].(string), "already record") {
		t.Fatalf("%v", m)
	}
	m, _ = e.importRun(importLines(refRec(201, nil), refRec(201, nil)), "--on-duplicate", "skip")
	if l1, l2 := lineAt(m, 1), lineAt(m, 2); l1["status"] != "written" || l2["status"] != "skipped" || l2["id"] != l1["id"] {
		t.Fatalf("%v", m)
	}
}

// import spec: Act refused; Childless container refused.
func TestImportActAndChildless(t *testing.T) {
	e := newEnv(t)
	act := map[string]any{"kind": "activity", "standpoint": ada, "occurred": map[string]any{"start": "2026-11-07", "basis": "stated"}, "content": map[string]any{"mode": "attested"}}
	m, code := e.importRun(importLines(act))
	if l := lineAt(m, 1); code != 2 || l["code"] != "refused" || !strings.Contains(l["message"].(string), "act") {
		t.Fatalf("%v", m)
	}
	conv := map[string]any{"@key": "c", "kind": "conversation", "standpoint": ada, "content": map[string]any{"mode": "none"}}
	m, _ = e.importRun(importLines(conv, noteRec(nil)))
	if l := lineAt(m, 1); l["code"] != "refused" || !strings.Contains(l["message"].(string), "no children") || !strings.Contains(l["message"].(string), "@<its key>") {
		t.Fatalf("%v", m)
	}
	turn := map[string]any{"kind": "message", "standpoint": ada, "container": "@c", "ordinal": "auto", "content": map[string]any{"mode": "inline", "text": "hi"}}
	if m, code = e.importRun(importLines(conv, turn)); code != 0 {
		t.Fatalf("%v", m)
	}
	if rep, code := e.validate("2026-11-21T02:00:00Z"); code != 0 || len(findings(rep, "warning")) != 0 {
		t.Fatalf("%v", rep)
	}
}

// import spec: Mapping lines to ids, and the --ndjson and text forms.
func TestImportResultForms(t *testing.T) {
	e := newEnv(t)
	r := execute(t, importLines(noteRec(nil), noteRec(nil)), "import", "--ndjson", "--now", importNow, "--workspace", e.dir)
	rows := strings.Split(strings.TrimSpace(r.out), "\n")
	if r.code != 0 || len(rows) != 3 {
		t.Fatalf("%+v", r)
	}
	var last map[string]any
	_ = json.Unmarshal([]byte(rows[2]), &last)
	if last["written"] != 2.0 || last["failed"] != 0.0 {
		t.Fatalf("%s", rows[2])
	}
	r = execute(t, importLines(noteRec(nil)), "import", "--now", importNow, "--workspace", e.dir)
	if r.code != 0 || !strings.HasPrefix(r.out, "Imported 1 written, 0 skipped, 0 failed\n") {
		t.Fatalf("%+v", r)
	}
	if code, _ := e.fails(2, "import", "--ndjson"); code != "usage" {
		t.Fatal(code)
	}
	path := filepath.Join(t.TempDir(), "batch.ndjson")
	_ = os.WriteFile(path, []byte(importLines(noteRec(nil))), 0o644)
	if r := e.run("import", path, "--now", importNow); r.code != 0 {
		t.Fatalf("%+v", r)
	}
}

// import 4.2: an imported episode is the one record writes from the same
// inputs, --now and id.
func TestImportMatchesRecord(t *testing.T) {
	e := newEnv(t, "--zone", mel)
	cases := []struct {
		args []string
		rec  map[string]any
	}{
		{[]string{"record", "inline", "--kind", "note", "--standpoint", ada, "--text", "a note\nover two lines\n", "--label", "Plan",
			"--participant", jane, "--participant", ada, "--place", "geo:-37.8,144.9", "--language", "EN-au"},
			map[string]any{"kind": "note", "standpoint": ada, "label": "Plan", "participants": []string{jane, ada}, "place": "geo:-37.8,144.9",
				"content": map[string]any{"mode": "inline", "text": "a note\nover two lines\n", "language": "EN-au"}}},
		{[]string{"record", "ref", "--kind", "email", "--standpoint", jane, "--ref", "gmail:1", "--message-id", "<1@example.com>", "--media-type", "message/rfc822",
			"--occurred", "2026-11-07T14:24:00+11:00[Australia/Melbourne]", "--basis", "stated", "--link", "related=https://example.com/x", "--version", "7"},
			map[string]any{"kind": "email", "standpoint": jane, "occurred": map[string]any{"start": "2026-11-07T14:24:00+11:00[Australia/Melbourne]", "basis": "stated"},
				"links":   []any{map[string]any{"rel": "related", "href": "https://example.com/x"}},
				"content": map[string]any{"mode": "ref", "ref": "gmail:1", "uid": "mid:1@example.com", "media_type": "message/rfc822", "version": "7"}}},
	}
	for i, c := range cases {
		want := e.ok(append(c.args, "--now", importNow, "--allow-duplicate")[:len(c.args)+2]...)
		m, code := e.importRun(importLines(c.rec), "--on-duplicate", "allow")
		if code != 0 {
			t.Fatalf("case %d: %v", i, m)
		}
		got := lineAt(m, 1)["id"].(string)
		wantID := want["id"].(string)
		if a, b := e.file(got), strings.ReplaceAll(e.file(wantID), wantID, got); a != b {
			t.Fatalf("case %d differs\n--- import\n%s\n--- record\n%s", i, a, b)
		}
	}
}

// import 4.3: a record running alongside a multi-chunk import completes,
// and both are indexed.
func TestImportConcurrentRecord(t *testing.T) {
	defer func(n int) { importChunkSize = n }(importChunkSize)
	importChunkSize = 25
	e := newEnv(t)
	var recs []map[string]any
	for i := 0; i < 200; i++ {
		recs = append(recs, refRec(i, nil))
	}
	var wg sync.WaitGroup
	codes := make(chan int, 6)
	wg.Add(1)
	go func() {
		defer wg.Done()
		codes <- execute(t, importLines(recs...), "import", "--workspace", e.dir, "--json").code
	}()
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- execute(t, "", "record", "inline", "--kind", "note", "--standpoint", ada, "--text", "hook", "--workspace", e.dir, "--json").code
		}()
	}
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != 0 {
			t.Fatalf("a writer exited %d", c)
		}
	}
	if r := e.run("index", "--check"); r.code != 0 {
		t.Fatalf("index drift: %s", r.out)
	}
	if n := e.episodeCount(); n != 205 {
		t.Fatalf("%d episodes", n)
	}
	if m := e.ok("list"); m["count"] != 205.0 {
		t.Fatalf("listed %v", m["count"])
	}
}

// import 4.4: an ordinal grabbed between chunks fails the recheck; under
// stop the chunks already written are reported and nothing more is written.
func TestImportRecheckFailure(t *testing.T) {
	defer func(n int, f func(int)) { importChunkSize, importBeforeChunk = n, f }(importChunkSize, importBeforeChunk)
	importChunkSize = 2
	e := newEnv(t)
	conv, _ := e.start("2026-11-21T00:00:00Z")
	var recs []map[string]any
	for o := 2; o <= 5; o++ {
		recs = append(recs, map[string]any{"kind": "message", "standpoint": ada, "container": conv, "ordinal": o, "content": map[string]any{"mode": "inline", "text": "hi"}})
	}
	importBeforeChunk = func(chunk int) {
		if chunk == 1 {
			e.turn(conv, jane, "2026-11-21T00:30:00Z", "--ordinal", "4")
		}
	}
	m, code := e.importRun(importLines(recs...))
	if w, _, f := counts(m); code != 2 || w != 2 || f != 1 {
		t.Fatalf("exit %d: %v", code, m)
	}
	if lineAt(m, 1)["status"] != "written" || lineAt(m, 2)["status"] != "written" || lineAt(m, 3)["code"] != "duplicate" || lineAt(m, 4) != nil {
		t.Fatalf("%v", m)
	}
	if n := e.episodeCount(); n != 5 {
		t.Fatalf("%d episodes", n)
	}
}

// --- list --ndjson ------------------------------------------------------

func ndjsonRows(t *testing.T, out string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("%v: %q", err, line)
		}
		rows = append(rows, m)
	}
	return rows
}

func (e *env) listNDJSON(args ...string) []map[string]any {
	e.t.Helper()
	r := execute(e.t, "", append([]string{"list", "--ndjson", "--workspace", e.dir}, args...)...)
	if r.code != 0 {
		e.t.Fatalf("exit %d: %s", r.code, r.err)
	}
	return ndjsonRows(e.t, r.out)
}

// recall spec: Not mixed with JSON.
func TestListNDJSONNotWithJSON(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.fails(2, "list", "--ndjson"); code != "usage" {
		t.Fatal(code)
	}
	if r := execute(t, "", "list", "--fields", "index", "--workspace", e.dir); r.code != 2 {
		t.Fatalf("%+v", r)
	}
}

// recall spec: the two field sets, the default order, withdrawn episodes
// and --limit.
func TestListNDJSON(t *testing.T) {
	e := newEnv(t)
	a := e.note("2026-11-07T03:00:00Z", "--occurred", "2026-11-07T09:00:00+11:00", "--basis", "stated")
	b := e.note("2026-11-07T04:00:00Z", "--occurred", "2026-11-06T09:00:00+11:00", "--basis", "stated")
	e.ok("redact", b, "--fields", "content.text", "--reason", "withdrawn", "--now", "2026-11-07T05:00:00Z")
	full := e.listNDJSON()
	if len(full) != 1 || full[0]["id"] != a || full[0]["content"].(map[string]any)["text"] != "a note" {
		t.Fatalf("%v", full)
	}
	if got := e.listNDJSON("--include-withdrawn"); len(got) != 2 || got[0]["id"] != a {
		t.Fatalf("by recorded: %v", got)
	}
	idx := e.listNDJSON("--fields", "index")
	if len(idx) != 2 || idx[1]["id"] != b || idx[1]["path"] != store.RelPath(b) || len(idx[1]) != 6 {
		t.Fatalf("index mode lists withdrawn episodes, seven fields less the absent container and ordinal, plus path: %v", idx)
	}
	if _, ok := idx[0]["content"]; ok {
		t.Fatalf("%v", idx[0])
	}
	if got := e.listNDJSON("--fields", "index", "--by", "occurred"); got[0]["id"] != b {
		t.Fatalf("by occurred: %v", got)
	}
	if got := e.listNDJSON("--fields", "index", "--limit", "1"); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	conv, turn := e.start("2026-11-07T06:00:00Z")
	if got := e.listNDJSON("--fields", "index", "--container", conv); len(got) != 1 || got[0]["id"] != turn || got[0]["ordinal"] != 1.0 {
		t.Fatalf("%v", got)
	}
}

// recall spec: Index fast path (no episode file is opened) and Stale index
// streamed.
func TestListNDJSONIndexFastPath(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs file permissions that deny the owner")
	}
	e := newEnv(t)
	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, e.note(fmt.Sprintf("2026-11-07T0%d:00:00Z", i)))
	}
	late := idAt(at("2026-11-07T06:00:00Z"))
	e.put(late, handMade(late, "")) // merged, missing from index.yaml
	for _, id := range ids {
		p := filepath.Join(e.dir, filepath.FromSlash(store.RelPath(id)))
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chmod(p, 0o644) }()
	}
	got := e.listNDJSON("--fields", "index")
	if len(got) != 6 || got[5]["id"] != late {
		t.Fatalf("%v", got)
	}
	for i, id := range ids {
		if got[i]["id"] != id {
			t.Fatalf("row %d: %v", i, got[i])
		}
	}
	if full := e.listNDJSON(); len(full) != 1 || full[0]["id"] != late {
		t.Fatalf("full mode parses files, and these are unreadable: %v", full)
	}
}
