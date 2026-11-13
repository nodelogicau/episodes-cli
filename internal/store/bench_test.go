package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// Seed writes n episode files minted in the open months and rebuilds the
// index once, as a workspace would look after n creates.
func Seed(b testing.TB, w *Workspace, n int, mk func(id string) *model.Episode) {
	b.Helper()
	start := time.Now().UTC().Add(-20 * 24 * time.Hour)
	for i := 0; i < n; i++ {
		id := idAt(start.Add(time.Duration(i) * time.Minute))
		data, err := model.Encode(mk(id))
		if err != nil {
			b.Fatal(err)
		}
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

// BenchmarkCreate20k is design D14's target: a Create, lock to index upsert,
// into a workspace whose open months hold 20,000 entries.
func BenchmarkCreate20k(b *testing.B) {
	cfg := NewConfig()
	w, _, err := Init(b.TempDir(), cfg)
	if err != nil {
		b.Fatal(err)
	}
	Seed(b, w, 20000, episode)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unlock, err := w.Lock()
		if err != nil {
			b.Fatal(err)
		}
		l, err := w.Scan()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := w.Create(l, episode(model.NewID())); err != nil {
			b.Fatal(err)
		}
		unlock()
	}
}
