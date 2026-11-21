package cli

import (
	"reflect"
	"strings"
	"testing"
)

// queryIDs runs query and returns the result ids in order, checking the
// result's shape on the way.
func (e *env) queryIDs(args ...string) []string {
	e.t.Helper()
	m := e.ok(append([]string{"query"}, args...)...)
	items := m["episodes"].([]any)
	if int(m["count"].(float64)) != len(items) {
		e.t.Fatalf("count %v for %d items", m["count"], len(items))
	}
	ids := []string{}
	for _, it := range items {
		x := it.(map[string]any)
		if r := x["resolution"]; r != "uid" && r != "lexical" {
			e.t.Fatalf("resolution %v", r)
		}
		if _, ok := x["matched"].(float64); !ok {
			e.t.Fatalf("matched %v", x["matched"])
		}
		ids = append(ids, x["id"].(string))
	}
	return ids
}

func (e *env) inlineNote(now, text string, extra ...string) string {
	e.t.Helper()
	return e.note(now, append([]string{"--text", text}, extra...)...)
}

// recall-by-query, spec "Query normalisation".
func TestQueryNormalisation(t *testing.T) {
	e := newEnv(t)
	inv := e.inlineNote("2026-11-07T03:00:00Z", "the invoice")
	ada := e.inlineNote("2026-11-07T03:01:00Z", "a word with Ada")
	mail := e.inlineNote("2026-11-07T03:02:00Z", "send an email")
	e.inlineNote("2026-11-07T03:03:00Z", "nothing relevant")

	m := e.ok("query", `"invoice?"  (Ada)`)
	if m["query"] != `"invoice?"  (Ada)` {
		t.Fatal(m["query"])
	}
	if got := e.queryIDs(`"invoice?"  (Ada)`); !reflect.DeepEqual(got, []string{ada, inv}) {
		t.Fatalf("edges: %v", got)
	}
	if got := e.queryIDs("e-mail"); len(got) != 0 {
		t.Fatalf("e-mail matched %v", got)
	}
	if got := e.queryIDs("email"); !reflect.DeepEqual(got, []string{mail}) {
		t.Fatalf("email: %v", got)
	}
	for _, q := range []string{"  ?! ", ""} {
		if got := e.queryIDs(q); len(got) != 0 {
			t.Fatalf("%q: %v", q, got)
		}
	}
	// Several arguments are one query.
	if got := e.queryIDs("invoice", "Ada"); !reflect.DeepEqual(got, []string{ada, inv}) {
		t.Fatalf("args: %v", got)
	}
	if r := e.run("query"); r.code != 2 {
		t.Fatalf("no text: %+v", r)
	}
}

