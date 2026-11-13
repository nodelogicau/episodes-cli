package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// Create writes a new episode at the path its id derives and upserts the
// index. The caller holds the lock and passes the listing it took under it.
// Create refuses a sealed month, and fails rather than overwrite an existing
// file; either way nothing is written.
func (w *Workspace) Create(l *Listing, ep *model.Episode) (string, error) {
	if err := l.SealedGuard(ep.ID); err != nil {
		return "", err
	}
	data, err := model.Encode(ep)
	if err != nil {
		return "", err
	}
	rel := RelPath(ep.ID)
	p := w.Abs(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	sweepTemps(filepath.Dir(p))
	if err := writeExclusive(p, data); err != nil {
		return "", fmt.Errorf("%s: %w", rel, err)
	}
	f := FileRef{ID: ep.ID, Rel: rel, Dir: Shard(ep.ID)}
	l.Files = append(l.Files, f)
	l.ByDir[f.Dir] = append(l.ByDir[f.Dir], f)
	if m, ok := IDMonth(ep.ID); ok {
		l.Months.See(m)
	}
	if err := w.upsert(l, EntryFor(ep)); err != nil {
		return rel, fmt.Errorf("%s written, index not updated (run `episodes index`): %w", rel, err)
	}
	return rel, nil
}

// ErrRevision is the store refusing a rewrite that would revise an episode.
var ErrRevision = errors.New("refused: episodes are never revised")

// Amend is the only path by which an existing episode file is rewritten. It
// reads the file, lets fn change a copy, and refuses the write unless the
// copy differs from the file only in source_status, in redacted, and in the
// fields redacted.fields removes, and unless every field an earlier
// redaction removed stays removed. The rewrite is atomic and the index is
// left alone, since neither block is indexed. The caller holds the lock.
func (w *Workspace) Amend(id string, fn func(ep *model.Episode) error) (*model.Episode, bool, error) {
	orig, data, err := w.Read(id)
	if err != nil {
		return nil, false, err
	}
	if _, probs, _ := model.Decode(data); len(probs) > 0 {
		return nil, false, fmt.Errorf("%s has malformed fields (%s); run `episodes validate`", RelPath(id), model.Problems(probs).Error())
	}
	next := orig.Clone()
	if err := fn(next); err != nil {
		return nil, false, err
	}
	if err := Guard(orig, next); err != nil {
		return nil, false, err
	}
	out, err := model.Encode(next)
	if err != nil {
		return nil, false, err
	}
	if bytes.Equal(out, data) {
		return next, false, nil
	}
	if err := writeAtomic(w.Path(id), out); err != nil {
		return nil, false, err
	}
	return next, true, nil
}

// Guard is accumulation as a property of the store: it fails unless next is
// orig with only source_status and redacted changed, the fields next's
// redaction lists removed, and nothing an earlier redaction removed restored.
func Guard(orig, next *model.Episode) error {
	if orig.ID != next.ID {
		return fmt.Errorf("%w: the id changed", ErrRevision)
	}
	if orig.Redacted != nil {
		if next.Redacted == nil {
			return fmt.Errorf("%w: a redacted block is never removed", ErrRevision)
		}
		for _, f := range orig.Redacted.Fields {
			if !next.Redacted.Lists(f) {
				return fmt.Errorf("%w: %s was redacted and is never restored", ErrRevision, f)
			}
		}
	}
	a, b := orig.Clone(), next.Clone()
	if next.Redacted != nil {
		for _, f := range next.Redacted.Fields {
			if !model.IsRedactable(f) {
				return fmt.Errorf("%w: %s is outside the closed set of redactable fields", ErrRevision, f)
			}
			if next.CarriesPath(f) {
				return fmt.Errorf("%w: redacted.fields lists %s, which is still present", ErrRevision, f)
			}
			a.RemovePath(f)
			b.RemovePath(f)
		}
	}
	a.SourceStatus, b.SourceStatus = nil, nil
	a.Redacted, b.Redacted = nil, nil
	ea, err := model.Encode(a)
	if err != nil {
		return err
	}
	eb, err := model.Encode(b)
	if err != nil {
		return err
	}
	if !bytes.Equal(ea, eb) {
		return fmt.Errorf("%w: only source_status and redaction may change an existing episode, and this change touches %s", ErrRevision, firstDiff(ea, eb))
	}
	return nil
}

// firstDiff names the first line where two encodings part, for the message.
func firstDiff(a, b []byte) string {
	la, lb := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := 0; i < len(la) && i < len(lb); i++ {
		if !bytes.Equal(la[i], lb[i]) {
			return fmt.Sprintf("%q", bytes.TrimSpace(la[i]))
		}
	}
	return "the end of the file"
}
