// The derived index: the root index.yaml and its sealed segments.
//
// Origin: nodelogicau/particulars-cli internal/store/sharded.go @ e8933aacecad
// (the root/segment split, segment counts, never rewriting a segment, and
// late-merge detection by count, reshaped for one object type, no
// retractions, and the seven-field entry)

package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// Entry mirrors exactly the indexed fields of one episode.
//
// Canonical order: id, kind, occurred, container, ordinal, standpoint,
// timestamp.
type Entry struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Occurred   model.Occurred `json:"-"`
	Container  string         `json:"container,omitempty"`
	Ordinal    *int           `json:"ordinal,omitempty"`
	Standpoint string         `json:"standpoint"`
	Timestamp  string         `json:"timestamp"`
}

// EntryFor is the index entry of an episode.
func EntryFor(e *model.Episode) Entry {
	o := e.Occurred
	o.Extra = nil
	return Entry{ID: e.ID, Kind: e.Kind, Occurred: o, Container: e.Container, Ordinal: e.Ordinal, Standpoint: e.Standpoint, Timestamp: e.Timestamp}
}

// Differs names the indexed fields on which two entries disagree.
func (a Entry) Differs(b Entry) []string {
	var out []string
	if a.Kind != b.Kind {
		out = append(out, "kind")
	}
	if a.Occurred.Start != b.Occurred.Start || a.Occurred.End != b.Occurred.End || a.Occurred.TZ != b.Occurred.TZ || a.Occurred.Basis != b.Occurred.Basis {
		out = append(out, "occurred")
	}
	if a.Container != b.Container {
		out = append(out, "container")
	}
	if (a.Ordinal == nil) != (b.Ordinal == nil) || (a.Ordinal != nil && *a.Ordinal != *b.Ordinal) {
		out = append(out, "ordinal")
	}
	if a.Standpoint != b.Standpoint {
		out = append(out, "standpoint")
	}
	if a.Timestamp != b.Timestamp {
		out = append(out, "timestamp")
	}
	return out
}

func (a Entry) node() *yaml.Node {
	ep := &model.Episode{ID: a.ID, Kind: a.Kind, Container: a.Container, Ordinal: a.Ordinal, Standpoint: a.Standpoint}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add := func(k string, v *yaml.Node) {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, v)
	}
	str := func(k, v string) {
		if v != "" {
			add(k, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
		}
	}
	str("id", ep.ID)
	str("kind", ep.Kind)
	if o := model.OccurredNode(a.Occurred, true); o != nil {
		add("occurred", o)
	}
	str("container", a.Container)
	if a.Ordinal != nil {
		add("ordinal", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(*a.Ordinal)})
	}
	str("standpoint", a.Standpoint)
	if a.Timestamp != "" {
		add("timestamp", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: a.Timestamp})
	}
	return m
}

// SegmentRef is one root segments item.
type SegmentRef struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// Root is index.yaml: the segment list and the open months' entries.
type Root struct {
	Format   string
	Segments []SegmentRef
	Entries  []Entry
}

// Segment is a sealed month's index/<YYYY-MM>.yaml, or index/legacy.yaml.
type Segment struct {
	Format  string
	Entries []Entry
}

// LegacySegment is the segment for ids from which no month derives.
const LegacySegment = IndexDir + "/legacy.yaml"

// SegmentPath is a month's segment path.
func SegmentPath(month int) string { return IndexDir + "/" + MonthName(month) + ".yaml" }

func sortEntries(es []Entry) {
	sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
}

func entriesNode(es []Entry) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if len(es) == 0 {
		seq.Style = yaml.FlowStyle
	}
	for _, e := range es {
		seq.Content = append(seq.Content, e.node())
	}
	return seq
}

func kv(k string, v *yaml.Node) []*yaml.Node {
	return []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, v}
}

// EncodeRoot renders index.yaml.
func EncodeRoot(r *Root) ([]byte, error) {
	sortEntries(r.Entries)
	segs := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if len(r.Segments) == 0 {
		segs.Style = yaml.FlowStyle
	}
	for _, s := range r.Segments {
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		m.Content = append(m.Content, kv("path", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s.Path})...)
		m.Content = append(m.Content, kv("count", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(s.Count)})...)
		segs.Content = append(segs.Content, m)
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, kv("format", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: r.Format})...)
	m.Content = append(m.Content, kv("segments", segs)...)
	m.Content = append(m.Content, kv("entries", entriesNode(r.Entries))...)
	return model.EncodeNode(m)
}

