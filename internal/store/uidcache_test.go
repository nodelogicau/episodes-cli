package store

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// put writes an episode file directly, as a merge or another tool would,
// bypassing Create and the index.
func put(t *testing.T, w *Workspace, ep *model.Episode) {
	t.Helper()
	data, err := model.Encode(ep)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Path(ep.ID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func refEpisode(id, standpoint, mediaType, uid string) *model.Episode {
	ep := episode(id)
	ep.Kind, ep.Standpoint = "email", standpoint
	ep.Content = model.Content{Mode: model.ModeRef, Ref: "gmail:" + id, UID: uid, MediaType: mediaType}
	return ep
}

// fullParse is the lookup before the cache: parse every file and match.
func fullParse(w *Workspace, l *Listing, standpoint, mediaType, uid string) []string {
	var out []string
	for _, ld := range w.LoadAll(l) {
		ep := ld.Episode
		if ld.Err != nil || ep == nil || ep.Content.Mode != model.ModeRef {
			continue
		}
		if ep.Standpoint == standpoint && ep.Content.UID == uid && lowerASCII(ep.Content.MediaType) == lowerASCII(mediaType) {
			out = append(out, ep.ID)
		}
	}
	sort.Strings(out)
	return out
}

// lookup opens the workspace afresh (so the cache is read from disk, not
// held from an earlier call), lists it and returns the candidates' ids.
func lookup(t *testing.T, root, standpoint, mediaType, uid string) []string {
	t.Helper()
	w, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range w.UIDCandidates(l, standpoint, mediaType, uid) {
		out = append(out, f.ID)
	}
	sort.Strings(out)
	return out
}

func cacheFile(t *testing.T, w *Workspace) string {
	t.Helper()
	p := w.uidCachePath(w.absRoot())
	if p == "" {
		t.Fatal("no cache path")
	}
	return p
}

func TestUIDCacheEqualsFullParse(t *testing.T) {
	t.Setenv(EnvCacheDir, t.TempDir())
	w := newWS(t)
	rng := rand.New(rand.NewSource(7))
	sps := []string{"https://example.com/ada", "https://example.com/jane"}
	mts := []string{"message/rfc822", "Message/RFC822", "text/calendar"}
	uids := []string{"mid:1@x", "mid:2@x", "mid:3@x", "MID:1@x"}
	start := month("2026-08-01")
	add := func(n int) {
		for i := 0; i < n; i++ {
			id := idAt(start.Add(time.Duration(rng.Intn(100*24)) * time.Hour))
			switch rng.Intn(4) {
			case 0:
				put(t, w, episode(id))
			case 1:
				put(t, w, refEpisode(id, sps[rng.Intn(2)], mts[rng.Intn(3)], ""))
			default:
				put(t, w, refEpisode(id, sps[rng.Intn(2)], mts[rng.Intn(3)], uids[rng.Intn(4)]))
			}
		}
	}
	check := func(stage string) {
		t.Helper()
		l, err := w.Scan()
		if err != nil {
			t.Fatal(err)
		}
		for _, sp := range sps {
			for _, mt := range mts {
				for _, u := range uids {
					want := fullParse(w, l, sp, mt, u)
					if got := lookup(t, w.Root, sp, mt, u); strings.Join(got, ",") != strings.Join(want, ",") {
						t.Fatalf("%s %s %s %s: cache %v, full parse %v", stage, sp, mt, u, got, want)
					}
				}
			}
		}
	}
	add(300)
	check("cold")
	if _, err := os.Stat(cacheFile(t, w)); err != nil {
		t.Fatalf("no cache written: %v", err)
	}
	check("warm")
	add(50) // behind the cache's back, across months
	check("after merge")
}

func TestUIDCacheMergedAndRemoved(t *testing.T) {
	t.Setenv(EnvCacheDir, t.TempDir())
	w := newWS(t)
	const sp, mt = "https://example.com/ada", "message/rfc822"
	a := refEpisode(idAt(month("2026-11-02")), sp, mt, "mid:a@x")
	create(t, w, a)
	if got := lookup(t, w.Root, sp, mt, "mid:b@x"); len(got) != 0 {
		t.Fatal(got)
	}
	b := refEpisode(idAt(month("2026-11-03")), sp, mt, "mid:b@x")
	put(t, w, b)
	if got := lookup(t, w.Root, sp, mt, "mid:b@x"); len(got) != 1 || got[0] != b.ID {
		t.Fatalf("merged file not found: %v", got)
	}
	if err := os.Remove(w.Path(a.ID)); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, w.Root, sp, mt, "mid:a@x"); len(got) != 0 {
		t.Fatalf("removed file still a candidate: %v", got)
	}
	data, _ := os.ReadFile(cacheFile(t, w))
	if strings.Contains(string(data), a.ID) {
		t.Fatal("removed file still in the cache")
	}
}

func TestUIDCacheUnparseableRetried(t *testing.T) {
	t.Setenv(EnvCacheDir, t.TempDir())
	w := newWS(t)
	const sp, mt = "https://example.com/ada", "message/rfc822"
	ep := refEpisode(idAt(month("2026-11-02")), sp, mt, "mid:a@x")
	if err := os.MkdirAll(filepath.Dir(w.Path(ep.ID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.Path(ep.ID), []byte("id: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, w.Root, sp, mt, "mid:a@x"); len(got) != 0 {
		t.Fatal(got)
	}
	put(t, w, ep) // repaired in place, same name
	if got := lookup(t, w.Root, sp, mt, "mid:a@x"); len(got) != 1 {
		t.Fatalf("repaired file not retried: %v", got)
	}
}

func TestUIDCacheRebuilt(t *testing.T) {
	const sp, mt = "https://example.com/ada", "message/rfc822"
	for name, content := range map[string]func(w *Workspace) string{
		"corrupt": func(*Workspace) string { return "{not json" },
		"version": func(w *Workspace) string { return `{"cache":"episodes-uid/0","dirs":{}}` },
		"foreign": func(w *Workspace) string {
			return fmt.Sprintf(`{"cache":%q,"workspace":{"id":%q,"root":"/elsewhere"},"dirs":{}}`, uidCacheVersion, w.Config.Workspace.ID)
		},
		"null dirs": func(w *Workspace) string {
			return fmt.Sprintf(`{"cache":%q,"workspace":{"id":%q,"root":%q},"dirs":{"2026-11":null}}`, uidCacheVersion, w.Config.Workspace.ID, w.absRoot())
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvCacheDir, t.TempDir())
			w := newWS(t)
			ep := refEpisode(idAt(month("2026-11-02")), sp, mt, "mid:a@x")
			create(t, w, ep)
			p := cacheFile(t, w)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content(w)), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := lookup(t, w.Root, sp, mt, "mid:a@x"); len(got) != 1 || got[0] != ep.ID {
				t.Fatalf("not rebuilt: %v", got)
			}
			data, _ := os.ReadFile(p)
			if !strings.Contains(string(data), uidCacheVersion) || !strings.Contains(string(data), ep.ID) {
				t.Fatalf("cache not rewritten: %s", data)
			}
		})
	}
}

func TestUIDCacheUnwritable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCacheDir, blocker) // a file: the cache directory cannot be created
	w := newWS(t)
	const sp, mt = "https://example.com/ada", "message/rfc822"
	ep := refEpisode(idAt(month("2026-11-02")), sp, mt, "mid:a@x")
	create(t, w, ep)
	if got := lookup(t, w.Root, sp, mt, "mid:a@x"); len(got) != 1 {
		t.Fatal(got)
	}
}

