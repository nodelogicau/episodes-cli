package model

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Mode selects which side of the format a check speaks for.
type Mode int

const (
	// ForValidate reports as the validator does: it warns where clocks and
	// copies can disagree, and never fails a file for a rel a reader ignores.
	ForValidate Mode = iota
	// ForWrite holds a writer to every MUST and MUST NOT: anything a
	// validator would warn on that a writer can avoid is an error.
	ForWrite
)

// Check applies every rule that one episode can be checked against alone.
// Rules that need the workspace (containers, links, acts, duplicates, paths,
// the index) are the query package's.
func Check(e *Episode, mode Mode) Problems {
	c := &checker{mode: mode}
	c.identity(e)
	c.occurred(e)
	c.references(e)
	c.containment(e)
	c.links(e)
	c.content(e)
	c.source(e)
	c.sourceStatus(e)
	c.redacted(e)
	return c.out
}

type checker struct {
	mode Mode
	out  Problems
}

func (c *checker) add(sev Severity, code, field, format string, args ...any) {
	c.out = append(c.out, Problem{Severity: sev, Code: code, Field: field, Message: fmt.Sprintf(format, args...)})
}

func (c *checker) err(code, field, format string, args ...any) {
	c.add(SevError, code, field, format, args...)
}

// soft is a warning when validating and an error when writing.
func (c *checker) soft(code, field, format string, args ...any) {
	sev := SevWarning
	if c.mode == ForWrite {
		sev = SevError
	}
	c.add(sev, code, field, format, args...)
}

func (c *checker) missing(field string) {
	c.err("missing_field", field, "%s is required", field)
}

func (c *checker) identity(e *Episode) {
	switch {
	case e.ID == "":
		c.missing("id")
	case !IsID(e.ID):
		c.err("invalid_id", "id", "id %q does not match ^epi_[A-Za-z0-9-]+$", e.ID)
	case !IsCanonicalID(e.ID):
		sev := SevInfo
		if c.mode == ForWrite {
			sev = SevError
		}
		c.add(sev, "id_not_uuidv7", "id", "id %q is not epi_ followed by a lowercase UUIDv7", e.ID)
	}
	switch {
	case e.Type == "":
		c.missing("type")
	case e.Type != TypeEpisode:
		c.err("invalid_type", "type", "type must be %q, not %q", TypeEpisode, e.Type)
	}
	switch {
	case e.Kind == "":
		c.missing("kind")
	case !kindPattern.MatchString(e.Kind):
		c.err("invalid_kind", "kind", "kind %q must be a short lowercase word matching ^[a-z][a-z0-9-]*$ (for example call, voicememo)", e.Kind)
	}
}

