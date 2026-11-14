package store

import (
	"bytes"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// indexFiles is index.yaml and every segment, by path.
func indexFiles(t *testing.T, w *Workspace) map[string]string {
	t.Helper()
	out := map[string]string{}
	root, err := os.ReadFile(w.Abs(IndexFile))
	if err != nil {
		t.Fatal(err)
	}
	out[IndexFile] = string(root)
	segs, _ := filepath.Glob(filepath.Join(w.Abs(IndexDir), "*.yaml"))
	for _, s := range segs {
		b, _ := os.ReadFile(s)
		out[w.Rel(s)] = string(b)
	}
	return out
}

func createWith(w *Workspace, ep *model.Episode, full bool) error {
	forceFullUpsert = full
	defer func() { forceFullUpsert = false }()
	unlock, err := w.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	l, err := w.Scan()
	if err != nil {
		return err
	}
	_, err = w.Create(l, ep)
	return err
}

// fast-index-upsert D4: the two paths write the same index after every create.
func TestUpsertEquivalence(t *testing.T) {
	fast, full := newWS(t), newWS(t)
	rng := rand.New(rand.NewSource(42))
	at := month("2026-08-25")
	fastBefore := fastUpserts
	var refused, outOfOrder int
	for i := 0; i < 200; i++ {
		var ts time.Time
		switch r := rng.Intn(100); {
		case r < 80:
			at = at.Add(time.Duration(rng.Intn(72*60)) * time.Minute)
			ts = at
		case r < 95:
			// Earlier than the newest id: out of order, sometimes into a
			// month already sealed.
			ts = at.Add(-time.Duration(rng.Intn(50*24*60)) * time.Minute)
			outOfOrder++
		default:
			at = at.AddDate(0, 1+rng.Intn(2), 0)
			ts = at
		}
		ep := episode(idAt(ts))
		ef := createWith(fast, ep, false)
		eb := createWith(full, ep.Clone(), true)
		if (ef == nil) != (eb == nil) || (ef != nil && !errors.Is(ef, ErrSealedMonth)) {
			t.Fatalf("step %d: fast %v, full %v", i, ef, eb)
		}
		if ef != nil {
			refused++
		}
		a, b := indexFiles(t, fast), indexFiles(t, full)
		if len(a) != len(b) {
			t.Fatalf("step %d: index files differ: %v vs %v", i, keys(a), keys(b))
		}
		for k, v := range b {
			if a[k] != v {
				t.Fatalf("step %d: %s differs\n--- fast\n%s\n--- full\n%s", i, k, a[k], v)
			}
		}
	}
	used := fastUpserts - fastBefore
	t.Logf("fast path %d of %d creates; %d out of order, %d refused as sealed", used, 200-refused, outOfOrder, refused)
	if used < 100 || refused == 0 || outOfOrder == 0 {
		t.Fatalf("sequence did not exercise both paths: fast %d, refused %d", used, refused)
	}
	if d, err := fast.CheckIndex(); err != nil || !d.Clean() {
		t.Fatalf("%+v %v", d, err)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// fast-index-upsert D2: each fallback takes the full path, with its result.
func TestUpsertFallbacks(t *testing.T) {
	setup := func(t *testing.T) (*Workspace, *Workspace) {
		a, b := newWS(t), newWS(t)
		for _, d := range []string{"2026-11-01", "2026-11-02"} {
			ep := episode(idAt(month(d)))
			if err := createWith(a, ep, false); err != nil {
				t.Fatal(err)
			}
			if err := createWith(b, ep.Clone(), true); err != nil {
				t.Fatal(err)
			}
		}
		return a, b
	}
	edit := func(w *Workspace, f func(string) string) {
		b, _ := os.ReadFile(w.Abs(IndexFile))
		_ = os.WriteFile(w.Abs(IndexFile), []byte(f(string(b))), 0o644)
	}
	cases := map[string]struct {
		mut  func(string) string
		next string
	}{
		"conflict markers":   {func(s string) string { return "<<<<<<< HEAD\n" + s + "=======\n>>>>>>> b\n" }, "2026-11-03"},
		"empty entries":      {func(string) string { return "format: episodes/0.1\nsegments: []\nentries: []\n" }, "2026-11-03"},
		"no final newline":   {func(s string) string { return strings.TrimSuffix(s, "\n") }, "2026-11-03"},
		"id sorts before":    {func(s string) string { return s }, "2026-10-31"},
		"non-canonical head": {func(s string) string { return strings.Replace(s, "segments: []", "segments: [ ]", 1) }, "2026-11-03"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a, b := setup(t)
			edit(a, c.mut)
			edit(b, c.mut)
			before := fastUpserts
			ep := episode(idAt(month(c.next)))
			if err := createWith(a, ep, false); err != nil {
				t.Fatal(err)
			}
			if err := createWith(b, ep.Clone(), true); err != nil {
				t.Fatal(err)
			}
			if fastUpserts != before {
				t.Fatal("took the fast path")
			}
			if ia, ib := indexFiles(t, a), indexFiles(t, b); ia[IndexFile] != ib[IndexFile] {
				t.Fatalf("results differ:\n%s\n---\n%s", ia[IndexFile], ib[IndexFile])
			}
		})
	}
	// A first entry in a sealed month: the root lags a seal.
	t.Run("first entry sealed", func(t *testing.T) {
		w := newWS(t)
		sep := episode(idAt(month("2026-09-03")))
		_ = createWith(w, sep, false)
		stale, _ := os.ReadFile(w.Abs(IndexFile))
		_ = createWith(w, episode(idAt(month("2026-11-03"))), false)
		// Put back a root still holding September, with its segment listed.
		seg := strings.Replace(string(stale), "segments: []", "segments:\n  - path: index/2026-09.yaml\n    count: 1", 1)
		_ = os.WriteFile(w.Abs(IndexFile), []byte(seg), 0o644)
		before := fastUpserts
		if err := createWith(w, episode(idAt(month("2026-11-04"))), false); err != nil {
			t.Fatal(err)
		}
		if fastUpserts != before {
			t.Fatal("took the fast path over a sealed first entry")
		}
		if root, _, _ := w.ReadRoot(); len(root.Entries) != 1 || strings.Contains(root.Entries[0].ID, sep.ID) {
			t.Fatalf("%+v", root.Entries)
		}
	})
}

// index spec: a malformed middle is not detected by the fast path, and the
// root stays a cache that --check reports and index rebuilds.
func TestUpsertMalformedMiddle(t *testing.T) {
	w := newWS(t)
	for _, d := range []string{"2026-11-01", "2026-11-02", "2026-11-03"} {
		_ = createWith(w, episode(idAt(month(d))), false)
	}
	b, _ := os.ReadFile(w.Abs(IndexFile))
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "    kind: ") && i > len(lines)/3 && i < 2*len(lines)/3 {
			lines[i] = "    kind: [unclosed"
			break
		}
	}
	_ = os.WriteFile(w.Abs(IndexFile), []byte(strings.Join(lines, "\n")), 0o644)
	before := fastUpserts
	if err := createWith(w, episode(idAt(month("2026-11-04"))), false); err != nil {
		t.Fatal(err)
	}
	if fastUpserts != before+1 {
		t.Fatal("expected the fast path")
	}
	if d, _ := w.CheckIndex(); d.RootUnreadable == "" {
		t.Fatalf("--check should report the root unreadable: %+v", d)
	}
	if _, err := w.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if d, _ := w.CheckIndex(); !d.Clean() {
		t.Fatalf("%+v", d)
	}
}

func TestEntryBytesMatchFullEncode(t *testing.T) {
	o := 3
	es := []Entry{
		{ID: "epi_a", Kind: "message", Occurred: model.Occurred{Start: "2026-11-07T14:24:00+11:00[Australia/Melbourne]", Basis: "recorded"}, Standpoint: "https://example.com/ada", Timestamp: "2026-11-07T03:24:40Z"},
		{ID: "epi_b", Kind: "note", Occurred: model.Occurred{Start: "2026-11-03", TZ: "Australia/Melbourne", Basis: "inferred"}, Container: "epi_a", Ordinal: &o, Standpoint: "Jane: at accounts # x", Timestamp: "2026-11-07T03:24:41Z"},
	}
	full, _ := EncodeRoot(&Root{Format: model.Format, Entries: es})
	head, _ := EncodeRoot(&Root{Format: model.Format, Entries: es[:1]})
	tail, _ := entryBytes(es[1])
	if !bytes.Equal(append(head, tail...), full) {
		t.Fatalf("splice differs:\n%s\n---\n%s", append(head, tail...), full)
	}
}
