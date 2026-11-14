package query

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

func benchID(t time.Time) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	ms := uint64(t.UnixMilli())
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (40 - 8*i))
	}
	b[6] = 0x70 | b[6]&0x0f
	b[8] = 0x80 | b[8]&0x3f
	h := hex.EncodeToString(b[:])
	return "epi_" + h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// seedRefs writes n reference episodes directly, as bench fixtures.
func seedRefs(b *testing.B, n int) *store.Workspace {
	w, _, err := store.Init(b.TempDir(), store.NewConfig())
	if err != nil {
		b.Fatal(err)
	}
	start := time.Now().UTC().Add(-20 * 24 * time.Hour)
	for i := 0; i < n; i++ {
		id := benchID(start.Add(time.Duration(i) * time.Minute))
		ep := &model.Episode{ID: id, Type: "episode", Kind: "email",
			Occurred:   model.Occurred{Start: "2026-11-07T14:24:00+11:00", Basis: "stated"},
			Standpoint: "https://example.com/ada",
			Content:    model.Content{Mode: model.ModeRef, Ref: fmt.Sprintf("gmail:%d", i), UID: fmt.Sprintf("mid:%d@example.com", i), MediaType: "message/rfc822"},
			Source:     model.Source{Author: "https://example.com/ada"}, Timestamp: "2026-11-08T00:00:00Z"}
		data, _ := model.Encode(ep)
		p := w.Path(id)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, data, 0o644)
	}
	return w
}

// lookupOnce is record ref's lookup as the CLI runs it: list, take the
// cache's candidates, verify each against its file.
func lookupOnce(b *testing.B, root string) {
	w, err := store.Open(root)
	if err != nil {
		b.Fatal(err)
	}
	l, err := w.Scan()
	if err != nil {
		b.Fatal(err)
	}
	var cands []store.Loaded
	for _, f := range w.UIDCandidates(l, "https://example.com/ada", "message/rfc822", "mid:19999@example.com") {
		cands = append(cands, w.LoadFile(f))
	}
	if ids := Duplicates(cands, "https://example.com/ada", "message/rfc822", "mid:19999@example.com"); len(ids) != 1 {
		b.Fatal(ids)
	}
}

// BenchmarkDuplicateLookup20k is task 10.2 of bootstrap-episodes-cli, and
// #3: record ref's duplicate lookup over 20,000 reference episodes with the
// uid cache already built (each iteration a fresh process's view: open,
// list, read the cache, verify).
func BenchmarkDuplicateLookup20k(b *testing.B) {
	b.Setenv(store.EnvCacheDir, b.TempDir())
	w := seedRefs(b, 20000)
	lookupOnce(b, w.Root)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lookupOnce(b, w.Root)
	}
}

// BenchmarkDuplicateLookup20kCold is the same lookup with no cache: every
// file parsed, as before #3, plus writing the cache.
func BenchmarkDuplicateLookup20kCold(b *testing.B) {
	b.Setenv(store.EnvCacheDir, b.TempDir())
	w := seedRefs(b, 20000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		b.Setenv(store.EnvCacheDir, b.TempDir())
		b.StartTimer()
		lookupOnce(b, w.Root)
	}
}

// BenchmarkDuplicateLookup20kFullParse is the lookup before #3, for
// comparison: list, parse every file, match.
func BenchmarkDuplicateLookup20kFullParse(b *testing.B) {
	w := seedRefs(b, 20000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, err := w.Scan()
		if err != nil {
			b.Fatal(err)
		}
		if ids := Duplicates(w.LoadAll(l), "https://example.com/ada", "message/rfc822", "mid:19999@example.com"); len(ids) != 1 {
			b.Fatal(ids)
		}
	}
}
