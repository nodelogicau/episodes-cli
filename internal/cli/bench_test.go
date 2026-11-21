package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

// seedRefs writes n reference episodes with uids directly, minted one
// interval apart and ending a day ago, and rebuilds the index once.
func seedRefs(b *testing.B, dir string, n int, span time.Duration) {
	b.Helper()
	w, _, err := store.Init(dir, store.NewConfig())
	if err != nil {
		b.Fatal(err)
	}
	end := time.Now().UTC().Add(-24 * time.Hour)
	step := span / time.Duration(n)
	for i := 0; i < n; i++ {
		id := idAt(end.Add(-time.Duration(n-i) * step))
		ep := &model.Episode{ID: id, Type: model.TypeEpisode, Kind: "email",
			Occurred:   model.Occurred{Start: "2026-11-07T14:24:00+11:00", Basis: model.BasisStated},
			Standpoint: jane,
			Content:    model.Content{Mode: model.ModeRef, Ref: fmt.Sprintf("gmail:seed%d", i), UID: fmt.Sprintf("mid:seed%d@example.com", i), MediaType: "message/rfc822"},
			Source:     model.Source{Author: ada}, Timestamp: "2026-11-08T00:00:00Z"}
		data, _ := model.Encode(ep)
		p := w.Path(id)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := w.Rebuild(); err != nil {
		b.Fatal(err)
	}
}

// fresh copies a seeded workspace and warms its uid cache, as a connector
// re-running on one machine finds it.
func fresh(b *testing.B, template string) string {
	b.Helper()
	dir := filepath.Join(b.TempDir(), "ws")
	if out, err := exec.Command("cp", "-R", template, dir).CombinedOutput(); err != nil {
		b.Fatalf("%v: %s", err, out)
	}
	w, err := store.Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	l, _ := w.Scan()
	_ = w.UIDSet(l)
	return dir
}

func refBatch(n int, tag string) string {
	var buf strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&buf, `{"kind":"email","standpoint":%q,"occurred":{"start":"2026-11-07T14:24:00+11:00","basis":"stated"},"label":"Subject %d","content":{"mode":"ref","ref":"gmail:%s%d","uid":"mid:%s%d@example.com","media_type":"message/rfc822"}}`+"\n", jane, i, tag, i, tag, i)
	}
	return buf.String()
}

func benchEnv(b *testing.B) {
	b.Helper()
	b.Setenv(store.EnvCacheDir, b.TempDir())
	b.Setenv("EPISODES_AUTHOR", ada)
}

// BenchmarkImport10kInto20k is bulk-import's target: one `import` of 10,000
// reference records with uids into a workspace whose open months hold
// 20,000 entries, under 10 s.
func BenchmarkImport10kInto20k(b *testing.B) {
	benchEnv(b)
	template := filepath.Join(b.TempDir(), "template")
	seedRefs(b, template, 20000, 14*24*time.Hour)
	in := refBatch(10000, "new")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dir := fresh(b, template)
		b.StartTimer()
		var out, errb bytes.Buffer
		if code := Execute([]string{"import", "--workspace", dir, "--json"}, strings.NewReader(in), &out, &errb, func() bool { return false }); code != 0 {
			b.Fatalf("exit %d: %s", code, errb.String())
		}
	}
}

// BenchmarkImportChunk is the lock hold time of one chunk: 1,000 records
// re-checked, written and indexed under the lock, in a workspace whose open
// months hold 20,000 entries. It is the longest a capture hook waits.
func BenchmarkImportChunk(b *testing.B) {
	benchEnv(b)
	defer func(f func(int)) { importBeforeChunk = f }(importBeforeChunk)
	template := filepath.Join(b.TempDir(), "template")
	seedRefs(b, template, 20000, 14*24*time.Hour)
	in := refBatch(importChunkSize, "chunk")
	importBeforeChunk = func(int) { b.StartTimer() }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dir := fresh(b, template)
		var out, errb bytes.Buffer
		if code := Execute([]string{"import", "--workspace", dir, "--json"}, strings.NewReader(in), &out, &errb, func() bool { return false }); code != 0 {
			b.Fatalf("exit %d: %s", code, errb.String())
		}
	}
	b.StopTimer()
}