// recall-by-query, spec "Lexical and uid matching".
func TestQueryMatching(t *testing.T) {
	e := newEnv(t)
	paid := e.inlineNote("2026-11-07T03:00:00Z", "Paid the INVOICE today")
	receipt := e.inlineNote("2026-11-07T03:01:00Z", "a receipt")
	labelled := e.inlineNote("2026-11-07T03:02:00Z", "no match here", "--label", "Invoice from Ada")
	ref := e.ok("record", "ref", "--kind", "email", "--standpoint", ada, "--ref", "gmail:abc",
		"--uid", "<abc@example.com>", "--media-type", "message/rfc822", "--label", "receipt attached",
		"--occurred", "2026-11-07T14:24:00+11:00", "--basis", "stated", "--now", "2026-11-07T03:30:00Z")["id"].(string)

	if got := e.queryIDs("invoice"); !reflect.DeepEqual(got, []string{labelled, paid}) {
		t.Fatalf("invoice: %v", got)
	}
	if got := e.queryIDs("invoice receipt"); !reflect.DeepEqual(got, []string{ref, labelled, receipt, paid}) {
		t.Fatalf("any term: %v", got)
	}
	m := e.ok("query", "<abc@example.com>")
	items := m["episodes"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != ref || items[0].(map[string]any)["resolution"] != "uid" {
		t.Fatalf("uid: %v", m)
	}
	if got := e.queryIDs("<ABC@example.com>"); len(got) != 0 {
		t.Fatalf("uid folded: %v", got)
	}

	// Redacted label does not match; the text still can.
	e.ok("redact", labelled, "--fields", "label", "--reason", "r", "--now", "2026-11-07T04:00:00Z")
	if got := e.queryIDs("invoice"); !reflect.DeepEqual(got, []string{paid}) {
		t.Fatalf("after redacting label: %v", got)
	}
	if got := e.queryIDs("match"); !reflect.DeepEqual(got, []string{labelled}) {
		t.Fatalf("text still matches: %v", got)
	}
	// Redacted uid does not match.
	e.ok("redact", ref, "--fields", "content.uid", "--reason", "r", "--now", "2026-11-07T04:01:00Z")
	if got := e.queryIDs("<abc@example.com>"); len(got) != 0 {
		t.Fatalf("after redacting uid: %v", got)
	}
}

// recall-by-query, spec "Query results and order".
func TestQueryOrder(t *testing.T) {
	e := newEnv(t)
	a := e.inlineNote("2026-11-07T03:00:00Z", "invoice for Ada")
	b := e.inlineNote("2026-11-07T03:01:00Z", "an invoice")
	c := e.inlineNote("2026-11-07T03:02:00Z", "another invoice")
	if got := e.queryIDs("invoice ada"); !reflect.DeepEqual(got, []string{a, c, b}) {
		t.Fatalf("order: %v", got)
	}
	// --limit is applied after ordering.
	if got := e.queryIDs("invoice ada", "--limit", "2"); !reflect.DeepEqual(got, []string{a, c}) {
		t.Fatalf("limit: %v", got)
	}
	m := e.ok("query", "invoice ada")["episodes"].([]any)[0].(map[string]any)
	if m["matched"] != 2.0 || m["resolution"] != "lexical" {
		t.Fatal(m)
	}

	// conversation start labels the turn, so the container comes by import.
	lines := `{"@key": "c", "kind": "conversation", "standpoint": "` + ada + `", "label": "planning", "content": {"mode": "none"}}
{"kind": "message", "standpoint": "` + ada + `", "container": "@c", "ordinal": "auto", "content": {"mode": "inline", "text": "the invoice again"}}
`
	imp := e.runIn(lines, "import", "-", "--now", "2026-11-07T03:10:00Z")
	if imp.code != 0 {
		t.Fatalf("import: %+v", imp)
	}
	res := imp.json(t)["lines"].([]any)
	conv, turn := res[0].(map[string]any)["id"].(string), res[1].(map[string]any)["id"].(string)
	got := e.queryIDs("invoice", "--limit", "1")
	if !reflect.DeepEqual(got, []string{turn}) {
		t.Fatalf("turn: %v", got)
	}
	for _, id := range e.queryIDs("invoice") {
		if id == conv {
			t.Fatal("container returned through its child")
		}
	}
	first := e.ok("query", "invoice")["episodes"].([]any)[0].(map[string]any)
	if first["container"] != conv {
		t.Fatalf("turn's container: %v", first)
	}
	if got := e.queryIDs("planning"); !reflect.DeepEqual(got, []string{conv}) {
		t.Fatalf("container matching itself: %v", got)
	}
	if got := e.queryIDs("invoice", "--container", conv); !reflect.DeepEqual(got, []string{turn}) {
		t.Fatalf("--container: %v", got)
	}
}

// recall-by-query, spec "Query filters and withdrawn episodes".
func TestQueryFilters(t *testing.T) {
	e := newEnv(t)
	secret := e.inlineNote("2026-11-07T03:00:00Z", "the invoice", "--label", "invoice note")
	e.ok("redact", secret, "--fields", "content.text", "--reason", "r", "--now", "2026-11-07T04:00:00Z")
	if got := e.queryIDs("invoice"); len(got) != 0 {
		t.Fatalf("withdrawn returned: %v", got)
	}
	if r := e.run("query", "invoice", "--include-withdrawn"); r.code != 2 {
		t.Fatalf("--include-withdrawn: %+v", r)
	}

	with := e.inlineNote("2026-11-07T03:01:00Z", "invoice", "--participant", "https://example.com/ada")
	e.inlineNote("2026-11-07T03:02:00Z", "invoice", "--participant", "https://example.com/Ada")
	e.inlineNote("2026-11-07T03:03:00Z", "invoice")
	if got := e.queryIDs("invoice", "--participant", "https://example.com/ada"); !reflect.DeepEqual(got, []string{with}) {
		t.Fatalf("--participant: %v", got)
	}

	other := e.inlineNote("2026-11-08T03:00:00Z", "invoice", "--standpoint", claude, "--kind", "message")
	if got := e.queryIDs("invoice", "--standpoint", claude); !reflect.DeepEqual(got, []string{other}) {
		t.Fatalf("--standpoint: %v", got)
	}
	if got := e.queryIDs("invoice", "--kind", "message"); !reflect.DeepEqual(got, []string{other}) {
		t.Fatalf("--kind: %v", got)
	}
	if got := e.queryIDs("invoice", "--since", "2026-11-08"); !reflect.DeepEqual(got, []string{other}) {
		t.Fatalf("--since: %v", got)
	}
	if r := e.run("query", "invoice", "--since", "yesterday"); r.code != 2 {
		t.Fatalf("bad --since: %+v", r)
	}

	// Index lagging after a merge: the file wins.
	merged := idAt(at("2026-11-07T05:00:00Z"))
	e.put(merged, strings.Replace(handMade(merged, ""), "timestamp: 2026-11-07T03:24:40Z", "timestamp: 2026-11-07T05:00:00Z", 1))
	if got := e.queryIDs("hi"); !reflect.DeepEqual(got, []string{merged}) {
		t.Fatalf("merged: %v", got)
	}
}

func TestQueryText(t *testing.T) {
	e := newEnv(t)
	id := e.inlineNote("2026-11-07T03:00:00Z", "the invoice")
	r := execute(t, "", "query", "invoice", "--workspace", e.dir)
	if r.code != 0 || !strings.HasPrefix(r.out, "lexical ") || !strings.Contains(r.out, id) {
		t.Fatalf("%+v", r)
	}
	r = execute(t, "", "query", "nothing", "--workspace", e.dir)
	if r.code != 0 || r.out != "No episodes found.\n" {
		t.Fatalf("%+v", r)
	}
}
