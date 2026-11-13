// Package temporal holds the format's time grammar, as pure functions: RFC
// 9557 values for occurred.start and occurred.end, a date alone with its
// optional tz, offset–zone agreement, the recording-instant comparison that
// enforces "nothing is recorded before it happens", and the on-disk forms.
package temporal

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Suffix is one bracketed RFC 9557 suffix as read. A time-zone suffix has an
// empty Key and its name in Value; a tagged suffix (u-ca=gregory) has both.
type Suffix struct {
	Critical bool
	Key      string
	Value    string
}

func (s Suffix) String() string {
	b := "["
	if s.Critical {
		b += "!"
	}
	if s.Key != "" {
		b += s.Key + "="
	}
	return b + s.Value + "]"
}

// Value is one occurred.start or occurred.end: a date-time with an offset and
// optional suffixes, or a date alone.
type Value struct {
	Raw      string
	DateOnly bool
	// Year, Month and Day are the date as written (for a date-time, the local
	// date the offset gives).
	Year  int
	Month time.Month
	Day   int
	// Hour, Minute and Second are the local wall-clock time as written.
	Hour, Minute, Second int
	// Instant is the instant the offset fixes, in a fixed zone carrying that
	// offset. Zero for a date alone.
	Instant time.Time
	// Offset is the offset in seconds east; UTC is true when it was written Z.
	Offset int
	UTC    bool
	// Zone is the time-zone suffix's name, when there is one.
	Zone          string
	ZoneCritical  bool
	Suffixes      []Suffix
	hasZoneSuffix bool
}