// EncodeSegment renders a segment.
func EncodeSegment(s *Segment) ([]byte, error) {
	sortEntries(s.Entries)
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, kv("format", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s.Format})...)
	m.Content = append(m.Content, kv("entries", entriesNode(s.Entries))...)
	return model.EncodeNode(m)
}

// ErrIndexUnreadable marks an index document that does not parse, such as a
// root holding git conflict markers.
var ErrIndexUnreadable = errors.New("index unreadable")

type rawIndex struct {
	Format   string       `yaml:"format"`
	Segments []SegmentRef `yaml:"segments"`
	Entries  []yaml.Node  `yaml:"entries"`
}

func decodeIndex(data []byte, rel string) (*rawIndex, []Entry, error) {
	var raw rawIndex
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %v", ErrIndexUnreadable, rel, err)
	}
	if raw.Format != model.Format {
		return nil, nil, fmt.Errorf("%w: %s: format %q", ErrIndexUnreadable, rel, raw.Format)
	}
	out := make([]Entry, 0, len(raw.Entries))
	for i := range raw.Entries {
		ep, _, err := model.DecodeNode(&raw.Entries[i])
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: entry %d: %v", ErrIndexUnreadable, rel, i, err)
		}
		out = append(out, EntryFor(ep))
	}
	return &raw, out, nil
}

// ReadRoot parses index.yaml.
func (w *Workspace) ReadRoot() (*Root, []byte, error) {
	data, err := os.ReadFile(w.Abs(IndexFile))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %v", ErrIndexUnreadable, IndexFile, err)
	}
	raw, entries, err := decodeIndex(data, IndexFile)
	if err != nil {
		return nil, data, err
	}
	return &Root{Format: raw.Format, Segments: raw.Segments, Entries: entries}, data, nil
}

// ReadSegment parses one segment by its root-relative path.
func (w *Workspace) ReadSegment(rel string) (*Segment, []byte, error) {
	data, err := os.ReadFile(w.Abs(rel))
	if err != nil {
		return nil, nil, err
	}
	raw, entries, err := decodeIndex(data, rel)
	if err != nil {
		return nil, data, err
	}
	return &Segment{Format: raw.Format, Entries: entries}, data, nil
}

func (w *Workspace) writeRoot(r *Root) error {
	data, err := EncodeRoot(r)
	if err != nil {
		return err
	}
	return w.writeIfChanged(IndexFile, data)
}

func (w *Workspace) writeIfChanged(rel string, data []byte) error {
	p := w.Abs(rel)
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(w.Abs(IndexDir), 0o755); err != nil {
		return err
	}
	return writeAtomic(p, data)
}

