// Zone loading and offset-at-local-time.
//
// Origin: nodelogicau/intentions-cli internal/temporal/context.go @ b72a395454aa
// (LocalTime's candidate-offset search, adapted to return every offset a
// wall-clock time can carry in a zone rather than choosing one)

package temporal

import (
	"fmt"
	"sort"
	"time"
)

// LoadZone loads an IANA zone by name. "Local" and the empty name are refused:
// a zone is written only when it is known, never taken from the host.
func LoadZone(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, fmt.Errorf("%q is not an IANA time zone name", name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%q is not a known IANA time zone", name)
	}
	return loc, nil
}

// OffsetsAt returns every UTC offset, in seconds east, that the wall-clock
// time (y-m-d h:m:s) carries in loc. It is one offset ordinarily, two in the
// hour repeated when clocks go back, and none in the hour skipped when they
// go forward. The result is sorted descending, so the first entry is the
// earlier instant.
func OffsetsAt(loc *time.Location, y int, mo time.Month, d, h, mi, s int) []int {
	naive := time.Date(y, mo, d, h, mi, s, 0, time.UTC)
	// Offsets in force well before and well after the wall time: at most one
	// transition lies within the window, so these are the only candidates.
	_, offBefore := naive.Add(-36 * time.Hour).In(loc).Zone()
	_, offAfter := naive.Add(36 * time.Hour).In(loc).Zone()
	cands := []int{offBefore}
	if offAfter != offBefore {
		cands = append(cands, offAfter)
	}
	var out []int
	for _, off := range cands {
		l := naive.Add(-time.Duration(off) * time.Second).In(loc)
		if l.Year() == y && l.Month() == mo && l.Day() == d && l.Hour() == h && l.Minute() == mi && l.Second() == s {
			if _, actual := l.Zone(); actual == off {
				out = append(out, off)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// OffsetAt is the offset, in seconds east, in force in loc at instant t.
func OffsetAt(loc *time.Location, t time.Time) int {
	_, off := t.In(loc).Zone()
	return off
}

// FormatOffset renders seconds east as ±hh:mm.
func FormatOffset(sec int) string {
	sign := '+'
	if sec < 0 {
		sign = '-'
		sec = -sec
	}
	return fmt.Sprintf("%c%02d:%02d", sign, sec/3600, (sec%3600)/60)
}
