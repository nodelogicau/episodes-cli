package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// idAt mints a UUIDv7 episode id whose instant is t.
func idAt(t time.Time) string {
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

func month(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func newWS(t *testing.T) *Workspace {
	t.Helper()
	cfg := NewConfig()
	cfg.Defaults.Source.Author = "https://example.com/ada"
	w, _, err := Init(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func episode(id string) *model.Episode {
	text := "hi"
	ts, _ := model.MintedAt(id)
	stamp := ts.UTC().Truncate(time.Second).Format("2006-01-02T15:04:05Z")
	return &model.Episode{
		ID: id, Type: model.TypeEpisode, Kind: "note",
		Occurred:   model.Occurred{Start: stamp, Basis: model.BasisRecorded},
		Standpoint: "https://example.com/ada",
		Content:    model.Content{Mode: model.ModeInline, MediaType: model.DefaultMediaType, Derivation: model.DerivationVerbatim, Text: &text},
		Source:     model.Source{Author: "https://example.com/ada"},
		Timestamp:  stamp,
	}
}

func create(t *testing.T, w *Workspace, ep *model.Episode) {
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
	if _, err := w.Create(l, ep); err != nil {
		t.Fatal(err)
	}
}

func TestInitAndPaths(t *testing.T) {
	w := newWS(t)
	idx, _ := os.ReadFile(w.Abs(IndexFile))
	if string(idx) != "format: episodes/0.1\nsegments: []\nentries: []\n" {
		t.Fatalf("empty index: %q", idx)
	}
	if _, _, err := Init(w.Root, NewConfig()); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("re-init: %v", err)
	}
	id := idAt(month("2026-11-07"))
	if RelPath(id) != "episodes/2026-11/"+id+".yaml" || RelPath("epi_import-42") != "episodes/legacy/epi_import-42.yaml" {
		t.Fatal(RelPath(id))
	}
}

func TestFormatRefused(t *testing.T) {
	w := newWS(t)
	if err := os.WriteFile(w.Abs(ConfigFile), []byte("format: episodes/0.2\nworkspace: {id: x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(w.Root)
	if !errors.Is(err, ErrUnsupportedFormat) || !strings.Contains(err.Error(), "episodes/0.2") {
		t.Fatalf("%v", err)
	}
}

func TestDiscovery(t *testing.T) {
	w := newWS(t)
	proj := t.TempDir()
	src := filepath.Join(proj, "src")
	_ = os.MkdirAll(src, 0o755)
	if _, err := WritePointer(proj, w.Root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePointer(proj, "/elsewhere", false); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("pointer overwrite: %v", err)
	}
	t.Chdir(src)
	t.Setenv(EnvWorkspace, "")
	got, err := Discover("")
	if err != nil || got.Root != w.Root || got.FoundBy != FoundByPointer {
		t.Fatalf("%+v %v", got, err)
	}
	t.Setenv(EnvWorkspace, t.TempDir())
	if _, err := Discover(""); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("strict env: %v", err)
	}
	t.Setenv(EnvWorkspace, src)
	if got, err := Discover(w.Root); err != nil || got.FoundBy != FoundByFlag {
		t.Fatalf("flag wins: %+v %v", got, err)
	}
	t.Chdir(w.Root)
	t.Setenv(EnvWorkspace, "")
	if got, err := Discover(""); err != nil || got.FoundBy != FoundByMarker {
		t.Fatalf("marker: %+v %v", got, err)
	}
}

func TestCreateAndCollision(t *testing.T) {
	w := newWS(t)
	ep := episode(idAt(month("2026-11-07")))
	create(t, w, ep)
	before, _ := os.ReadFile(w.Path(ep.ID))
	l, _ := w.Scan()
	other := episode(ep.ID)
	other.Kind = "call"
	if _, err := w.Create(l, other); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("collision: %v", err)
	}
	after, _ := os.ReadFile(w.Path(ep.ID))
	if !bytes.Equal(before, after) {
		t.Fatal("existing file changed")
	}
	root, _, err := w.ReadRoot()
	if err != nil || len(root.Entries) != 1 || root.Entries[0].ID != ep.ID {
		t.Fatalf("%+v %v", root, err)
	}
}

// accumulation: an interrupted write leaves no partial file, and the next
// write sweeps its temporary file.
func TestInterruptedWrite(t *testing.T) {
	w := newWS(t)
	id := idAt(month("2026-11-07"))
	dir := filepath.Dir(w.Path(id))
	_ = os.MkdirAll(dir, 0o755)
	tmp := filepath.Join(dir, "."+id+".yaml.123.tmp")
	_ = os.WriteFile(tmp, []byte("id: epi_"), 0o644)
	if _, err := os.Stat(w.Path(id)); !os.IsNotExist(err) {
		t.Fatal("target exists before rename")
	}
	create(t, w, episode(idAt(month("2026-11-08"))))
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temporary file remains after the next write")
	}
}

// accumulation: the guard catches a stray change.
func TestGuard(t *testing.T) {
	w := newWS(t)
	ep := episode(idAt(month("2026-11-07")))
	ep.Content = model.Content{Mode: model.ModeRef, Ref: "gmail:1"}
	create(t, w, ep)
	before, _ := os.ReadFile(w.Path(ep.ID))
	_, _, err := w.Amend(ep.ID, func(e *model.Episode) error {
		e.SourceStatus = &model.SourceStatus{State: "gone", Verified: ep.Timestamp, Detected: ep.Timestamp}
		e.Label = "sneaky"
		return nil
	})
	if !errors.Is(err, ErrRevision) {
		t.Fatalf("guard: %v", err)
	}
	after, _ := os.ReadFile(w.Path(ep.ID))
	if !bytes.Equal(before, after) {
		t.Fatal("file changed")
	}
	// The permitted change goes through.
	if _, changed, err := w.Amend(ep.ID, func(e *model.Episode) error {
		e.SourceStatus = &model.SourceStatus{State: "gone", Verified: ep.Timestamp, Detected: ep.Timestamp}
		return nil
	}); err != nil || !changed {
		t.Fatal(err)
	}
	// Restoring a redacted field is refused.
	if _, _, err := w.Amend(ep.ID, func(e *model.Episode) error {
		e.Content.Ref = ""
		e.Redacted = &model.Redacted{Timestamp: ep.Timestamp, Reason: "r", Source: ep.Source, Fields: []string{"content.ref"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.Amend(ep.ID, func(e *model.Episode) error {
		e.Content.Ref = "gmail:1"
		e.Redacted.Fields = nil
		return nil
	}); !errors.Is(err, ErrRevision) {
		t.Fatalf("restore: %v", err)
	}
}

// sharding and index: sealing across a year boundary, no segment for an
// empty month, and refusal to mint into a sealed month.
func TestSealingAcrossYear(t *testing.T) {
	w := newWS(t)
	oct1, oct2 := episode(idAt(month("2026-10-03"))), episode(idAt(month("2026-10-20")))
	nov := episode(idAt(month("2026-11-07")))
	for _, e := range []*model.Episode{oct1, oct2, nov} {
		create(t, w, e)
	}
	root, _, _ := w.ReadRoot()
	if len(root.Entries) != 3 || len(root.Segments) != 0 {
		t.Fatalf("before: %+v", root)
	}
	jan := episode(idAt(month("2027-01-02")))
	create(t, w, jan)
	root, _, _ = w.ReadRoot()
	want := []SegmentRef{{Path: "index/2026-10.yaml", Count: 2}, {Path: "index/2026-11.yaml", Count: 1}}
	if fmt.Sprint(root.Segments) != fmt.Sprint(want) {
		t.Fatalf("segments %v", root.Segments)
	}
	if len(root.Entries) != 1 || root.Entries[0].ID != jan.ID {
		t.Fatalf("root entries %+v", root.Entries)
	}
	if _, err := os.Stat(w.Abs("index/2026-12.yaml")); !os.IsNotExist(err) {
		t.Fatal("segment written for an empty month")
	}
	seg, _, err := w.ReadSegment("index/2026-10.yaml")
	if err != nil || len(seg.Entries) != 2 {
		t.Fatalf("%+v %v", seg, err)
	}
	// A clock behind a sealed month.
	l, _ := w.Scan()
	if _, err := w.Create(l, episode(idAt(month("2026-11-20")))); !errors.Is(err, ErrSealedMonth) {
		t.Fatalf("sealed: %v", err)
	}
	// December is open (grace month).
	create(t, w, episode(idAt(month("2026-12-20"))))
	// Rebuild leaves segments byte-identical and reports nothing.
	segBefore, _ := os.ReadFile(w.Abs("index/2026-10.yaml"))
	if res, err := w.Rebuild(); err != nil || len(res.Late) != 0 {
		t.Fatal(res, err)
	}
	segAfter, _ := os.ReadFile(w.Abs("index/2026-10.yaml"))
	if !bytes.Equal(segBefore, segAfter) {
		t.Fatal("segment rewritten")
	}
	if d, err := w.CheckIndex(); err != nil || !d.Clean() {
		t.Fatalf("%+v %v", d, err)
	}
}

// sharding: a late merge is detected by count and never papered over.
func TestLateMerge(t *testing.T) {
	w := newWS(t)
	create(t, w, episode(idAt(month("2026-09-10"))))
	create(t, w, episode(idAt(month("2026-11-07"))))
	// A branch merged a September file after September sealed.
	late := episode(idAt(month("2026-09-28")))
	data, _ := model.Encode(late)
	_ = os.WriteFile(w.Path(late.ID), data, 0o644)
	segBefore, _ := os.ReadFile(w.Abs("index/2026-09.yaml"))
	res, err := w.Rebuild()
	if err != nil || len(res.Late) != 1 || res.Late[0].Files[0] != RelPath(late.ID) {
		t.Fatalf("%+v %v", res, err)
	}
	segAfter, _ := os.ReadFile(w.Abs("index/2026-09.yaml"))
	if !bytes.Equal(segBefore, segAfter) {
		t.Fatal("late-merged segment rewritten")
	}
	root, _, _ := w.ReadRoot()
	if root.Segments[0].Count != 1 {
		t.Fatalf("count papered over: %v", root.Segments)
	}
	if d, _ := w.CheckIndex(); d.Clean() || len(d.Late) != 1 {
		t.Fatalf("%+v", d)
	}
}

// index: drift detected, conflict markers replaced, and recall over a
// lagging index.
func TestDriftAndConflict(t *testing.T) {
	w := newWS(t)
	a := episode(idAt(month("2026-11-07")))
	create(t, w, a)
	merged := episode(idAt(month("2026-11-08")))
	data, _ := model.Encode(merged)
	_ = os.WriteFile(w.Path(merged.ID), data, 0o644)
	d, _ := w.CheckIndex()
	if d.Clean() || len(d.Missing) != 1 || d.Missing[0] != merged.ID {
		t.Fatalf("%+v", d)
	}
	l, _ := w.Scan()
	if es := w.Entries(l, 0, false); len(es) != 2 {
		t.Fatalf("recall over lagging index: %d", len(es))
	}
	// An index entry whose file is gone is dropped.
	_ = os.Remove(w.Path(a.ID))
	l, _ = w.Scan()
	if es := w.Entries(l, 0, false); len(es) != 1 || es[0].ID != merged.ID {
		t.Fatalf("dropped: %+v", es)
	}
	_ = os.WriteFile(w.Abs(IndexFile), []byte("<<<<<<< HEAD\nformat: x\n=======\n>>>>>>> b\n"), 0o644)
	if _, err := w.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if root, _, err := w.ReadRoot(); err != nil || len(root.Entries) != 1 {
		t.Fatalf("%+v %v", root, err)
	}
}

// accumulation: concurrent writers serialise on the lock, and the index is
// never torn.
func TestConcurrentCreates(t *testing.T) {
	w := newWS(t)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := w.Lock()
			if err != nil {
				errs <- err
				return
			}
			defer unlock()
			l, err := w.Scan()
			if err != nil {
				errs <- err
				return
			}
			if _, err := w.Create(l, episode(model.NewID())); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	root, _, err := w.ReadRoot()
	if err != nil || len(root.Entries) != 20 {
		t.Fatalf("torn index: %d %v", len(root.Entries), err)
	}
}

// accumulation: a killed writer releases the lock.
func TestKilledWriterReleasesLock(t *testing.T) {
	if os.Getenv("EPISODES_HOLD_LOCK") != "" {
		ws, err := Open(os.Getenv("EPISODES_HOLD_LOCK"))
		if err != nil {
			os.Exit(3)
		}
		if _, err := ws.Lock(); err != nil {
			os.Exit(4)
		}
		fmt.Println("locked")
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	w := newWS(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestKilledWriterReleasesLock$")
	cmd.Env = append(os.Environ(), "EPISODES_HOLD_LOCK="+w.Root)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 6)
	if _, err := out.Read(buf); err != nil || string(buf) != "locked" {
		t.Fatalf("child: %q %v", buf, err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	done := make(chan error, 1)
	go func() {
		unlock, err := w.Lock()
		if err == nil {
			unlock()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lock not released by a killed writer")
	}
}