// writeNewSegment writes a sealed segment that does not yet exist. It never
// replaces one: a segment, once written, never changes.
func (w *Workspace) writeNewSegment(rel string, s *Segment) error {
	data, err := EncodeSegment(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(w.Abs(IndexDir), 0o755); err != nil {
		return err
	}
	return writeExclusive(w.Abs(rel), data)
}

// entriesOf parses files into entries, skipping any that do not parse:
// validate reports those.
func (w *Workspace) entriesOf(files []FileRef) []Entry {
	out := make([]Entry, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(w.Abs(f.Rel))
		if err != nil {
			continue
		}
		ep, _, err := model.Decode(data)
		if err != nil || ep.ID != f.ID {
			continue
		}
		out = append(out, EntryFor(ep))
	}
	return out
}

// LateMerge is a sealed month whose directory and segment disagree.
type LateMerge struct {
	Segment string `json:"segment"`
	// Files are the episode files in the month's directory that the segment
	// does not hold.
	Files []string `json:"files"`
	// Count is the segment's count; Present is the number of files.
	Count   int `json:"count"`
	Present int `json:"present"`
}

// LateMergeRemedy is what to do about a late merge.
const LateMergeRemedy = "a branch open across two or more month boundaries added files to a sealed month; re-mint that branch's episodes (record them again, which gives them current ids) before merging, and remove the late files"

// plan is the index a workspace's files imply.
type plan struct {
	months   Months
	root     *Root
	segments map[string]*Segment // to write: missing sealed segments, and legacy
	late     []LateMerge
	// listedCount is each sealed segment's count as the root should list it.
	listedCount map[string]int
}

// placed returns the files under their derived path, by directory: a
// misplaced file belongs to no month and is left to validate.
func placed(l *Listing) map[string][]FileRef {
	out := map[string][]FileRef{}
	for _, f := range l.Files {
		if !f.Misplaced() {
			out[f.Dir] = append(out[f.Dir], f)
		}
	}
	return out
}

// planIndex derives the whole index from the files. Sealed months whose
// segment exists are compared by count and parsed only when it differs.
func (w *Workspace) planIndex(l *Listing) (*plan, error) {
	p := &plan{months: l.Months, root: &Root{Format: model.Format}, segments: map[string]*Segment{}, listedCount: map[string]int{}}
	dirs := placed(l)
	names := make([]string, 0, len(dirs))
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)
	for _, d := range names {
		files := dirs[d]
		if d == LegacyShard {
			p.segments[LegacySegment] = &Segment{Format: model.Format, Entries: w.entriesOf(files)}
			p.listedCount[LegacySegment] = len(files)
			continue
		}
		m, ok := MonthOfName(d)
		if !ok {
			continue
		}
		if l.Months.Open(m) {
			p.root.Entries = append(p.root.Entries, w.entriesOf(files)...)
			continue
		}
		rel := SegmentPath(m)
		seg, _, err := w.ReadSegment(rel)
		switch {
		case errors.Is(err, os.ErrNotExist):
			p.segments[rel] = &Segment{Format: model.Format, Entries: w.entriesOf(files)}
			p.listedCount[rel] = len(files)
		case err != nil:
			return nil, err
		case len(seg.Entries) == len(files):
			p.listedCount[rel] = len(files)
		default:
			have := map[string]bool{}
			for _, e := range seg.Entries {
				have[e.ID] = true
			}
			lm := LateMerge{Segment: rel, Count: len(seg.Entries), Present: len(files)}
			for _, f := range files {
				if !have[f.ID] {
					lm.Files = append(lm.Files, f.Rel)
				}
			}
			p.late = append(p.late, lm)
			// The segment is never rewritten, so the root keeps the count of
			// what it holds and the late merge stays visible.
			p.listedCount[rel] = len(seg.Entries)
		}
	}
	for rel, n := range p.listedCount {
		p.root.Segments = append(p.root.Segments, SegmentRef{Path: rel, Count: n})
	}
	sortRefs(p.root.Segments)
	sortEntries(p.root.Entries)
	return p, nil
}

// sortRefs orders segment references legacy first, then months ascending.
func sortRefs(refs []SegmentRef) {
	sort.Slice(refs, func(i, j int) bool {
		if (refs[i].Path == LegacySegment) != (refs[j].Path == LegacySegment) {
			return refs[i].Path == LegacySegment
		}
		return refs[i].Path < refs[j].Path
	})
}

// RebuildResult is what `index` did.
type RebuildResult struct {
	Entries  int         `json:"entries"`
	Segments []string    `json:"segments_written"`
	Late     []LateMerge `json:"late_merges"`
}

