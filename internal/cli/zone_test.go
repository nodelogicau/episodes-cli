package cli

import (
	"reflect"
	"strings"
	"testing"
)

// filter-dates-in-zone: a Melbourne workspace with the #27 email, at 09:12
// Melbourne time on the 16th (22:12 UTC on the 15th), and a note at 08:00 on
// the 17th (21:00 UTC on the 16th).
func melbourne(t *testing.T) (e *env, email, late string) {
	t.Helper()
	e = newEnv(t, "--zone", "Australia/Melbourne")
	email = e.ok("record", "ref", "--kind", "email", "--standpoint", ada, "--ref", "mail:inbox/4471",
		"--occurred", "2026-11-16T09:12:00+11:00", "--basis", "stated", "--label", "the invoice",
		"--now", "2026-11-17T00:00:00Z")["id"].(string)
	late = e.note("2026-11-17T00:00:00Z", "--occurred", "2026-11-17T08:00:00+11:00", "--basis", "stated")
	return e, email, late
}

func (e *env) listIDs(args ...string) ([]string, map[string]any) {
	e.t.Helper()
	m := e.ok(append([]string{"list"}, args...)...)
	ids := []string{}
	for _, it := range m["episodes"].([]any) {
		ids = append(ids, it.(map[string]any)["id"].(string))
	}
	return ids, m
}

// recall spec, "Date bounds in the reader's zone".
func TestDateBoundsInZone(t *testing.T) {
	t.Setenv(EnvZone, "")
	e, email, late := melbourne(t)

	// Since a local day.
	ids, m := e.listIDs("--since", "2026-11-16")
	if !reflect.DeepEqual(ids, []string{email, late}) || m["zone"] != "Australia/Melbourne" {
		t.Fatalf("since: %v %v", ids, m["zone"])
	}
	// Until a local day: 08:00 on the 17th is not the 16th in Melbourne.
	if ids, _ = e.listIDs("--until", "2026-11-16"); !reflect.DeepEqual(ids, []string{email}) {
		t.Fatalf("until: %v", ids)
	}
	// --tz overrides the workspace.
	ids, m = e.listIDs("--since", "2026-11-16", "--tz", "UTC")
	if !reflect.DeepEqual(ids, []string{late}) || m["zone"] != "UTC" {
		t.Fatalf("--tz: %v %v", ids, m["zone"])
	}
	// A date-time bound ignores the zone and reports none.
	ids, m = e.listIDs("--since", "2026-11-16T00:00:00Z")
	if _, has := m["zone"]; !reflect.DeepEqual(ids, []string{late}) || has {
		t.Fatalf("date-time: %v %v", ids, m)
	}
	// No date bound: no zone.
	if _, m = e.listIDs(); m["zone"] != nil {
		t.Fatalf("unfiltered: %v", m)
	}
	// --ndjson has no envelope and carries no zone.
	r := execute(t, "", "list", "--since", "2026-11-16", "--ndjson", "--workspace", e.dir)
	if r.code != 0 || strings.Count(r.out, "\n") != 2 || strings.Contains(r.out, `"zone":"Australia`) {
		t.Fatalf("ndjson: %+v", r)
	}

	// --tz without a date is a usage error.
	for _, args := range [][]string{
		{"list", "--tz", "Australia/Melbourne"},
		{"list", "--since", "2026-11-16T00:00:00Z", "--tz", "Australia/Melbourne"},
	} {
		if r := e.run(args...); r.code != 2 || !strings.Contains(r.err, "--tz") {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	// An unknown zone names its source.
	if r := e.run("list", "--since", "2026-11-16", "--tz", "Mars/Olympus"); r.code != 2 || !strings.Contains(r.err, "--tz") {
		t.Fatalf("bad --tz: %+v", r)
	}
	t.Setenv(EnvZone, "Mars/Olympus")
	if r := e.run("list", "--since", "2026-11-16"); r.code != 2 || !strings.Contains(r.err, EnvZone) {
		t.Fatalf("bad %s: %+v", EnvZone, r)
	}
	// EPISODES_ZONE comes before defaults.zone.
	t.Setenv(EnvZone, "UTC")
	ids, m = e.listIDs("--since", "2026-11-16")
	if !reflect.DeepEqual(ids, []string{late}) || m["zone"] != "UTC" {
		t.Fatalf("%s: %v %v", EnvZone, ids, m["zone"])
	}
}

func TestDateBoundsNoZone(t *testing.T) {
	t.Setenv(EnvZone, "")
	e := newEnv(t)
	email := e.ok("record", "ref", "--kind", "email", "--standpoint", ada, "--ref", "mail:inbox/4471",
		"--occurred", "2026-11-16T09:12:00+11:00", "--basis", "stated", "--now", "2026-11-17T00:00:00Z")["id"].(string)
	// The bound begins at 2026-11-16T00:00Z, after the email (22:12Z on the 15th).
	ids, m := e.listIDs("--since", "2026-11-16")
	if len(ids) != 0 || m["zone"] != "UTC" {
		t.Fatalf("%v %v", ids, m)
	}
	if ids, _ = e.listIDs("--since", "2026-11-15"); !reflect.DeepEqual(ids, []string{email}) {
		t.Fatalf("%v", ids)
	}
}

// query spec, "Query filters and withdrawn episodes": Since a local day.
func TestQueryDateBoundsInZone(t *testing.T) {
	t.Setenv(EnvZone, "")
	e, email, _ := melbourne(t)
	m := e.ok("query", "invoice", "--since", "2026-11-16")
	if got := e.queryIDs("invoice", "--since", "2026-11-16"); !reflect.DeepEqual(got, []string{email}) || m["zone"] != "Australia/Melbourne" {
		t.Fatalf("%v %v", got, m["zone"])
	}
	if got := e.queryIDs("invoice", "--since", "2026-11-16", "--tz", "UTC"); len(got) != 0 {
		t.Fatalf("--tz UTC: %v", got)
	}
	if r := e.run("query", "invoice", "--tz", "UTC"); r.code != 2 {
		t.Fatalf("--tz without a date: %+v", r)
	}
}