func (c *checker) occurred(e *Episode) {
	o := e.Occurred
	if o.Start == "" {
		c.missing("occurred.start")
	}
	switch {
	case o.Basis == "":
		c.missing("occurred.basis")
	case o.Basis == "planned":
		c.err("invalid_basis", "occurred.basis", "basis %q is not allowed: occurred comes from the happening, never from a plan, schedule or time set aside; use recorded, stated or inferred", o.Basis)
	case !slices.Contains(Bases, o.Basis):
		c.err("invalid_basis", "occurred.basis", "basis %q must be one of %s", o.Basis, strings.Join(Bases, ", "))
	}
	var start, end *temporal.Value
	for _, b := range []struct {
		field, raw string
		dst        **temporal.Value
	}{{"occurred.start", o.Start, &start}, {"occurred.end", o.End, &end}} {
		if b.raw == "" {
			continue
		}
		v, err := temporal.Parse(b.raw)
		if err != nil {
			c.err("invalid_occurred", b.field, "%v", err)
			continue
		}
		*b.dst = &v
		if err := v.ReadProblem(); err != nil {
			c.err("critical_suffix", b.field, "%v", err)
			continue
		}
		if c.mode == ForWrite {
			if err := v.WriteProblem(); err != nil {
				c.err("suffix_refused", b.field, "%v", err)
				continue
			}
		} else if v.Zone != "" {
			if _, err := temporal.LoadZone(v.Zone); err != nil {
				c.add(SevWarning, "zone_unknown", b.field, "%s: zone suffix %v; the instant is taken from the offset", b.raw, err)
				continue
			}
		}
		if err := v.ZoneAgreement(); err != nil {
			c.soft("zone_offset_mismatch", b.field, "%v; the offset governs", err)
		}
	}
	if start != nil && end != nil {
		if !temporal.SameForm(*start, *end) {
			c.err("occurred_mixed_forms", "occurred.end", "start and end must take the same form, both dates or both date-times (start %q, end %q)", o.Start, o.End)
		} else {
			tz := tzLoc(o.TZ)
			if end.End(tz).Before(start.Start(tz)) || (!end.DateOnly && end.Instant.Before(start.Instant)) {
				c.soft("occurred_end_before_start", "occurred.end", "end %q is before start %q", o.End, o.Start)
			}
		}
	}
	if o.TZ != "" {
		if start != nil && !start.DateOnly {
			c.err("tz_beside_datetime", "occurred.tz", "tz must not appear beside a date-time, whose zone is its suffix; write [%s] after the offset instead", o.TZ)
		} else if _, err := temporal.LoadZone(o.TZ); err != nil {
			if c.mode == ForWrite {
				c.err("invalid_tz", "occurred.tz", "tz %v", err)
			} else {
				c.add(SevWarning, "zone_unknown", "occurred.tz", "tz %v", err)
			}
		}
	}
	// Nothing is recorded before it happens.
	if e.Timestamp == "" {
		return
	}
	rec, err := temporal.ParseTimestamp(e.Timestamp)
	if err != nil {
		return
	}
	for _, b := range []struct {
		field string
		v     *temporal.Value
	}{{"occurred.start", start}, {"occurred.end", end}} {
		if b.v == nil {
			continue
		}
		if after, by := temporal.AfterRecording(*b.v, o.TZ, rec); after {
			hint := ""
			if b.field == "occurred.end" {
				hint = "; record it with its start and no end, or once it is over"
			}
			c.soft("occurred_after_recording", b.field, "%s %q is later than the recording instant %s by %s: nothing is recorded before it happens%s", b.field, b.v.Raw, e.Timestamp, by, hint)
		}
	}
}

func tzLoc(name string) *time.Location {
	if name == "" {
		return nil
	}
	loc, err := temporal.LoadZone(name)
	if err != nil {
		return nil
	}
	return loc
}

func (c *checker) references(e *Episode) {
	if e.Standpoint == "" {
		c.missing("standpoint")
	}
	if e.Has("participants") && len(e.Participants) == 0 && c.mode == ForValidate {
		c.add(SevInfo, "empty_participants", "participants", "participants is present and empty")
	}
	for _, p := range e.Participants {
		if strings.TrimSpace(p) == "" {
			c.err("empty_reference", "participants", "a participant must be a non-empty reference")
		}
	}
	if e.Has("place") && strings.TrimSpace(e.Place) == "" {
		c.err("empty_reference", "place", "place must be a non-empty reference")
	}
	if e.Has("label") && e.Label == "" && !e.Redacted.Lists("label") {
		c.err("empty_label", "label", "label must not be empty")
	}
}

func (c *checker) containment(e *Episode) {
	if e.Container != "" && !IsID(e.Container) {
		c.err("invalid_container", "container", "container %q is not an episode id", e.Container)
	}
	if e.Container != "" && e.Container == e.ID {
		c.err("invalid_container", "container", "an episode cannot contain itself")
	}
	if e.Ordinal != nil && *e.Ordinal < 1 {
		c.err("invalid_ordinal", "ordinal", "ordinal must be an integer from 1, not %d", *e.Ordinal)
	}
	if e.Ordinal != nil && e.Container == "" {
		c.soft("ordinal_without_container", "ordinal", "ordinal is a position within container, and there is no container")
	}
}

