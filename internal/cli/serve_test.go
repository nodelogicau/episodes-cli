package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nodelogicau/episodes-cli/internal/mcp"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

// add-mcp-server 5.1: every flag a tool maps to exists on its verb.
func TestToolFlagsExist(t *testing.T) {
	a := &app{getenv: os.Getenv}
	root := a.rootCmd()
	for _, d := range mcp.Tools() {
		cmd, _, err := root.Find(d.Verb)
		if err != nil || cmd.Name() != d.Verb[len(d.Verb)-1] {
			t.Fatalf("%s: verb %v not found", d.Name, d.Verb)
		}
		for _, f := range mcp.Flags(d.Input) {
			if cmd.Flags().Lookup(f) == nil {
				t.Errorf("%s: --%s is not a flag of %v", d.Name, f, d.Verb)
			}
		}
	}
}

// client connects an in-memory MCP client, named name, to a server for e.
type client struct {
	t  *testing.T
	cs *sdk.ClientSession
	mu sync.Mutex
	// updated collects resource-updated notifications.
	updated []string
}

func (e *env) serve(name string, conversation bool, flags ...string) *client {
	e.t.Helper()
	ws, err := store.Discover(e.dir)
	if err != nil {
		e.t.Fatal(err)
	}
	author, harness := "", ""
	for i := 0; i+1 < len(flags); i += 2 {
		switch flags[i] {
		case "--author":
			author = flags[i+1]
		case "--harness":
			harness = flags[i+1]
		}
	}
	a := &app{getenv: os.Getenv}
	srv := a.newMCPServer(ws, author, harness, "", conversation)
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.SDK().Connect(ctx, st, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	c := &client{t: e.t}
	cl := sdk.NewClient(&sdk.Implementation{Name: name, Version: "1"}, &sdk.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, r *sdk.ResourceUpdatedNotificationRequest) {
			c.mu.Lock()
			c.updated = append(c.updated, r.Params.URI)
			c.mu.Unlock()
		},
	})
	cs, err := cl.Connect(ctx, ct, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	c.cs = cs
	e.t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })
	return c
}

func (c *client) tools() []string {
	c.t.Helper()
	res, err := c.cs.ListTools(context.Background(), nil)
	if err != nil {
		c.t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	sort.Strings(names)
	return names
}

func (c *client) tool(name string) *sdk.Tool {
	res, _ := c.cs.ListTools(context.Background(), nil)
	for _, tl := range res.Tools {
		if tl.Name == name {
			return tl
		}
	}
	return nil
}

// call returns the result's structured content and whether it is an error.
// A protocol error (such as a schema violation) is returned as err.
func (c *client) call(name string, args map[string]any) (map[string]any, bool, error) {
	c.t.Helper()
	res, err := c.cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, false, err
	}
	m, _ := res.StructuredContent.(map[string]any)
	if m == nil {
		// Decode from the text when the client did not unmarshal it.
		for _, ct := range res.Content {
			if tc, ok := ct.(*sdk.TextContent); ok {
				_ = json.Unmarshal([]byte(tc.Text), &m)
			}
		}
	}
	return m, res.IsError, nil
}

func (c *client) ok(name string, args map[string]any) map[string]any {
	c.t.Helper()
	m, isErr, err := c.call(name, args)
	if err != nil || isErr {
		c.t.Fatalf("%s: %v %v", name, err, m)
	}
	return m
}

// refused reports the error code of a refused call: the tool error's code,
// or "protocol" for a call rejected before it reached the verb.
func (c *client) refused(name string, args map[string]any) string {
	c.t.Helper()
	m, isErr, err := c.call(name, args)
	if err != nil {
		return "protocol"
	}
	if !isErr {
		c.t.Fatalf("%s: not refused: %v", name, m)
	}
	e, _ := m["error"].(map[string]any)
	code, _ := e["code"].(string)
	if code == "" {
		// Rejected by the schema before reaching the verb.
		return "protocol"
	}
	return code
}

