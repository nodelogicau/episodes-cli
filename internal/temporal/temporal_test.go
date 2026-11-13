package temporal

import (
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) Value {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return v
}

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseForms(t *testing.T) {
	cases := []struct {
		in       string
		ok       bool
		dateOnly bool
		zone     string
	}{
		{"2026-11-07T14:24:00+11:00[Australia/Melbourne]", true, false, "Australia/Melbourne"},
		{"2026-11-07T14:24:00+11:00", true, false, ""},
		{"2026-11-07T03:24:00Z", true, false, ""},
		{"2026-11-07T14:24:00.5+11:00", true, false, ""},
		{"2026-11-03", true, true, ""},
		{"2026-11-07T14:24:00+11:00[Australia/Melbourne][u-ca=gregory]", true, false, "Australia/Melbourne"},
		{"2026-11-07T14:24:00+11:00[u-ca=gregory]", true, false, ""},
		{"2026-11-07T14:24:00+11:00[!Australia/Melbourne]", true, false, "Australia/Melbourne"},
		{"2026-11-07T14:24:00", false, false, ""},                                    // no offset
		{"2026-11-07 14:24:00+11:00", false, false, ""},                              // space
		{"2026-02-30", false, true, ""},                                              // not a date
		{"2026-11-07T14:24:00+11:00[A][B]", false, false, ""},                        // two zones
		{"2026-11-07T14:24:00+11:00[u-ca=x][Australia/Melbourne]", false, false, ""}, // zone not first
		{"next tuesday", false, false, ""},
	}
	for _, c := range cases {
		v, err := Parse(c.in)
		if (err == nil) != c.ok {
			t.Errorf("Parse(%q) err=%v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if !c.ok {
			continue
		}
		if v.DateOnly != c.dateOnly || v.Zone != c.zone {
			t.Errorf("Parse(%q) = dateOnly %v zone %q", c.in, v.DateOnly, v.Zone)
		}
	}
}

// occurred-time (upstream): a mail header with an offset only.
func TestMailDateOffsetOnly(t *testing.T) {
	v := mustParse(t, "2026-11-07T14:24:00+11:00")
	if v.Zone != "" || v.WriteProblem() != nil || v.ZoneAgreement() != nil {
		t.Fatalf("offset-only value should be valid with no zone: %+v", v)
	}
	if got := v.Instant.UTC().Format(time.RFC3339); got != "2026-11-07T03:24:00Z" {
		t.Fatalf("instant %s", got)
	}
}

// occurred-time: the repeated hour (Melbourne, 2026-04-05, clocks go back
// from +11:00 to +10:00 at 03:00).
func TestFallBackHourKeptApart(t *testing.T) {
	first := mustParse(t, "2026-04-05T02:30:00+11:00[Australia/Melbourne]")
	second := mustParse(t, "2026-04-05T02:30:00+10:00[Australia/Melbourne]")
	for _, v := range []Value{first, second} {
		if err := v.ZoneAgreement(); err != nil {
			t.Errorf("%s: %v", v.Raw, err)
		}
	}
	if d := second.Instant.Sub(first.Instant); d != time.Hour {
		t.Fatalf("the two 02:30s are %v apart, want 1h", d)
	}
	if first.Instant.UTC().Format(time.RFC3339) != "2026-04-04T15:30:00Z" || second.Instant.UTC().Format(time.RFC3339) != "2026-04-04T16:30:00Z" {
		t.Fatalf("instants %v %v", first.Instant.UTC(), second.Instant.UTC())
	}
}

// Melbourne springs forward at 02:00 on 2026-10-04: 02:30 does not exist.
func TestSpringForwardGap(t *testing.T) {
	for _, s := range []string{
		"2026-10-04T02:30:00+10:00[Australia/Melbourne]",
		"2026-10-04T02:30:00+11:00[Australia/Melbourne]",
	} {
		err := mustParse(t, s).ZoneAgreement()
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("%s: want gap error, got %v", s, err)
		}
	}
	if err := mustParse(t, "2026-10-04T03:30:00+11:00[Australia/Melbourne]").ZoneAgreement(); err != nil {
		t.Errorf("03:30+11:00 after the gap: %v", err)
	}
	if err := mustParse(t, "2026-10-04T01:30:00+10:00[Australia/Melbourne]").ZoneAgreement(); err != nil {
		t.Errorf("01:30+10:00 before the gap: %v", err)
	}
}

// occurred-time: wrong offset for the zone names the expected offset.
func TestWrongOffsetForZone(t *testing.T) {
	err := mustParse(t, "2026-11-07T14:24:00+10:00[Australia/Melbourne]").ZoneAgreement()
	if err == nil || !strings.Contains(err.Error(), "+11:00") {
		t.Fatalf("want mismatch naming +11:00, got %v", err)
	}
	// upstream: inconsistent offset and zone; readers take the instant from the offset.
	v := mustParse(t, "2026-01-15T10:00:00+10:00[Australia/Melbourne]")
	if v.ZoneAgreement() == nil {
		t.Fatal("want mismatch")
	}
	if v.Instant.UTC().Format(time.RFC3339) != "2026-01-15T00:00:00Z" {
		t.Fatalf("instant not from offset: %v", v.Instant.UTC())
	}
}

