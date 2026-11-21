package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// episodeFiles is every episode file's bytes, by path.
func episodeFiles(t *testing.T, w *Workspace) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(w.Abs(EpisodesDir), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			out[w.Rel(p)] = string(b)
		}
		return nil
	})
	return out
}

func batchWith(t *testing.T, w *Workspace, eps []*model.Episode) ([]string, error) {
	t.Helper()
	unlock, err := w.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	l, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	return w.CreateBatch(l, eps)
}

func idsFrom(start time.Time, n int, step time.Duration) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = idAt(start.Add(time.Duration(i) * step))
	}
	sort.Strings(out)
	return out
}

func sameWorkspaces(t *testing.T, a, b *Workspace) {
	t.Helper()
	for name, get := range map[string]func(*testing.T, *Workspace) map[string]string{"index": indexFiles, "episodes": episodeFiles} {
		fa, fb := get(t, a), get(t, b)
		if !reflect.DeepEqual(keys(fa), keys(fb)) {
			t.Fatalf("%s files differ: %v vs %v", name, keys(fa), keys(fb))
		}
		for k, v := range fb {
			if fa[k] != v {
				t.Fatalf("%s differs\n--- batch\n%s\n--- creates\n%s", k, fa[k], v)
			}
		}
	}
}

// bulk-import 1.4: CreateBatch gives the same files and index as N Creates,
// on the splice path and when the batch opens a month that seals others.
func TestCreateBatchEqualsCreates(t *testing.T) {
	cases := map[string]struct {
		seed, batch []string
		splice      bool
	}{
		"splice":       {idsFrom(month("2026-11-01"), 5, time.Hour), idsFrom(month("2026-11-02"), 40, time.Minute), true},
		"opens months": {idsFrom(month("2026-08-01"), 5, 24*time.Hour), idsFrom(month("2026-08-20"), 60, 24*time.Hour), false},
		"empty root":   {nil, idsFrom(month("2026-11-02"), 3, time.Minute), false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a, b := newWS(t), newWS(t)
			for _, id := range c.seed {
				create(t, a, episode(id))
				create(t, b, episode(id))
			}
			var eps []*model.Episode
			for _, id := range c.batch {
				eps = append(eps, episode(id))
			}
			before := fastUpserts
			paths, err := batchWith(t, a, eps)
			if err != nil {
				t.Fatal(err)
			}
			if spliced := fastUpserts == before+1; spliced != c.splice {
				t.Fatalf("spliced %v, want %v", spliced, c.splice)
			}
			if len(paths) != len(eps) || paths[0] != RelPath(eps[0].ID) {
				t.Fatalf("paths %v", paths)
			}
			for _, ep := range eps {
				create(t, b, ep.Clone())
			}
			sameWorkspaces(t, a, b)
			if d, err := a.CheckIndex(); err != nil || !d.Clean() {
				t.Fatalf("%+v %v", d, err)
			}
		})
	}
}

// bulk-import 1.4: a batch whose newest id opens a new month seals the old
// one, writing its segment once.
func TestCreateBatchSeals(t *testing.T) {
	w := newWS(t)
	aug := episode(idAt(month("2026-08-10")))
	create(t, w, aug)
	eps := []*model.Episode{episode(idAt(month("2026-09-10"))), episode(idAt(month("2026-10-10")))}
	if _, err := batchWith(t, w, eps); err != nil {
		t.Fatal(err)
	}
	seg, _, err := w.ReadSegment(SegmentPath(MonthOfTime(month("2026-08-01"))))
	if err != nil || len(seg.Entries) != 1 || seg.Entries[0].ID != aug.ID {
		t.Fatalf("august not sealed: %+v %v", seg, err)
	}
	root, _, _ := w.ReadRoot()
	if len(root.Entries) != 2 || len(root.Segments) != 1 {
		t.Fatalf("root %+v", root)
	}
}

// bulk-import 1.4: a failure part way returns the paths written, which the
// index still holds; a sealed month refuses the whole batch.
func TestCreateBatchPartialFailure(t *testing.T) {
	w := newWS(t)
	ids := idsFrom(month("2026-11-02"), 5, time.Minute)
	var eps []*model.Episode
	for _, id := range ids {
		eps = append(eps, episode(id))
	}
	put(t, w, episode(ids[3])) // occupies the fourth path
	paths, err := batchWith(t, w, eps)
	if !errors.Is(err, ErrAlreadyExists) || !strings.Contains(err.Error(), RelPath(ids[3])) {
		t.Fatalf("err %v", err)
	}
	if want := []string{RelPath(ids[0]), RelPath(ids[1]), RelPath(ids[2])}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths %v, want %v", paths, want)
	}
	root, _, _ := w.ReadRoot()
	got := map[string]bool{}
	for _, e := range root.Entries {
		got[e.ID] = true
	}
	for _, id := range ids[:3] {
		if !got[id] {
			t.Fatalf("written %s not indexed: %+v", id, root.Entries)
		}
	}
	if got[ids[4]] {
		t.Fatal("unwritten episode indexed")
	}

	sealed := newWS(t)
	create(t, sealed, episode(idAt(month("2026-11-02"))))
	before := episodeFiles(t, sealed)
	_, err = batchWith(t, sealed, []*model.Episode{episode(idAt(month("2026-11-03"))), episode(idAt(month("2026-08-03")))})
	if !errors.Is(err, ErrSealedMonth) || !reflect.DeepEqual(episodeFiles(t, sealed), before) {
		t.Fatalf("sealed: %v", err)
	}
}

