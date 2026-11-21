// Batch writes and streamed reads (change bulk-import, #1).

package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// CreateBatch writes new episodes as Create does, each at the path its id
// derives and never over an existing file, and then upserts the index once
// for all of them (design D3). The caller holds the lock and passes the
// listing it took under it.
//
// Every sealed-month guard is checked, and every episode encoded, before
// any file is written, so either refusal writes nothing. Each file's bytes
// are synced as written, and the run is made durable by one barrier before
// the index names it. A failure after some files are written returns their
// paths, in order, with the error; the index is still upserted for them, and
// when that fails too the error says to run `episodes index`.
func (w *Workspace) CreateBatch(l *Listing, eps []*model.Episode) ([]string, error) {
	data := make([][]byte, len(eps))
	for i, ep := range eps {
		if err := l.SealedGuard(ep.ID); err != nil {
			return nil, err
		}
		b, err := model.Encode(ep)
		if err != nil {
			return nil, err
		}
		data[i] = b
	}
	var written []string
	var entries []Entry
	var werr error
	swept := map[string]bool{}
	for i, ep := range eps {
		rel := RelPath(ep.ID)
		p := w.Abs(rel)
		dir := filepath.Dir(p)
		if !swept[dir] {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				werr = err
				break
			}
			// Once per directory: under the lock no temporary file there
			// is in use, and this batch's own are removed as it goes.
			sweepTemps(dir)
			swept[dir] = true
		}
		if err := writeExclusiveWith(p, data[i], syncData); err != nil {
			werr = fmt.Errorf("%s: %w", rel, err)
			break
		}
		f := FileRef{ID: ep.ID, Rel: rel, Dir: Shard(ep.ID)}
		l.Files = append(l.Files, f)
		l.ByDir[f.Dir] = append(l.ByDir[f.Dir], f)
		if m, ok := IDMonth(ep.ID); ok {
			l.Months.See(m)
		}
		written = append(written, rel)
		entries = append(entries, EntryFor(ep))
	}
	if len(written) == 0 {
		return nil, werr
	}
	if err := syncBarrier(w.Abs(written[len(written)-1])); err != nil {
		werr = errors.Join(werr, fmt.Errorf("syncing the batch: %w", err))
	}
	if err := w.upsert(l, entries...); err != nil {
		return written, errors.Join(werr, fmt.Errorf("%d files written, index not updated (run `episodes index`): %w", len(written), err))
	}
	return written, werr
}

// NoteUIDs adds just-created episodes to the uid cache, when this process
// has loaded it, and saves it once. Otherwise the next lookup's reconcile
// parses the new files.
func (w *Workspace) NoteUIDs(eps []*model.Episode) {
	c := w.uids
	if c == nil {
		return
	}
	names := map[string]map[string]bool{}
	for _, ep := range eps {
		dir := Shard(ep.ID)
		d := c.Dirs[dir]
		if d == nil {
			d = &uidDir{}
			c.Dirs[dir] = d
		}
		seen := names[dir]
		if seen == nil {
			seen = make(map[string]bool, len(d.Names))
			for _, n := range d.Names {
				seen[n] = true
			}
			names[dir] = seen
		}
		if seen[ep.ID] {
			continue
		}
		d.add(ep.ID, ep)
		seen[ep.ID] = true
		c.changed = true
	}
	c.save()
}

type uidKey struct{ standpoint, mediaType, uid string }

// UIDSet is UIDCandidates for many lookups against one listing: the cache
// is reconciled once and indexed, so each lookup is a map read. A batch
// takes a new set for each fresh listing, which catches the cache up with
// whatever was written since.
type UIDSet struct{ by map[uidKey][]FileRef }

// UIDSet reconciles the uid cache with l and indexes it. Like
// UIDCandidates, it never fails, and every candidate is for the caller to
// verify against its file.
func (w *Workspace) UIDSet(l *Listing) *UIDSet {
	c := w.loadUIDCache()
	w.reconcile(c, l)
	s := &UIDSet{by: map[uidKey][]FileRef{}}
	for dir, d := range c.Dirs {
		if len(d.Refs) == 0 {
			continue
		}
		files := make(map[string]FileRef, len(l.ByDir[dir]))
		for _, f := range l.ByDir[dir] {
			files[f.ID] = f
		}
		for _, r := range d.Refs {
			if f, ok := files[r.ID]; ok {
				k := uidKey{r.Standpoint, r.MediaType, r.UID}
				s.by[k] = append(s.by[k], f)
			}
		}
	}
	c.save()
	return s
}

