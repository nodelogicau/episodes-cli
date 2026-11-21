package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// add-mcp-server 1.3: ExecuteWith's Getenv is what the verbs see, so a
// caller can hide EPISODES_NOW and the clock is used.
func TestExecuteWithHidesNow(t *testing.T) {
	e := newEnv(t)
	t.Setenv(EnvNow, "2020-01-01T00:00:00Z")
	hide := func(k string) string {
		if k == EnvNow {
			return ""
		}
		return os.Getenv(k)
	}
	var out, errb bytes.Buffer
	before := time.Now().UTC().Truncate(time.Second)
	code := ExecuteWith(Options{Getenv: hide}, []string{"record", "inline", "--kind", "note", "--standpoint", ada, "--text", "a note", "--workspace", e.dir, "--json"},
		strings.NewReader(""), &out, &errb, func() bool { return true })
	if code != 0 {
		t.Fatalf("%d %s", code, errb.String())
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339, m["episode"].(map[string]any)["timestamp"].(string))
	if err != nil || ts.Before(before) {
		t.Fatalf("timestamp %v from EPISODES_NOW, not the clock: %v", ts, m)
	}
	// Execute still honours EPISODES_NOW.
	r := newEnv(t).run("record", "inline", "--kind", "note", "--standpoint", ada, "--text", "b")
	if !strings.Contains(r.out, "2020-01-01T00:00:00Z") {
		t.Fatalf("Execute ignored EPISODES_NOW: %+v", r)
	}
}