// firstWrite records when its first byte arrives.
type firstWrite struct {
	once  sync.Once
	at    time.Time
	bytes int
}

func (f *firstWrite) Write(p []byte) (int, error) {
	f.once.Do(func() { f.at = time.Now() })
	f.bytes += len(p)
	return len(p), nil
}

var _ io.Writer = (*firstWrite)(nil)

func benchListNDJSON(b *testing.B, span time.Duration) {
	benchEnv(b)
	dir := filepath.Join(b.TempDir(), "ws")
	seedRefs(b, dir, 100000, span)
	var first time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &firstWrite{}
		var errb bytes.Buffer
		start := time.Now()
		if code := Execute([]string{"list", "--ndjson", "--fields", "index", "--workspace", dir}, strings.NewReader(""), out, &errb, func() bool { return false }); code != 0 {
			b.Fatalf("exit %d: %s", code, errb.String())
		}
		first += out.at.Sub(start)
	}
	b.ReportMetric(float64(first.Milliseconds())/float64(b.N), "ms-to-first-line")
}

// BenchmarkListNDJSONIndex100k is bulk-import's recall target: the first
// line of `list --ndjson --fields index` over 100,000 indexed episodes in
// under 1 s. They span two years, so most months are sealed segments.
func BenchmarkListNDJSONIndex100k(b *testing.B) { benchListNDJSON(b, 730*24*time.Hour) }

// BenchmarkListNDJSONIndex100kOpen is the worst case: all 100,000 in the
// open months, so the first line waits on parsing the whole root.
func BenchmarkListNDJSONIndex100kOpen(b *testing.B) { benchListNDJSON(b, 20*24*time.Hour) }

// seedNotes writes n inline episodes directly, a label and a paragraph of
// text each, alternating between two standpoints, and one in a hundred
// mentioning an invoice; then rebuilds the index once.
func seedNotes(b *testing.B, dir string, n int) {
	b.Helper()
	w, _, err := store.Init(dir, store.NewConfig())
	if err != nil {
		b.Fatal(err)
	}
	end := time.Now().UTC().Add(-24 * time.Hour)
	step := 14 * 24 * time.Hour / time.Duration(n)
	para := strings.Repeat("We talked through the session timeout and the deploy plan for next week. ", 3)
	for i := 0; i < n; i++ {
		id := idAt(end.Add(-time.Duration(n-i) * step))
		sp, text := jane, para
		if i%2 == 1 {
			sp = ada
		}
		if i%100 == 0 {
			text += "Paid the INVOICE today."
		}
		ep := &model.Episode{ID: id, Type: model.TypeEpisode, Kind: "note",
			Occurred:   model.Occurred{Start: "2026-11-07T14:24:00+11:00", Basis: model.BasisStated},
			Standpoint: sp, Label: fmt.Sprintf("Note %d", i),
			Content: model.Content{Mode: model.ModeInline, MediaType: "text/markdown", Derivation: "verbatim", Text: &text},
			Source:  model.Source{Author: ada}, Timestamp: "2026-11-08T00:00:00Z"}
		data, _ := model.Encode(ep)
		p := w.Path(id)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := w.Rebuild(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkQuery20k is one `query` over 20,000 inline episodes in the open
// months, unfiltered (every file parsed and matched) and with --standpoint
// (half of them).
func BenchmarkQuery20k(b *testing.B) {
	benchEnv(b)
	dir := filepath.Join(b.TempDir(), "ws")
	seedNotes(b, dir, 20000)
	for _, c := range []struct {
		name string
		args []string
	}{{"unfiltered", nil}, {"standpoint", []string{"--standpoint", jane}}} {
		b.Run(c.name, func(b *testing.B) {
			args := append([]string{"query", "invoice ada", "--workspace", dir, "--json"}, c.args...)
			for i := 0; i < b.N; i++ {
				var out, errb bytes.Buffer
				if code := Execute(args, strings.NewReader(""), &out, &errb, func() bool { return false }); code != 0 {
					b.Fatalf("exit %d: %s", code, errb.String())
				}
			}
		})
	}
}