func TestSuffixPolicy(t *testing.T) {
	// Calendar suffix refused on write, ignored on read.
	cal := mustParse(t, "2026-11-07T14:24:00+11:00[u-ca=gregory]")
	if cal.WriteProblem() == nil {
		t.Error("u-ca suffix should be refused on write")
	}
	if cal.ReadProblem() != nil {
		t.Error("elective suffix should be ignored on read")
	}
	// upstream: an elective calendar tag after a zone.
	both := mustParse(t, "2026-11-07T14:24:00+11:00[Australia/Melbourne][u-ca=gregory]")
	if both.ReadProblem() != nil || both.Zone != "Australia/Melbourne" || both.ZoneAgreement() != nil {
		t.Errorf("reader should read instant and zone and ignore u-ca: %+v", both)
	}
	// Critical suffix invalid on read and refused on write.
	crit := mustParse(t, "2026-11-07T14:24:00+11:00[!Australia/Melbourne]")
	if crit.ReadProblem() == nil || crit.WriteProblem() == nil {
		t.Error("critical suffix should be invalid on read and refused on write")
	}
	if mustParse(t, "2026-11-07T14:24:00+11:00[Not/AZone]").WriteProblem() == nil {
		t.Error("an unknown zone should be refused on write")
	}
}

func TestSameForm(t *testing.T) {
	if SameForm(mustParse(t, "2026-11-03"), mustParse(t, "2026-11-03T10:00:00Z")) {
		t.Error("mixed forms reported same")
	}
	if !SameForm(mustParse(t, "2026-11-03"), mustParse(t, "2026-11-04")) {
		t.Error("two dates reported mixed")
	}
}

func TestAfterRecording(t *testing.T) {
	mel := "Australia/Melbourne"
	cases := []struct {
		name, v, tz, rec string
		after            bool
	}{
		// occurred-time: ninety minutes logged at the start (end 10:30 at 09:05).
		{"end ahead", "2026-11-08T10:30:00+11:00[Australia/Melbourne]", "", "2026-11-07T22:05:00Z", true},
		{"start before", "2026-11-08T09:00:00+11:00[Australia/Melbourne]", "", "2026-11-07T22:05:00Z", false},
		// upstream: recording a meeting before it starts.
		{"meeting 14:00 at 09:00", "2026-11-07T14:00:00+11:00", "", "2026-11-06T22:00:00Z", true},
		// upstream: an end in the future.
		{"end 11:00 at 10:00", "2026-11-07T11:00:00Z", "", "2026-11-07T10:00:00Z", true},
		// upstream: the repeated hour; second 02:30 is after a 15:45Z recording, the first is not.
		{"second 02:30", "2026-04-05T02:30:00+10:00[Australia/Melbourne]", "", "2026-04-04T15:45:00Z", true},
		{"first 02:30", "2026-04-05T02:30:00+11:00[Australia/Melbourne]", "", "2026-04-04T15:45:00Z", false},
		// upstream: a date alone with no zone, within one day of the UTC date.
		{"date no tz +1", "2026-11-08", "", "2026-11-07T22:00:00Z", false},
		{"date no tz +2", "2026-11-09", "", "2026-11-07T22:00:00Z", true},
		// A date in tz: 2026-11-08 is today in Melbourne at 22:00Z on the 7th.
		{"date tz today", "2026-11-08", mel, "2026-11-07T22:00:00Z", false},
		{"date tz tomorrow", "2026-11-09", mel, "2026-11-07T22:00:00Z", true},
		{"date tz ahead of utc", "2026-11-08", "UTC", "2026-11-07T22:00:00Z", true},
	}
	for _, c := range cases {
		got, why := AfterRecording(mustParse(t, c.v), c.tz, ts(c.rec))
		if got != c.after {
			t.Errorf("%s: AfterRecording = %v (%s), want %v", c.name, got, why, c.after)
		}
	}
	_, why := AfterRecording(mustParse(t, "2026-11-08T10:30:00+11:00"), "", ts("2026-11-07T22:05:00Z"))
	if why != "85 minutes" {
		t.Errorf("difference = %q, want 85 minutes", why)
	}
}

func TestFormat(t *testing.T) {
	mel, _ := LoadZone("Australia/Melbourne")
	at := ts("2026-11-07T03:24:00Z")
	if got := FormatDateTime(at.In(mel), "Australia/Melbourne"); got != "2026-11-07T14:24:00+11:00[Australia/Melbourne]" {
		t.Errorf("zoned: %s", got)
	}
	if got := FormatDateTime(at, ""); got != "2026-11-07T03:24:00Z" {
		t.Errorf("utc: %s", got)
	}
	if got, _ := RecordedNow(at, "Australia/Melbourne"); got != "2026-11-07T14:24:00+11:00[Australia/Melbourne]" {
		t.Errorf("recorded now with zone: %s", got)
	}
	// With no zone configured: the local offset and no suffix.
	saved := time.Local
	time.Local = time.FixedZone("x", 11*3600)
	defer func() { time.Local = saved }()
	if got, _ := RecordedNow(at, ""); got != "2026-11-07T14:24:00+11:00" {
		t.Errorf("recorded now without zone: %s", got)
	}
	if _, err := RecordedNow(at, "Local"); err == nil {
		t.Error("Local is not a zone name")
	}
}

func TestStartEnd(t *testing.T) {
	d := mustParse(t, "2026-11-03")
	mel, _ := LoadZone("Australia/Melbourne")
	if got := d.Start(mel).UTC().Format(time.RFC3339); got != "2026-11-02T13:00:00Z" {
		t.Errorf("start in tz: %s", got)
	}
	if got := d.End(nil).UTC().Format(time.RFC3339); got != "2026-11-04T00:00:00Z" {
		t.Errorf("end in utc: %s", got)
	}
}

func TestTimestamp(t *testing.T) {
	got, err := ParseTimestamp("2026-11-07T14:24:40.9+11:00")
	if err != nil || FormatTimestamp(got) != "2026-11-07T03:24:40Z" {
		t.Fatalf("%v %v", got, err)
	}
	if !IsCanonicalTimestamp("2026-11-07T03:24:40Z") || IsCanonicalTimestamp("2026-11-07T14:24:40+11:00") {
		t.Fatal("canonical check")
	}
}