// Candidates returns the listed files whose cached standpoint, media_type
// (without case) and uid match.
func (s *UIDSet) Candidates(standpoint, mediaType, uid string) []FileRef {
	return s.by[uidKey{standpoint, lowerASCII(mediaType), uid}]
}

// EachEntry streams recall's view, the one Entries returns, in id order and
// without building it (design D6): for each listed file whose id month is at
// least minMonth, the index entry where the index has the id, and the
// file's own entry, parsed, where it does not. A file that does not parse is
// left out. Segments are read one month at a time as the ids reach them, and
// the root's entries only when an id is in no segment, so memory holds one
// month's index, the root and the listing. fn's error stops the stream and
// is returned.
func (w *Workspace) EachEntry(l *Listing, minMonth int, hasMin bool, fn func(Entry, FileRef) error) error {
	ix := w.openStreamIndex()
	for _, f := range l.Files {
		if hasMin {
			if m, ok := IDMonth(f.ID); ok && m < minMonth {
				continue
			}
		}
		if !f.Misplaced() {
			if e, ok := ix.lookup(f.ID); ok {
				if err := fn(e, f); err != nil {
					return err
				}
				continue
			}
		}
		ld := w.load(f)
		if ld.Err != nil || ld.Episode.ID != f.ID {
			continue
		}
		if err := fn(EntryFor(ld.Episode), f); err != nil {
			return err
		}
	}
	return nil
}

// streamIndex is the committed index read lazily for EachEntry.
type streamIndex struct {
	w        *Workspace
	ok       bool
	rootData []byte
	listed   map[string]bool
	root     map[string]Entry // nil until first needed
	segRel   string
	seg      map[string]Entry
	legacy   map[string]Entry // nil until first needed
}

// openStreamIndex reads the root's head (format and segments) without
// parsing its entries. A root that cannot be read, or carries conflict
// markers, gives no index, and every file is parsed.
func (w *Workspace) openStreamIndex() *streamIndex {
	ix := &streamIndex{w: w}
	data, err := os.ReadFile(w.Abs(IndexFile))
	if err != nil {
		return ix
	}
	for _, m := range conflictMarkers {
		if bytes.HasPrefix(data, m) || bytes.Contains(data, append([]byte("\n"), m...)) {
			return ix
		}
	}
	head := data
	if h, _, found := bytes.Cut(data, []byte("\nentries:")); found {
		head = h
	}
	raw, _, err := decodeIndex(head, IndexFile)
	if err != nil {
		return ix
	}
	ix.ok, ix.rootData = true, data
	ix.listed = map[string]bool{}
	for _, s := range raw.Segments {
		ix.listed[s.Path] = true
	}
	return ix
}

func entryMap(es []Entry) map[string]Entry {
	m := make(map[string]Entry, len(es))
	for _, e := range es {
		m[e.ID] = e
	}
	return m
}

func (ix *streamIndex) segment(rel string) map[string]Entry {
	if seg, _, err := ix.w.ReadSegment(rel); err == nil {
		return entryMap(seg.Entries)
	}
	return map[string]Entry{}
}

func (ix *streamIndex) lookup(id string) (Entry, bool) {
	if !ix.ok {
		return Entry{}, false
	}
	if m, ok := IDMonth(id); ok {
		if rel := SegmentPath(m); ix.listed[rel] {
			if ix.segRel != rel {
				ix.segRel, ix.seg = rel, ix.segment(rel)
			}
			if e, ok := ix.seg[id]; ok {
				return e, true
			}
		}
	} else if ix.listed[LegacySegment] {
		if ix.legacy == nil {
			ix.legacy = ix.segment(LegacySegment)
		}
		if e, ok := ix.legacy[id]; ok {
			return e, true
		}
	}
	if ix.root == nil {
		ix.root = map[string]Entry{}
		if _, es, err := decodeIndex(ix.rootData, IndexFile); err == nil {
			ix.root = entryMap(es)
		}
	}
	e, ok := ix.root[id]
	return e, ok
}