func collect(t *testing.T, w *Workspace, l *Listing, minMonth int, hasMin bool) []Entry {
	t.Helper()
	var out []Entry
	if err := w.EachEntry(l, minMonth, hasMin, func(e Entry, f FileRef) error {
		if f.ID != e.ID {
			t.Fatalf("file %s for entry %s", f.ID, e.ID)
		}
		out = append(out, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// bulk-import D6: the stream is Entries, in id order, however stale the
// index: sealed segments, a file the root lacks, a legacy id, a missing
// file, and a root that does not parse.
func TestEachEntryEqualsEntries(t *testing.T) {
	w := newWS(t)
	for _, id := range idsFrom(month("2026-07-01"), 30, 4*24*time.Hour) {
		create(t, w, episode(id))
	}
	late := episode(idAt(month("2026-11-02").Add(time.Hour)))
	put(t, w, late) // merged, not indexed
	legacy := episode("epi_import-42")
	put(t, w, legacy)
	_, _ = w.Rebuild()
	put(t, w, episode(idAt(month("2026-11-03")))) // after the rebuild
	l, _ := w.Scan()
	_ = os.Remove(w.Abs(l.Files[3].Rel)) // listed, then gone

	check := func(t *testing.T) {
		t.Helper()
		l, _ := w.Scan()
		for _, mm := range []struct {
			m  int
			ok bool
		}{{0, false}, {MonthOfTime(month("2026-09-01")), true}} {
			want := w.Entries(l, mm.m, mm.ok)
			sort.SliceStable(want, func(i, j int) bool { return want[i].ID < want[j].ID })
			got := collect(t, w, l, mm.m, mm.ok)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("min %v: stream %d entries, Entries %d", mm, len(got), len(want))
			}
		}
	}
	t.Run("stale", check)
	t.Run("conflicted root", func(t *testing.T) {
		b, _ := os.ReadFile(w.Abs(IndexFile))
		_ = os.WriteFile(w.Abs(IndexFile), append([]byte("<<<<<<< HEAD\n"), b...), 0o644)
		check(t)
	})
	t.Run("stops", func(t *testing.T) {
		l, _ := w.Scan()
		stop := errors.New("stop")
		n := 0
		if err := w.EachEntry(l, 0, false, func(Entry, FileRef) error { n++; return stop }); !errors.Is(err, stop) || n != 1 {
			t.Fatalf("%v after %d", err, n)
		}
	})
}

// bulk-import D2: a UIDSet answers as UIDCandidates does, and a fresh one
// catches up with what was written since.
func TestUIDSet(t *testing.T) {
	t.Setenv(EnvCacheDir, t.TempDir())
	w := newWS(t)
	const sp, mt = "https://example.com/ada", "message/rfc822"
	ids := idsFrom(month("2026-11-02"), 4, time.Minute)
	for i, id := range ids[:3] {
		put(t, w, refEpisode(id, sp, mt, []string{"mid:a@x", "mid:b@x", "mid:a@x"}[i]))
	}
	l, _ := w.Scan()
	got := func(s *UIDSet, uid string) []string {
		var out []string
		for _, f := range s.Candidates(sp, "Message/RFC822", uid) {
			out = append(out, f.ID)
		}
		sort.Strings(out)
		return out
	}
	s := w.UIDSet(l)
	if a := got(s, "mid:a@x"); !reflect.DeepEqual(a, fullParse(w, l, sp, mt, "mid:a@x")) || len(a) != 2 {
		t.Fatalf("%v", a)
	}
	ep := refEpisode(ids[3], sp, mt, "mid:c@x")
	if _, err := w.CreateBatch(l, []*model.Episode{ep}); err != nil {
		t.Fatal(err)
	}
	w.NoteUIDs([]*model.Episode{ep})
	l, _ = w.Scan()
	if c := got(w.UIDSet(l), "mid:c@x"); !reflect.DeepEqual(c, []string{ids[3]}) {
		t.Fatalf("%v", c)
	}
}
