package query

import (
	"sort"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

// Interval returns the instants an occurred block spans: from the start
// bound's start to the end bound's end, or to the start bound's end when
// there is no end (the whole day, for a date alone). A date alone is reckoned
// in tz, or UTC. ok is false when start does not parse.
func Interval(o model.Occurred) (from, to time.Time, ok bool) {
	start, err := temporal.Parse(o.Start)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	loc := zone(o.TZ)
	from, to = start.Start(loc), start.End(loc)
	if o.End != "" {
		if end, err := temporal.Parse(o.End); err == nil {
			to = end.End(loc)
		}
	}
	return from, to, true
}

func zone(name string) *time.Location {
	if name == "" {
		return nil
	}
	loc, err := temporal.LoadZone(name)
	if err != nil {
		return nil
	}
	return loc
}

// Filter selects episodes by their indexed fields.
type Filter struct {
	Since, Until     *temporal.Value
	Standpoint, Kind string
	Container        string
}

// Match reports whether an entry passes the filter. The occurred interval
// must overlap [since, until], each bound a date (the whole day) or a
// date-time.
func (f Filter) Match(e store.Entry) bool {
	if f.Standpoint != "" && e.Standpoint != f.Standpoint {
		return false
	}
	if f.Kind != "" && e.Kind != f.Kind {
		return false
	}
	if f.Container != "" && e.Container != f.Container {
		return false
	}
	if f.Since == nil && f.Until == nil {
		return true
	}
	from, to, ok := Interval(e.Occurred)
	if !ok {
		return false
	}
	if f.Since != nil && to.Before(f.Since.Start(nil)) {
		return false
	}
	if u := f.Until; u != nil {
		// A date includes its whole day; a date-time is inclusive.
		if u.DateOnly && !from.Before(u.End(nil)) || !u.DateOnly && from.After(u.Instant) {
			return false
		}
	}
	return true
}

// SortByOccurred orders entries by the occurred.start instant, a date alone
// counted as the start of that day in tz or UTC, ties broken by id. Entries
// whose start does not parse come last, by id.
func SortByOccurred(es []store.Entry) {
	at := func(e store.Entry) (time.Time, bool) {
		from, _, ok := Interval(e.Occurred)
		return from, ok
	}
	sort.SliceStable(es, func(i, j int) bool {
		a, aok := at(es[i])
		b, bok := at(es[j])
		switch {
		case aok != bok:
			return aok
		case aok && !a.Equal(b):
			return a.Before(b)
		}
		return es[i].ID < es[j].ID
	})
}

// SortByID orders entries by id: the order they were recorded in.
func SortByID(es []store.Entry) {
	sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
}

// SortChildren orders a container's children by ordinal, then by id; a
// child with no ordinal comes after those with one.
func SortChildren(es []store.Entry) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i].Ordinal, es[j].Ordinal
		switch {
		case (a == nil) != (b == nil):
			return a != nil
		case a != nil && *a != *b:
			return *a < *b
		}
		return es[i].ID < es[j].ID
	})
}

// Children returns the entries naming container, in thread order.
func Children(es []store.Entry, container string) []store.Entry {
	var out []store.Entry
	for _, e := range es {
		if e.Container == container {
			out = append(out, e)
		}
	}
	SortChildren(out)
	return out
}

// NextOrdinal is one more than the highest ordinal among a container's
// children, or 1.
func NextOrdinal(children []store.Entry) int {
	max := 0
	for _, c := range children {
		if c.Ordinal != nil && *c.Ordinal > max {
			max = *c.Ordinal
		}
	}
	return max + 1
}

// HolderOf returns the child holding ordinal, if any.
func HolderOf(children []store.Entry, ordinal int) (string, bool) {
	for _, c := range children {
		if c.Ordinal != nil && *c.Ordinal == ordinal {
			return c.ID, true
		}
	}
	return "", false
}

// Duplicates returns the reference episodes with the same standpoint,
// media_type and uid (uid compared exactly, media type without case).
func Duplicates(loaded []store.Loaded, standpoint, mediaType, uid string) []string {
	var out []string
	for _, ld := range loaded {
		ep := ld.Episode
		if ld.Err != nil || ep == nil || ep.Content.Mode != model.ModeRef {
			continue
		}
		if ep.Standpoint == standpoint && ep.Content.UID == uid && equalFold(ep.Content.MediaType, mediaType) {
			out = append(out, ep.ID)
		}
	}
	sort.Strings(out)
	return out
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