// Rebuild regenerates the root from the files, replacing whatever was there
// (conflict markers included), and writes every missing sealed segment and
// the legacy segment. It never rewrites an existing sealed segment: a sealed
// month whose directory count differs from its segment is reported in Late
// and left as it is. The caller holds the lock.
func (w *Workspace) Rebuild() (*RebuildResult, error) {
	l, err := w.Scan()
	if err != nil {
		return nil, err
	}
	p, err := w.planIndex(l)
	if err != nil {
		return nil, err
	}
	res := &RebuildResult{Entries: len(p.root.Entries), Segments: []string{}, Late: p.late}
	rels := make([]string, 0, len(p.segments))
	for rel := range p.segments {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		s := p.segments[rel]
		if rel == LegacySegment {
			data, err := EncodeSegment(s)
			if err != nil {
				return nil, err
			}
			if err := w.writeIfChanged(rel, data); err != nil {
				return nil, err
			}
		} else if err := w.writeNewSegment(rel, s); err != nil {
			return nil, err
		}
		res.Segments = append(res.Segments, rel)
	}
	if _, ok := p.segments[LegacySegment]; !ok {
		if err := os.Remove(w.Abs(LegacySegment)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := w.writeRoot(p.root); err != nil {
		return nil, err
	}
	return res, nil
}

// IndexDiff is what `index --check` found.
type IndexDiff struct {
	RootUnreadable  string      `json:"root_unreadable,omitempty"`
	Missing         []string    `json:"missing"`
	Extra           []string    `json:"extra"`
	Changed         []string    `json:"changed"`
	SegmentsMissing []string    `json:"segments_missing"`
	SegmentsExtra   []string    `json:"segments_extra"`
	CountsChanged   []string    `json:"counts_changed"`
	Late            []LateMerge `json:"late_merges"`
}

// Clean reports whether the committed index matches the files.
func (d *IndexDiff) Clean() bool {
	return d.RootUnreadable == "" && len(d.Missing)+len(d.Extra)+len(d.Changed)+len(d.SegmentsMissing)+len(d.SegmentsExtra)+len(d.CountsChanged)+len(d.Late) == 0
}

// CheckIndex compares the committed index with the files, writing nothing.
func (w *Workspace) CheckIndex() (*IndexDiff, error) {
	l, err := w.Scan()
	if err != nil {
		return nil, err
	}
	p, err := w.planIndex(l)
	if err != nil {
		return nil, err
	}
	d := &IndexDiff{Missing: []string{}, Extra: []string{}, Changed: []string{}, SegmentsMissing: []string{},
		SegmentsExtra: []string{}, CountsChanged: []string{}, Late: p.late}
	if d.Late == nil {
		d.Late = []LateMerge{}
	}
	compare := func(want, got []Entry) {
		gotBy := map[string]Entry{}
		for _, e := range got {
			gotBy[e.ID] = e
		}
		wantBy := map[string]bool{}
		for _, e := range want {
			wantBy[e.ID] = true
			g, ok := gotBy[e.ID]
			switch {
			case !ok:
				d.Missing = append(d.Missing, e.ID)
			case len(e.Differs(g)) > 0:
				d.Changed = append(d.Changed, e.ID)
			}
		}
		for _, e := range got {
			if !wantBy[e.ID] {
				d.Extra = append(d.Extra, e.ID)
			}
		}
	}
	root, _, err := w.ReadRoot()
	if err != nil {
		d.RootUnreadable = err.Error()
		root = &Root{}
	}
	compare(p.root.Entries, root.Entries)
	listed := map[string]int{}
	for _, s := range root.Segments {
		listed[s.Path] = s.Count
	}
	for _, s := range p.root.Segments {
		n, ok := listed[s.Path]
		switch {
		case !ok:
			d.SegmentsMissing = append(d.SegmentsMissing, s.Path)
		case n != s.Count:
			d.CountsChanged = append(d.CountsChanged, s.Path)
		}
		if _, toWrite := p.segments[s.Path]; toWrite {
			got, _, err := w.ReadSegment(s.Path)
			if err != nil {
				if !contains(d.SegmentsMissing, s.Path) {
					d.SegmentsMissing = append(d.SegmentsMissing, s.Path)
				}
				for _, e := range p.segments[s.Path].Entries {
					d.Missing = append(d.Missing, e.ID)
				}
				continue
			}
			compare(p.segments[s.Path].Entries, got.Entries)
		}
		delete(listed, s.Path)
	}
	for path := range listed {
		d.SegmentsExtra = append(d.SegmentsExtra, path)
	}
	for _, l := range [][]string{d.Missing, d.Extra, d.Changed, d.SegmentsMissing, d.SegmentsExtra, d.CountsChanged} {
		sort.Strings(l)
	}
	return d, nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// upsert applies one create to the index under the lock: it seals any month
// the new id's month seals, writing each such month's segment from its files
// with their count, and upserts the new entry into the root. A root that
// cannot be read is rebuilt from the files instead. When nothing seals and
// the new id sorts last, the entry is appended to the root's bytes without
// parsing the rest, which gives the same bytes (spliceable).
func (w *Workspace) upsert(l *Listing, e Entry) error {
	if !forceFullUpsert {
		if data, err := os.ReadFile(w.Abs(IndexFile)); err == nil {
			if tail, ok := spliceable(data, e, l); ok {
				fastUpserts++
				return writeAtomic(w.Abs(IndexFile), append(data, tail...))
			}
		}
	}
	return w.fullUpsert(l, e)
}

// forceFullUpsert makes every upsert take the full path; tests set it to
// compare the two paths.
var forceFullUpsert bool

// fastUpserts counts upserts that took the splice, for tests.
var fastUpserts int

// entryBytes is one entry exactly as EncodeRoot writes it inside a block
// sequence of entries.
func entryBytes(e Entry) ([]byte, error) {
	one, err := EncodeRoot(&Root{Format: model.Format, Entries: []Entry{e}})
	if err != nil {
		return nil, err
	}
	_, after, ok := bytes.Cut(one, []byte("\nentries:\n"))
	if !ok {
		return nil, fmt.Errorf("index: unexpected encoding of one entry")
	}
	return after, nil
}

var conflictMarkers = [][]byte{[]byte("<<<<<<<"), []byte("======="), []byte(">>>>>>>")}

const entryLead = "\n  - id: "

// spliceable decides, from the root's bytes alone, whether appending e's
// encoded entry gives exactly what fullUpsert would write, and returns that
// entry's bytes when it does (design D2 of fast-index-upsert). It parses only
// the root's head (format and segments), never its entries.
func spliceable(data []byte, e Entry, l *Listing) ([]byte, bool) {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, false
	}
	for _, m := range conflictMarkers {
		if bytes.HasPrefix(data, m) || bytes.Contains(data, append([]byte("\n"), m...)) {
			return nil, false
		}
	}
	head, body, ok := bytes.Cut(data, []byte("\nentries:\n"))
	if !ok || !bytes.HasPrefix(body, []byte(entryLead[1:])) {
		return nil, false // no entries, or a flow sequence
	}
	// The head must be exactly what EncodeRoot writes for its segments.
	var raw struct {
		Format   string       `yaml:"format"`
		Segments []SegmentRef `yaml:"segments"`
	}
	if err := yaml.Unmarshal(head, &raw); err != nil || raw.Format != model.Format {
		return nil, false
	}
	canon, err := EncodeRoot(&Root{Format: model.Format, Segments: raw.Segments})
	if err != nil || !bytes.Equal(append(append([]byte{}, head...), "\nentries: []\n"...), canon) {
		return nil, false
	}
	// Nothing to seal: every sealed month with files already has its segment.
	listed := map[string]bool{}
	for _, s := range raw.Segments {
		listed[s.Path] = true
	}
	for d := range placed(l) {
		if dm, ok := MonthOfName(d); ok && !l.Months.Open(dm) && !listed[SegmentPath(dm)] {
			return nil, false
		}
	}
	// The first entry is open (so nothing is dropped) and the last sorts
	// before the new id (so the new entry goes last).
	first := lineAt(body, len(entryLead)-1)
	lastAt := bytes.LastIndex(data, []byte(entryLead))
	if lastAt < 0 {
		return nil, false
	}
	last := lineAt(data, lastAt+len(entryLead))
	if fm, ok := IDMonth(first); !ok || !l.Months.Open(fm) {
		return nil, false
	}
	if last == "" || last >= e.ID {
		return nil, false
	}
	tail, err := entryBytes(e)
	if err != nil {
		return nil, false
	}
	return tail, true
}

// lineAt reads from data at i up to the end of the line.
func lineAt(data []byte, i int) string {
	if i > len(data) {
		return ""
	}
	end := bytes.IndexByte(data[i:], '\n')
	if end < 0 {
		return ""
	}
	return string(data[i : i+end])
}

// fullUpsert parses the root, seals what the create seals, adds the entry and
// re-encodes the whole root.
func (w *Workspace) fullUpsert(l *Listing, e Entry) error {
	root, _, err := w.ReadRoot()
	if err != nil {
		_, err := w.Rebuild()
		return err
	}
	root.Format = model.Format
	m, ok := IDMonth(e.ID)
	if !ok {
		// Writers mint only UUIDv7, so a legacy entry reaches the index only
		// through a rebuild.
		_, err := w.Rebuild()
		return err
	}
	next := l.Months
	next.See(m)
	listed := map[string]bool{}
	for _, s := range root.Segments {
		listed[s.Path] = true
	}
	dirs := placed(l)
	for d, files := range dirs {
		dm, ok := MonthOfName(d)
		if !ok || next.Open(dm) {
			continue
		}
		rel := SegmentPath(dm)
		if listed[rel] {
			continue
		}
		// A month this create seals, or one sealed earlier whose segment
		// was never written: its segment comes from its files.
		if _, err := os.Stat(w.Abs(rel)); err == nil {
			seg, _, err := w.ReadSegment(rel)
			if err != nil {
				return err
			}
			root.Segments = append(root.Segments, SegmentRef{Path: rel, Count: len(seg.Entries)})
			continue
		}
		seg := &Segment{Format: model.Format, Entries: w.entriesOf(files)}
		if err := w.writeNewSegment(rel, seg); err != nil {
			return err
		}
		root.Segments = append(root.Segments, SegmentRef{Path: rel, Count: len(files)})
	}
	sortRefs(root.Segments)
	keep := root.Entries[:0]
	for _, x := range root.Entries {
		if x.ID == e.ID {
			continue
		}
		if xm, ok := IDMonth(x.ID); ok && !next.Open(xm) {
			continue
		}
		keep = append(keep, x)
	}
	root.Entries = append(keep, e)
	return w.writeRoot(root)
}
