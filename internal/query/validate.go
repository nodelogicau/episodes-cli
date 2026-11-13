// Package query holds pure functions over a loaded workspace: validation,
// recall (list, thread, children), ordinal assignment and duplicate lookup.
package query

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

// Finding is one validation result.
type Finding struct {
	Severity model.Severity `json:"severity"`
	Code     string         `json:"code"`
	ID       string         `json:"id,omitempty"`
	Path     string         `json:"path,omitempty"`
	Field    string         `json:"field,omitempty"`
	Message  string         `json:"message"`
}

// Report is the result of validate.
type Report struct {
	Findings []Finding `json:"findings"`
	Episodes int       `json:"episodes"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Infos    int       `json:"infos"`
}

// Workspace is everything validate reads.
type Workspace struct {
	WS      *store.Workspace
	Listing *store.Listing
	Loaded  []store.Loaded
	Now     time.Time
}

type validator struct {
	in  Workspace
	out []Finding
}

func (v *validator) add(sev model.Severity, code, id, path, field, format string, args ...any) {
	v.out = append(v.out, Finding{Severity: sev, Code: code, ID: id, Path: path, Field: field, Message: fmt.Sprintf(format, args...)})
}

// Validate checks every episode file, the root index and every segment. It
// writes nothing. It needs nothing outside the workspace, and reports
// nothing about objects of other formats: a reference is opaque.
func Validate(in Workspace) *Report {
	v := &validator{in: in}
	byID := map[string]*model.Episode{}
	var eps []*model.Episode
	for _, ld := range in.Loaded {
		f := ld.File
		if ld.Err != nil {
			v.add(model.SevError, "unparseable", f.ID, f.Rel, "", "%v", ld.Err)
			continue
		}
		ep := ld.Episode
		if ep.ID != f.ID {
			v.add(model.SevError, "id_mismatch", ep.ID, f.Rel, "id", "the file is named for %s but carries id %q", f.ID, ep.ID)
		}
		if f.Misplaced() {
			v.add(model.SevError, "misplaced", f.ID, f.Rel, "", "%s is in a directory its id does not derive; its path is %s", f.Rel, store.RelPath(f.ID))
		}
		for _, p := range ld.Problems {
			v.add(p.Severity, p.Code, ep.ID, f.Rel, p.Field, "%s", p.Message)
		}
		for _, p := range model.Check(ep, model.ForValidate) {
			v.add(p.Severity, p.Code, ep.ID, f.Rel, p.Field, "%s", p.Message)
		}
		v.clock(ep, f)
		if _, dup := byID[ep.ID]; dup {
			v.add(model.SevError, "duplicate_id", ep.ID, f.Rel, "id", "id %s appears in more than one file", ep.ID)
			continue
		}
		byID[ep.ID] = ep
		eps = append(eps, ep)
	}
	v.relations(eps, byID)
	v.index(byID)
	sort.SliceStable(v.out, func(i, j int) bool {
		a, b := v.out[i], v.out[j]
		if rank(a.Severity) != rank(b.Severity) {
			return rank(a.Severity) < rank(b.Severity)
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Code < b.Code
	})
	r := &Report{Findings: v.out, Episodes: len(in.Loaded)}
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	for _, f := range r.Findings {
		switch f.Severity {
		case model.SevError:
			r.Errors++
		case model.SevWarning:
			r.Warnings++
		default:
			r.Infos++
		}
	}
	return r
}

func rank(s model.Severity) int {
	switch s {
	case model.SevError:
		return 0
	case model.SevWarning:
		return 1
	}
	return 2
}

// clock reports broken clocks: an id or timestamp later than now, and an id
// two or more months after the current month.
func (v *validator) clock(ep *model.Episode, f store.FileRef) {
	now := v.in.Now
	if t, ok := model.MintedAt(ep.ID); ok {
		if store.MonthOfTime(t) >= store.MonthOfTime(now)+2 {
			v.add(model.SevError, "id_far_future", ep.ID, f.Rel, "id", "id %s was minted in %s, two or more months after the current month %s; it would seal the month every correct writer mints into",
				ep.ID, t.Format("2006-01"), now.UTC().Format("2006-01"))
		} else if t.After(now) {
			v.add(model.SevWarning, "id_from_future", ep.ID, f.Rel, "id", "id %s was minted at %s, later than now (%s): a broken clock", ep.ID, t.Format(time.RFC3339), temporal.FormatTimestamp(now))
		}
	}
	if ts, err := temporal.ParseTimestamp(ep.Timestamp); err == nil && ts.After(now) {
		v.add(model.SevWarning, "timestamp_from_future", ep.ID, f.Rel, "timestamp", "timestamp %s is later than now (%s): a broken clock", ep.Timestamp, temporal.FormatTimestamp(now))
	}
}

func (v *validator) relations(eps []*model.Episode, byID map[string]*model.Episode) {
	rel := func(id string) string { return store.RelPath(id) }
	children := map[string][]*model.Episode{}
	for _, ep := range eps {
		if ep.Container != "" {
			children[ep.Container] = append(children[ep.Container], ep)
		}
	}
	for _, ep := range eps {
		if c := ep.Container; c != "" && model.IsID(c) {
			if _, ok := byID[c]; !ok {
				v.add(model.SevWarning, "container_unresolved", ep.ID, rel(ep.ID), "container", "container %s does not resolve in this workspace", c)
			}
			if c >= ep.ID {
				v.add(model.SevWarning, "container_later", ep.ID, rel(ep.ID), "container", "container %s was not minted before %s; a container is recorded before its children", c, ep.ID)
			}
		}
		for _, l := range ep.Links {
			if !model.IsID(l.Href) {
				continue
			}
			if _, ok := byID[l.Href]; !ok {
				v.add(model.SevInfo, "link_unresolved", ep.ID, rel(ep.ID), "links", "%s links %s to %s, which does not resolve in this workspace", ep.ID, l.Rel, l.Href)
			}
		}
		// Self-attested acts, recognised by shape.
		if ep.Content.Mode == model.ModeNone && len(children[ep.ID]) == 0 {
			v.act(ep, byID)
		}
	}
	// (container, ordinal) repeated.
	containers := make([]string, 0, len(children))
	for c := range children {
		containers = append(containers, c)
	}
	sort.Strings(containers)
	for _, c := range containers {
		byOrd := map[int][]string{}
		for _, ch := range children[c] {
			if ch.Ordinal != nil {
				byOrd[*ch.Ordinal] = append(byOrd[*ch.Ordinal], ch.ID)
			}
		}
		ords := make([]int, 0, len(byOrd))
		for o := range byOrd {
			ords = append(ords, o)
		}
		sort.Ints(ords)
		for _, o := range ords {
			if ids := byOrd[o]; len(ids) > 1 {
				sort.Strings(ids)
				v.add(model.SevWarning, "ordinal_repeated", ids[0], rel(ids[0]), "ordinal", "%s all carry container %s and ordinal %d", strings.Join(ids, ", "), c, o)
			}
		}
	}
	// Possible duplicates.
	type key struct{ standpoint, mediaType, uid string }
	groups := map[key][]string{}
	for _, ep := range eps {
		if ep.Content.Mode == model.ModeRef && ep.Content.UID != "" {
			k := key{ep.Standpoint, strings.ToLower(ep.Content.MediaType), ep.Content.UID}
			groups[k] = append(groups[k], ep.ID)
		}
	}
	var keys []key
	for k, ids := range groups {
		if len(ids) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return groups[keys[i]][0] < groups[keys[j]][0] })
	for _, k := range keys {
		ids := groups[k]
		sort.Strings(ids)
		v.add(model.SevInfo, "possible_duplicate", ids[0], rel(ids[0]), "content.uid", "%s share standpoint %s, media_type %s and uid %s: possibly, never certainly, one recording made twice; both are valid",
			strings.Join(ids, ", "), k.standpoint, k.mediaType, k.uid)
	}
}

func (v *validator) act(ep *model.Episode, byID map[string]*model.Episode) {
	rel := store.RelPath(ep.ID)
	if ep.Standpoint != ep.Source.Author {
		v.add(model.SevError, "act_standpoint_mismatch", ep.ID, rel, "standpoint", "a self-attested act (content mode none, no children) must have standpoint equal to source.author: standpoint is %q, source.author is %q", ep.Standpoint, ep.Source.Author)
	}
	for _, l := range ep.Links {
		if l.Rel != "via" || !model.IsID(l.Href) {
			continue
		}
		if t, ok := byID[l.Href]; ok && t.Standpoint != ep.Standpoint {
			v.add(model.SevError, "act_via_standpoint", ep.ID, rel, "links", "a self-attested act is attested by its own standpoint: via %s has standpoint %q, the act %q", l.Href, t.Standpoint, ep.Standpoint)
		}
	}
	if ep.Kind == "conversation" || ep.Kind == "thread" {
		v.add(model.SevInfo, "container_without_children", ep.ID, rel, "", "%s is a %s with content mode none and no children, so it is read as a self-attested act; if it is a container whose first child was lost, `episodes conversation turn %s` appends a child, which makes it a container again and changes no existing file", ep.ID, ep.Kind, ep.ID)
	}
}

// index checks the root and every segment: readability, sealed counts
// against directory listings, and every entry against its file.
func (v *validator) index(byID map[string]*model.Episode) {
	ws, l := v.in.WS, v.in.Listing
	root, _, err := ws.ReadRoot()
	if err != nil {
		v.add(model.SevError, "index_unreadable", "", store.IndexFile, "", "%v; run `episodes index` to rebuild it from the files", err)
		return
	}
	check := func(rel string, entries []store.Entry) {
		seen := map[string]bool{}
		for _, e := range entries {
			if seen[e.ID] {
				v.add(model.SevError, "index_duplicate_entry", e.ID, rel, "", "%s lists %s more than once", rel, e.ID)
			}
			seen[e.ID] = true
			ep, ok := byID[e.ID]
			if !ok {
				continue
			}
			if diff := e.Differs(store.EntryFor(ep)); len(diff) > 0 {
				v.add(model.SevError, "revised", e.ID, store.RelPath(e.ID), strings.Join(diff, ","), "%s differs from its entry in %s in %s: an indexed field was revised, and episodes are never revised", store.RelPath(e.ID), rel, strings.Join(diff, ", "))
			}
		}
	}
	check(store.IndexFile, root.Entries)
	listed := map[string]bool{}
	for _, s := range root.Segments {
		listed[s.Path] = true
		seg, _, err := ws.ReadSegment(s.Path)
		if err != nil {
			v.add(model.SevError, "index_unreadable", "", s.Path, "", "segment %s listed in %s cannot be read: %v", s.Path, store.IndexFile, err)
			continue
		}
		check(s.Path, seg.Entries)
		if s.Path == store.LegacySegment {
			continue
		}
		dir := strings.TrimSuffix(strings.TrimPrefix(s.Path, store.IndexDir+"/"), ".yaml")
		var files []store.FileRef
		for _, f := range l.ByDir[dir] {
			if !f.Misplaced() {
				files = append(files, f)
			}
		}
		if len(files) != s.Count || len(seg.Entries) != s.Count {
			have := map[string]bool{}
			for _, e := range seg.Entries {
				have[e.ID] = true
			}
			var late []string
			for _, f := range files {
				if !have[f.ID] {
					late = append(late, f.Rel)
				}
			}
			v.add(model.SevError, "late_merge", "", s.Path, "", "sealed month %s holds %d files and its segment counts %d (%d entries); not in the segment: %s. Remedy: %s",
				dir, len(files), s.Count, len(seg.Entries), orNone(late), store.LateMergeRemedy)
		}
	}
	// A sealed month with files and no listed segment.
	for dir, files := range l.ByDir {
		m, ok := store.MonthOfName(dir)
		if !ok || l.Months.Open(m) || len(files) == 0 || listed[store.SegmentPath(m)] {
			continue
		}
		v.add(model.SevInfo, "segment_missing", "", store.SegmentPath(m), "", "sealed month %s has no segment listed in %s; run `episodes index`", dir, store.IndexFile)
	}
	// Files the index lacks: normal after a merge, and recall is unaffected.
	indexed := map[string]bool{}
	for _, e := range root.Entries {
		indexed[e.ID] = true
	}
	var missing int
	for _, f := range l.Files {
		if m, ok := store.IDMonth(f.ID); ok && l.Months.Open(m) && !indexed[f.ID] && !f.Misplaced() {
			missing++
		}
	}
	if missing > 0 {
		v.add(model.SevInfo, "index_stale", "", store.IndexFile, "", "%d episode file(s) in open months have no entry in %s; run `episodes index`", missing, store.IndexFile)
	}
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "(none; an entry or count is wrong)"
	}
	return strings.Join(s, ", ")
}
