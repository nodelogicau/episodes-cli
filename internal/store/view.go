package store

import (
	"os"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// Loaded is one episode file as read, or the reason it could not be.
type Loaded struct {
	File     FileRef
	Episode  *model.Episode
	Data     []byte
	Problems []model.Problem
	Err      error
}

// LoadAll parses every episode file. It is validation's cost and duplicate
// lookup's, not recall's.
func (w *Workspace) LoadAll(l *Listing) []Loaded {
	out := make([]Loaded, 0, len(l.Files))
	for _, f := range l.Files {
		out = append(out, w.load(f))
	}
	return out
}

func (w *Workspace) load(f FileRef) Loaded {
	data, err := os.ReadFile(w.Abs(f.Rel))
	if err != nil {
		return Loaded{File: f, Err: err}
	}
	ep, probs, err := model.Decode(data)
	return Loaded{File: f, Episode: ep, Data: data, Problems: probs, Err: err}
}

// LoadFile parses one listed file.
func (w *Workspace) LoadFile(f FileRef) Loaded { return w.load(f) }

// Entries is recall's view: an entry for every file the directory listing
// finds whose id month is at least minMonth (legacy always included), taken
// from the index where the index has the id and parsed from the file where
// it does not. Index entries whose file is gone are dropped. A file that
// does not parse is left out. The index is a cache; this view is never
// wrong because it lags, only slower.
func (w *Workspace) Entries(l *Listing, minMonth int, hasMin bool) []Entry {
	indexed := map[string]Entry{}
	if root, _, err := w.ReadRoot(); err == nil {
		for _, e := range root.Entries {
			indexed[e.ID] = e
		}
		for _, s := range root.Segments {
			if hasMin && s.Path != LegacySegment {
				if m, ok := segmentMonth(s.Path); ok && m < minMonth {
					continue
				}
			}
			if seg, _, err := w.ReadSegment(s.Path); err == nil {
				for _, e := range seg.Entries {
					indexed[e.ID] = e
				}
			}
		}
	}
	var out []Entry
	for _, f := range l.Files {
		if hasMin {
			if m, ok := IDMonth(f.ID); ok && m < minMonth {
				continue
			}
		}
		if e, ok := indexed[f.ID]; ok && !f.Misplaced() {
			out = append(out, e)
			continue
		}
		ld := w.load(f)
		if ld.Err != nil || ld.Episode.ID != f.ID {
			continue
		}
		out = append(out, EntryFor(ld.Episode))
	}
	return out
}

func segmentMonth(rel string) (int, bool) {
	const pre, suf = IndexDir + "/", ".yaml"
	if len(rel) != len(pre)+7+len(suf) {
		return 0, false
	}
	return MonthOfName(rel[len(pre) : len(pre)+7])
}