func (c *checker) links(e *Episode) {
	for i, l := range e.Links {
		field := fmt.Sprintf("links[%d]", i)
		if len(l.Extra) > 0 || l.Rel == "" || l.Href == "" {
			c.err("link_shape", field, "a link must have exactly rel and href (has %s)", linkKeys(l))
			if l.Rel == "" || l.Href == "" {
				continue
			}
		}
		if err := RelProblem(l.Rel); err != nil {
			c.err("link_rel_invalid", field+".rel", "%v", err)
		}
		if !IsID(l.Href) && !IsAbsoluteURI(l.Href) {
			c.err("link_href_invalid", field+".href", "href %q is neither an episode id nor an absolute URI", l.Href)
		}
		if l.Href == e.ID {
			c.err("link_href_invalid", field+".href", "an episode cannot link to itself")
		}
		if (l.Rel == "up" || l.Rel == "collection") && e.Container != "" && l.Href == e.Container {
			c.soft("link_restates_container", field, "a %s link to the episode's own container restates containment; container and ordinal already say it", l.Rel)
		}
	}
}

func linkKeys(l Link) string {
	var ks []string
	if l.Rel != "" {
		ks = append(ks, "rel")
	}
	if l.Href != "" {
		ks = append(ks, "href")
	}
	ks = append(ks, l.Extra.Keys()...)
	if len(ks) == 0 {
		return "no keys"
	}
	return strings.Join(ks, ", ")
}

func (c *checker) content(e *Episode) {
	ct := e.Content
	if !e.Has("content") && e.present != nil {
		c.missing("content")
		return
	}
	switch ct.Mode {
	case "":
		c.missing("content.mode")
		return
	case ModeInline:
		if ct.Text == nil && !e.Redacted.Lists("content.text") {
			c.err("inline_without_text", "content.text", "an inline episode must carry text, unless redacted.fields lists content.text")
		}
		if ct.UID != "" {
			c.err("uid_on_inline", "content.uid", "an inline episode must not carry uid: its identity is its episode id")
		}
		if ct.Ref != "" {
			c.err("ref_on_inline", "content.ref", "an inline episode must not carry ref")
		}
		if ct.Derivation == "" {
			c.missing("content.derivation")
		} else if !slices.Contains(Derivations, ct.Derivation) {
			c.err("invalid_derivation", "content.derivation", "derivation %q must be one of %s", ct.Derivation, strings.Join(Derivations, ", "))
		}
		if ct.MediaType == "" {
			c.missing("content.media_type")
		}
	case ModeRef:
		if ct.Ref == "" && !e.Redacted.Lists("content.ref") {
			c.err("ref_without_ref", "content.ref", "a reference episode must carry ref, unless redacted.fields lists content.ref")
		}
		if ct.Ref != "" && IsID(ct.Ref) {
			c.err("ref_is_episode_id", "content.ref", "ref %q is an episode id; what a reference episode points at lives outside the workspace, and a relation between episodes is a link (--link via=%s)", ct.Ref, ct.Ref)
		}
		if ct.Text != nil {
			c.err("text_on_ref", "content.text", "a reference episode must not carry text: it is a trace of the content, not a copy")
		}
		if ct.UID != "" && ct.MediaType == "" {
			c.err("uid_without_media_type", "content.media_type", "media_type is required with uid: it says which format's identifier uid is")
		}
		if ct.Derivation != "" {
			c.err("derivation_on_ref", "content.derivation", "derivation says how inline text came to exist; a reference episode carries none")
		}
		if ct.Hash != "" {
			p := ParseHash(ct.Hash)
			switch {
			case p.Malformed:
				c.err("hash_invalid", "content.hash", "hash %q must be <algorithm>:<digest>", ct.Hash)
			case !p.Implemented:
				if c.mode == ForWrite {
					c.err("hash_invalid", "content.hash", "%v", HashWriteProblem(ct.Hash))
				} else {
					c.add(SevInfo, "hash_unverified", "content.hash", "hash algorithm %q is not one this implementation computes; the hash is unverified", p.Algorithm)
				}
			case c.mode == ForWrite:
				if err := HashWriteProblem(ct.Hash); err != nil {
					c.err("hash_invalid", "content.hash", "%v", err)
				}
			}
		}
	case ModeNone:
		var extra []string
		for _, f := range []struct{ name, v string }{
			{"ref", ct.Ref}, {"uid", ct.UID}, {"version", ct.Version}, {"hash", ct.Hash},
			{"media_type", ct.MediaType}, {"derivation", ct.Derivation},
		} {
			if f.v != "" {
				extra = append(extra, f.name)
			}
		}
		if ct.Text != nil {
			extra = append(extra, "text")
		}
		if ct.Language != "" {
			c.err("language_on_none", "content.language", "content with mode none must not carry language")
		}
		if len(extra) > 0 {
			c.err("none_with_content", "content", "content with mode none carries nothing else (has %s)", strings.Join(extra, ", "))
		}
		return
	default:
		c.err("invalid_mode", "content.mode", "mode %q must be one of %s", ct.Mode, strings.Join(Modes, ", "))
		return
	}
	if ct.Language != "" {
		if err := LanguageProblem(ct.Language); err != nil {
			code := "language_invalid"
			if strings.EqualFold(ct.Language, "und") {
				code = "language_und"
			}
			c.err(code, "content.language", "%v", err)
		} else if c.mode == ForWrite && ct.Language != CanonicalLanguageCase(ct.Language) {
			c.err("language_case", "content.language", "language %q must be written %q", ct.Language, CanonicalLanguageCase(ct.Language))
		}
	}
}