func TestUIDCacheKeyedByRoot(t *testing.T) {
	t.Setenv(EnvCacheDir, t.TempDir())
	w := newWS(t)
	clone := filepath.Join(t.TempDir(), "clone")
	if err := os.CopyFS(clone, os.DirFS(w.Root)); err != nil {
		t.Fatal(err)
	}
	c, err := Open(clone)
	if err != nil {
		t.Fatal(err)
	}
	if c.Config.Workspace.ID != w.Config.Workspace.ID || cacheFile(t, c) == cacheFile(t, w) {
		t.Fatal("two clones share a cache file")
	}
}

func TestNoteUID(t *testing.T) {
	t.Setenv(EnvCacheDir, t.TempDir())
	w := newWS(t)
	const sp, mt = "https://example.com/ada", "message/rfc822"
	l, _ := w.Scan()
	_ = w.UIDCandidates(l, sp, mt, "mid:a@x")
	ep := refEpisode(idAt(month("2026-11-02")), sp, mt, "mid:a@x")
	if _, err := w.Create(l, ep); err != nil {
		t.Fatal(err)
	}
	w.NoteUID(ep)
	data, _ := os.ReadFile(cacheFile(t, w))
	if !strings.Contains(string(data), `"uid":"mid:a@x"`) {
		t.Fatalf("created episode not noted: %s", data)
	}
}