var (
	dateRe     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	dateTimeRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})((?:\[[^\[\]]*\])*)$`)
	suffixRe   = regexp.MustCompile(`\[([^\[\]]*)\]`)
	tagKeyRe   = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)
	zoneNameRe = regexp.MustCompile(`^(?:[A-Za-z._][A-Za-z0-9._+-]*(?:/[A-Za-z._][A-Za-z0-9._+-]*)*|[+-]\d{2}:\d{2})$`)
)

// Parse reads one occurred bound. It checks syntax only: suffix policy is
// WriteProblem's and ReadProblem's, and zone agreement is ZoneAgreement's.
func Parse(s string) (Value, error) {
	if m := dateRe.FindStringSubmatch(s); m != nil {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return Value{}, fmt.Errorf("%q is not a calendar date", s)
		}
		return Value{Raw: s, DateOnly: true, Year: t.Year(), Month: t.Month(), Day: t.Day()}, nil
	}
	m := dateTimeRe.FindStringSubmatch(s)
	if m == nil {
		return Value{}, fmt.Errorf("%q is neither an RFC 3339 date-time with an offset (2026-11-07T14:24:00+11:00), optionally followed by an IANA zone in brackets, nor a date (2026-11-07)", s)
	}
	base := s[:len(s)-len(m[9])]
	t, err := time.Parse(time.RFC3339Nano, base)
	if err != nil {
		return Value{}, fmt.Errorf("%q is not a valid date-time: %v", s, err)
	}
	v := Value{Raw: s, Instant: t, UTC: m[8] == "Z"}
	v.Year, _ = strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	v.Month = time.Month(mo)
	v.Day, _ = strconv.Atoi(m[3])
	v.Hour, _ = strconv.Atoi(m[4])
	v.Minute, _ = strconv.Atoi(m[5])
	v.Second, _ = strconv.Atoi(m[6])
	_, v.Offset = t.Zone()
	for i, sm := range suffixRe.FindAllStringSubmatch(m[9], -1) {
		body := sm[1]
		suf := Suffix{}
		if strings.HasPrefix(body, "!") {
			suf.Critical = true
			body = body[1:]
		}
		if k, val, ok := strings.Cut(body, "="); ok {
			if !tagKeyRe.MatchString(k) || val == "" {
				return Value{}, fmt.Errorf("%q has a malformed suffix [%s]", s, sm[1])
			}
			suf.Key, suf.Value = k, val
		} else {
			if !zoneNameRe.MatchString(body) {
				return Value{}, fmt.Errorf("%q has a malformed time-zone suffix [%s]", s, sm[1])
			}
			// RFC 9557: the time-zone suffix, when present, comes first and
			// appears once.
			if v.hasZoneSuffix || i != 0 {
				return Value{}, fmt.Errorf("%q has a second or misplaced time-zone suffix [%s]; only one, first, is allowed", s, sm[1])
			}
			suf.Value = body
			v.hasZoneSuffix = true
			v.Zone = body
			v.ZoneCritical = suf.Critical
		}
		v.Suffixes = append(v.Suffixes, suf)
	}
	return v, nil
}

// Critical returns the suffixes carrying the critical flag.
func (v Value) Critical() []Suffix {
	var out []Suffix
	for _, s := range v.Suffixes {
		if s.Critical {
			out = append(out, s)
		}
	}
	return out
}

// Elective returns the tagged suffixes without the critical flag: those a
// reader ignores.
func (v Value) Elective() []Suffix {
	var out []Suffix
	for _, s := range v.Suffixes {
		if !s.Critical && s.Key != "" {
			out = append(out, s)
		}
	}
	return out
}

// ReadProblem is why a reader must treat the value as invalid: a critical
// suffix. This implementation honours no critical suffix, the zone included,
// because a writer never writes one.
func (v Value) ReadProblem() error {
	if c := v.Critical(); len(c) > 0 {
		return fmt.Errorf("%q carries the critical suffix %s, which this reader cannot honour", v.Raw, c[0])
	}
	return nil
}

// WriteProblem is why a writer must refuse the value: a critical flag, a
// suffix other than one IANA zone, or a zone this binary does not know.
func (v Value) WriteProblem() error {
	if c := v.Critical(); len(c) > 0 {
		return fmt.Errorf("%q carries the critical flag (%s); writers must not write it", v.Raw, c[0])
	}
	for _, s := range v.Suffixes {
		if s.Key != "" {
			return fmt.Errorf("%q carries the suffix %s; the only suffix a writer writes is one IANA time zone, so give the value without it", v.Raw, s)
		}
	}
	if v.Zone != "" {
		if _, err := LoadZone(v.Zone); err != nil {
			return fmt.Errorf("%q: zone suffix %v", v.Raw, err)
		}
	}
	return nil
}

// ZoneAgreement checks a date-time's offset against its zone's rules at the
// local time written. It returns nil when there is no zone, the value is in
// UTC (Z fixes the instant and says nothing about the local offset), or the
// offset is one the local time can carry. Otherwise it names the offsets the
// zone expects, or says the local time does not exist there.
func (v Value) ZoneAgreement() error {
	if v.DateOnly || v.Zone == "" || v.UTC {
		return nil
	}
	loc, err := LoadZone(v.Zone)
	if err != nil {
		return err
	}
	offs := OffsetsAt(loc, v.Year, v.Month, v.Day, v.Hour, v.Minute, v.Second)
	for _, o := range offs {
		if o == v.Offset {
			return nil
		}
	}
	if len(offs) == 0 {
		return fmt.Errorf("%q: %02d:%02d:%02d on %04d-%02d-%02d does not exist in %s (clocks went forward), and %s fixes an instant the zone shows as %s",
			v.Raw, v.Hour, v.Minute, v.Second, v.Year, v.Month, v.Day, v.Zone, FormatOffset(v.Offset), v.Instant.In(loc).Format("15:04:05-07:00"))
	}
	exp := make([]string, len(offs))
	for i, o := range offs {
		exp[i] = FormatOffset(o)
	}
	return fmt.Errorf("%q: the offset %s disagrees with %s, whose offset at that local time is %s", v.Raw, FormatOffset(v.Offset), v.Zone, strings.Join(exp, " or "))
}

// Date returns the date part as a UTC midnight, for date arithmetic.
func (v Value) Date() time.Time { return time.Date(v.Year, v.Month, v.Day, 0, 0, 0, 0, time.UTC) }

// Start returns the instant a bound begins: the instant for a date-time, and
// midnight of the date in loc (UTC when nil) for a date alone.
func (v Value) Start(loc *time.Location) time.Time {
	if !v.DateOnly {
		return v.Instant
	}
	if loc == nil {
		loc = time.UTC
	}
	return time.Date(v.Year, v.Month, v.Day, 0, 0, 0, 0, loc)
}

// End returns the instant a bound ends: the instant for a date-time, and the
// next midnight for a date alone.
func (v Value) End(loc *time.Location) time.Time {
	if !v.DateOnly {
		return v.Instant
	}
	if loc == nil {
		loc = time.UTC
	}
	return time.Date(v.Year, v.Month, v.Day+1, 0, 0, 0, 0, loc)
}

// AfterRecording reports whether a bound lies later than the recording
// instant rec, as the format compares them: a date-time as the instant its
// offset fixes; a date alone against rec's date in tz when tz is given, and
// otherwise against rec's UTC date allowing one day. The string says by how
// much, for a message.
func AfterRecording(v Value, tz string, rec time.Time) (bool, string) {
	if !v.DateOnly {
		if d := v.Instant.Sub(rec); d > 0 {
			return true, HumanDuration(d)
		}
		return false, ""
	}
	if tz != "" {
		if loc, err := LoadZone(tz); err == nil {
			r := rec.In(loc)
			recDate := time.Date(r.Year(), r.Month(), r.Day(), 0, 0, 0, 0, time.UTC)
			if days := int(v.Date().Sub(recDate).Hours() / 24); days > 0 {
				return true, fmt.Sprintf("%d day(s) after the recording date %s in %s", days, recDate.Format("2006-01-02"), tz)
			}
			return false, ""
		}
	}
	r := rec.UTC()
	recDate := time.Date(r.Year(), r.Month(), r.Day(), 0, 0, 0, 0, time.UTC)
	if days := int(v.Date().Sub(recDate).Hours() / 24); days > 1 {
		return true, fmt.Sprintf("%d day(s) after the recording date %s in UTC, beyond the one day allowed for an unknown zone", days, recDate.Format("2006-01-02"))
	}
	return false, ""
}

// HumanDuration renders a positive duration in whole units for messages.
func HumanDuration(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d hours %d minutes", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}

// SameForm reports whether start and end are both dates or both date-times.
func SameForm(start, end Value) bool { return start.DateOnly == end.DateOnly }

// FormatDateTime renders t, at seconds precision, with its offset, followed
// by [zone] when zone is given. An offset of zero is written Z only when no
// zone is written, since Z says nothing of the local offset.
func FormatDateTime(t time.Time, zone string) string {
	t = t.Truncate(time.Second)
	if zone == "" {
		return t.Format(time.RFC3339)
	}
	return t.Format("2006-01-02T15:04:05-07:00") + "[" + zone + "]"
}

// RecordedNow is the occurred value a verb writes when it supplies the time
// itself. With a configured zone, the instant is shown in that zone, so the
// offset and the zone agree by construction; without one, the instant is
// shown at the process's local offset and no zone is written, because a zone
// is never inferred from the host.
func RecordedNow(now time.Time, zone string) (string, error) {
	if zone == "" {
		return FormatDateTime(now.In(time.Local), ""), nil
	}
	loc, err := LoadZone(zone)
	if err != nil {
		return "", err
	}
	return FormatDateTime(now.In(loc), zone), nil
}

// TimeLayout is the on-disk recording-instant form: RFC 3339, UTC, seconds.
const TimeLayout = "2006-01-02T15:04:05Z"

// FormatTimestamp renders a recording instant in the on-disk form.
func FormatTimestamp(t time.Time) string { return t.UTC().Format(TimeLayout) }

// ParseTimestamp reads an RFC 3339 date-time (any offset, optional fraction)
// and returns it in UTC at seconds precision. A zone suffix is refused: a
// recording instant is UTC.
func ParseTimestamp(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not an RFC 3339 date-time", s)
	}
	return t.UTC().Truncate(time.Second), nil
}

// IsCanonicalTimestamp reports whether s is in the on-disk form.
func IsCanonicalTimestamp(s string) bool {
	t, err := time.Parse(TimeLayout, s)
	return err == nil && t.Format(TimeLayout) == s
}