func (c *checker) source(e *Episode) {
	if e.Source.Author == "" && e.Source.Harness == "" {
		c.err("source_missing", "source", "source must contain at least one of author or harness")
	}
	c.timestamp("timestamp", e.Timestamp, true)
}

func (c *checker) timestamp(field, v string, required bool) {
	if v == "" {
		if required {
			c.missing(field)
		}
		return
	}
	if _, err := temporal.ParseTimestamp(v); err != nil {
		c.err("invalid_timestamp", field, "%s %q is not an RFC 3339 date-time", field, v)
		return
	}
	if c.mode == ForWrite && !temporal.IsCanonicalTimestamp(v) {
		c.err("invalid_timestamp", field, "%s %q must be UTC with Z at seconds precision", field, v)
	}
}

func (c *checker) sourceStatus(e *Episode) {
	ss := e.SourceStatus
	if ss == nil {
		return
	}
	if e.Content.Mode != ModeRef {
		c.err("source_status_not_ref", "source_status", "source_status is an observation of referenced content; this episode has mode %q", e.Content.Mode)
	}
	switch {
	case ss.State == "":
		c.missing("source_status.state")
	case !slices.Contains(States, ss.State):
		c.err("invalid_state", "source_status.state", "state %q must be one of %s", ss.State, strings.Join(States, ", "))
	}
	c.timestamp("source_status.verified", ss.Verified, true)
	c.timestamp("source_status.detected", ss.Detected, true)
}

func (c *checker) redacted(e *Episode) {
	r := e.Redacted
	if r == nil {
		return
	}
	c.timestamp("redacted.timestamp", r.Timestamp, true)
	if r.Reason == "" {
		c.missing("redacted.reason")
	}
	if r.Source.Author == "" && r.Source.Harness == "" {
		c.err("source_missing", "redacted.source", "redacted.source must contain at least one of author or harness")
	}
	if len(r.Fields) == 0 {
		c.missing("redacted.fields")
	}
	for _, f := range r.Fields {
		if !IsRedactable(f) {
			c.err("redaction_path_invalid", "redacted.fields", "%q cannot be redacted; the closed set is %s", f, strings.Join(Redactable, ", "))
			continue
		}
		if e.CarriesPath(f) {
			c.err("redaction_field_present", f, "redacted.fields lists %s, and the file still carries it", f)
		}
	}
}