// 5.2: the tool list, annotations, results and refusals.
func TestServeTools(t *testing.T) {
	e := newEnv(t)
	c := e.serve("test-client", false)
	want := []string{"act_attest", "episode_list", "episode_observe", "episode_query", "episode_record_inline", "episode_record_ref",
		"episode_redact", "episode_show", "episode_thread", "validate", "workspace_status"}
	if got := c.tools(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("tools: %v", got)
	}
	if !c.tool("episode_show").Annotations.ReadOnlyHint || !*c.tool("episode_redact").Annotations.DestructiveHint ||
		*c.tool("episode_record_inline").Annotations.DestructiveHint {
		t.Fatal("annotations")
	}
	if got := e.serve("test-client", true).tools(); len(got) != 13 {
		t.Fatalf("with conversation tools: %v", got)
	}
	if code := c.refused("conversation_turn", map[string]any{"container": "x", "standpoint": ada, "text": "t"}); code != "protocol" {
		t.Fatalf("unlisted conversation tool: %s", code)
	}

	// A note; the result is the verb's --json shape.
	m := c.ok("episode_record_inline", map[string]any{"kind": "note", "standpoint": ada, "text": "the invoice"})
	ep := m["episode"].(map[string]any)
	if ep["occurred"].(map[string]any)["basis"] != "recorded" || m["id"] == nil || m["path"] == nil || ep["source"].(map[string]any)["author"] != ada {
		t.Fatalf("record: %v", m)
	}
	id := m["id"].(string)
	if got := c.ok("episode_show", map[string]any{"id": id}); got["episode"].(map[string]any)["id"] != id {
		t.Fatalf("show: %v", got)
	}

	// Text that looks like a flag stays text; links as objects.
	m = c.ok("episode_record_inline", map[string]any{"kind": "note", "standpoint": ada, "text": "--kind=other",
		"links": []any{map[string]any{"rel": "related", "href": id}}})
	ep = m["episode"].(map[string]any)
	if ep["kind"] != "note" || ep["content"].(map[string]any)["text"] != "--kind=other" || ep["links"] == nil {
		t.Fatalf("flag-like text: %v", m)
	}
	// A positional that looks like a flag is an id, not --help.
	if code := c.refused("episode_show", map[string]any{"id": "--help"}); code != "not_found" {
		t.Fatalf("flag-like id: %s", code)
	}

	// Duplicate refusal is the verb's.
	ref := map[string]any{"kind": "email", "standpoint": ada, "ref": "gmail:1", "uid": "<a@x>", "media_type": "message/rfc822",
		"occurred": "2026-11-07T14:24:00+11:00", "basis": "stated"}
	first := c.ok("episode_record_ref", ref)
	if code := c.refused("episode_record_ref", ref); code != "duplicate" {
		t.Fatalf("duplicate: %s", code)
	}
	if got := c.ok("episode_query", map[string]any{"text": "<a@x>"}); got["count"] != 1.0 {
		t.Fatalf("query: %v", got)
	}
	_ = first

	// list and workspace_status read.
	if got := c.ok("episode_list", map[string]any{"kind": "note"}); got["count"] != 2.0 {
		t.Fatalf("list: %v", got)
	}
	if got := c.ok("workspace_status", nil); got["root"] == nil {
		t.Fatalf("workspace_status: %v", got)
	}
	// An invalid workspace still reports.
	bad := filepath.Join(e.dir, "episodes", "2026-11", "epi_broken.yaml")
	if err := os.WriteFile(bad, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := c.ok("validate", nil); got["valid"] != false {
		t.Fatalf("validate: %v", got)
	}
}

// 5.3: author, harness and clock are bound.
func TestServeBinding(t *testing.T) {
	t.Setenv(EnvNow, "2020-01-01T00:00:00Z")
	t.Setenv("EPISODES_HARNESS", "")
	e := newEnv(t)
	c := e.serve("claude-code", false)
	for _, k := range []string{"author", "harness", "now"} {
		if code := c.refused("episode_record_inline", map[string]any{"kind": "note", "standpoint": ada, "text": "x", k: "eve"}); code != "protocol" {
			t.Fatalf("%s accepted: %s", k, code)
		}
	}
	before := time.Now().UTC().Truncate(time.Second)
	m := c.ok("episode_record_inline", map[string]any{"kind": "note", "standpoint": ada, "text": "x", "model": "m-1"})
	ep := m["episode"].(map[string]any)
	src := ep["source"].(map[string]any)
	if src["harness"] != "claude-code" || src["author"] != ada || src["model"] != "m-1" {
		t.Fatalf("source: %v", src)
	}
	ts, _ := time.Parse(time.RFC3339, ep["timestamp"].(string))
	if ts.Before(before) {
		t.Fatalf("timestamp %v from EPISODES_NOW", ts)
	}
	// --harness wins over the client's name.
	m = e.serve("claude-code", false, "--harness", "hook").ok("episode_record_inline", map[string]any{"kind": "note", "standpoint": ada, "text": "y"})
	if m["episode"].(map[string]any)["source"].(map[string]any)["harness"] != "hook" {
		t.Fatalf("--harness: %v", m)
	}
}

// 5.4: act_attest.
func TestServeAct(t *testing.T) {
	e := newEnv(t)
	c := e.serve("claude-code", true)
	if code := c.refused("act_attest", map[string]any{"occurred": "2026-11-07", "basis": "stated"}); code != "protocol" {
		t.Fatalf("no via: %s", code)
	}
	if code := c.refused("act_attest", map[string]any{"via": "x", "occurred": "2026-11-07", "basis": "stated", "control": true}); code != "protocol" {
		t.Fatalf("control: %s", code)
	}
	conv := c.ok("conversation_start", map[string]any{"standpoint": ada, "text": "please log that I called the bank", "occurred": "2026-11-07T03:00:00Z", "basis": "stated"})
	turn := conv["turn"].(map[string]any)["id"].(string)
	container := conv["container"].(map[string]any)["id"].(string)
	theirs := c.ok("conversation_turn", map[string]any{"container": container, "standpoint": claude, "text": "noted", "occurred": "2026-11-07T03:00:10Z", "basis": "stated"})["id"].(string)
	if code := c.refused("act_attest", map[string]any{"via": theirs, "occurred": "2026-11-07", "basis": "stated", "kind": "call"}); code != "refused" {
		t.Fatalf("via from another standpoint: %s", code)
	}
	m := c.ok("act_attest", map[string]any{"via": turn, "occurred": "2026-11-07", "basis": "stated", "kind": "call"})
	if ep := m["episode"].(map[string]any); ep["standpoint"] != ada || ep["content"].(map[string]any)["mode"] != "attested" {
		t.Fatalf("act: %v", m)
	}
}

// 5.5: relative content_file; concurrent turns.
func TestServeFilesAndConcurrency(t *testing.T) {
	e := newEnv(t)
	c := e.serve("claude-code", true)
	if err := os.MkdirAll(filepath.Join(e.dir, "inbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.dir, "inbox", "doc.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref := c.ok("episode_record_ref", map[string]any{"kind": "document", "standpoint": ada, "ref": "drive:doc", "media_type": "text/plain",
		"hash_file": "inbox/doc.txt", "occurred": "2026-11-07", "basis": "stated"})
	if ref["episode"].(map[string]any)["content"].(map[string]any)["hash"] == nil {
		t.Fatalf("hash_file: %v", ref)
	}
	obs := c.ok("episode_observe", map[string]any{"id": ref["id"], "content_file": "inbox/doc.txt"})
	if !strings.Contains(mustJSON(obs), "available") {
		t.Fatalf("observe: %v", obs)
	}

	conv := c.ok("conversation_start", map[string]any{"standpoint": ada, "text": "hi"})
	container := conv["container"].(map[string]any)["id"].(string)
	var wg sync.WaitGroup
	ords := make([]float64, 4)
	for i := range ords {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := c.ok("conversation_turn", map[string]any{"container": container, "standpoint": claude, "text": "t"})
			ords[i] = m["episode"].(map[string]any)["ordinal"].(float64)
		}(i)
	}
	wg.Wait()
	sort.Float64s(ords)
	for i, o := range ords {
		if o != float64(i+2) {
			t.Fatalf("ordinals %v", ords)
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// 5.6: resources and notifications.
func TestServeResources(t *testing.T) {
	e := newEnv(t)
	c := e.serve("claude-code", true)
	ctx := context.Background()
	conv := c.ok("conversation_start", map[string]any{"standpoint": ada, "text": "hi"})
	container := conv["container"].(map[string]any)["id"].(string)
	turn := conv["turn"].(map[string]any)["id"].(string)

	res, err := c.cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "episode://" + turn})
	if err != nil || len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, turn) || res.Contents[0].MIMEType != "application/json" {
		t.Fatalf("read: %v %v", res, err)
	}
	res, err = c.cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "episode://" + container + "/thread"})
	if err != nil || !strings.Contains(res.Contents[0].Text, turn) {
		t.Fatalf("thread: %v %v", res, err)
	}
	if _, err := c.cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "episode://epi_nope"}); err == nil {
		t.Fatal("unknown id read")
	}

	if err := c.cs.Subscribe(ctx, &sdk.SubscribeParams{URI: "episode://" + container + "/thread"}); err != nil {
		t.Fatal(err)
	}
	m := c.ok("episode_redact", map[string]any{"id": turn, "fields": []any{"content.text"}, "reason": "asked"})
	if !strings.Contains(mustJSON(m), "content.text") {
		t.Fatalf("redact: %v", m)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		got := append([]string(nil), c.updated...)
		c.mu.Unlock()
		if len(got) > 0 {
			if got[0] != "episode://"+container+"/thread" {
				t.Fatalf("notified %v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no notification")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// 5.7: serve without --mcp, and outside a workspace.
func TestServeCommand(t *testing.T) {
	r := execute(t, "", "serve")
	if r.code != 2 || r.out != "" {
		t.Fatalf("no --mcp: %+v", r)
	}
	t.Setenv(store.EnvWorkspace, "")
	t.Chdir(t.TempDir())
	r = execute(t, "", "serve", "--mcp", "--json")
	if r.code != 5 || r.out != "" {
		t.Fatalf("no workspace: %+v", r)
	}
}
