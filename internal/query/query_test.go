package query

import (
	"testing"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

func entry(id, start, end, tz string) store.Entry {
	return store.Entry{ID: id, Occurred: model.Occurred{Start: start, End: end, TZ: tz, Basis: "stated"}}
}

func val(t *testing.T, s string) *temporal.Value {
	v, err := temporal.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return &v
}

func TestFilterOverlap(t *testing.T) {
	f := Filter{Since: val(t, "2026-11-02"), Until: val(t, "2026-11-09")}
	cases := map[string]bool{
		"2026-11-02T00:00:00Z": true,
		"2026-11-09T23:59:59Z": true,
		"2026-11-10T00:00:00Z": false,
		"2026-11-01T23:59:59Z": false,
		"2026-11-05":           true,
	}
	for start, want := range cases {
		if got := f.Match(entry("e", start, "", "")); got != want {
			t.Errorf("%s: %v", start, got)
		}
	}
	// An interval that spans the range overlaps it.
	if !f.Match(entry("e", "2026-10-01", "2026-12-01", "")) {
		t.Error("spanning interval")
	}
	// A date-time until is inclusive.
	g := Filter{Until: val(t, "2026-11-07T10:00:00Z")}
	if !g.Match(entry("e", "2026-11-07T10:00:00Z", "", "")) || g.Match(entry("e", "2026-11-07T10:00:01Z", "", "")) {
		t.Error("date-time until")
	}
}

func TestSortByOccurred(t *testing.T) {
	es := []store.Entry{
		entry("c", "2026-11-03T00:30:00+11:00", "", ""),     // 2026-11-02T13:30Z
		entry("b", "2026-11-03", "", "Australia/Melbourne"), // 2026-11-02T13:00Z
		entry("a", "2026-11-03", "", ""),                    // 2026-11-03T00:00Z
		entry("d", "bad", "", ""),
		entry("e", "2026-11-03", "", ""),
	}
	SortByOccurred(es)
	got := ""
	for _, e := range es {
		got += e.ID
	}
	if got != "bcaed" {
		t.Fatal(got)
	}
}

func TestChildrenAndOrdinals(t *testing.T) {
	one, two := 1, 2
	es := []store.Entry{
		{ID: "z", Container: "c", Ordinal: &two},
		{ID: "y", Container: "c"},
		{ID: "x", Container: "c", Ordinal: &one},
		{ID: "w", Container: "other", Ordinal: &two},
	}
	kids := Children(es, "c")
	if len(kids) != 3 || kids[0].ID != "x" || kids[1].ID != "z" || kids[2].ID != "y" {
		t.Fatal(kids)
	}
	if NextOrdinal(kids) != 3 || NextOrdinal(nil) != 1 {
		t.Fatal("next ordinal")
	}
	if h, ok := HolderOf(kids, 2); !ok || h != "z" {
		t.Fatal(h)
	}
}
