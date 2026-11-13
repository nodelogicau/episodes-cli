// Package model is the format layer: the EPISODE object and its blocks, ids,
// canonical order, the YAML codec, the field rules shared by writers and the
// validator, the content hash, mid: conversion, BCP 47 well-formedness, and
// the link-relation registry snapshot. All knowledge of episodes/0.1 lives
// here and in temporal, so a change upstream is a change to these packages.
package model

import "gopkg.in/yaml.v3"

// Format is the format version this implementation reads and writes.
const Format = "episodes/0.1"

// TypeEpisode is the only object type.
const TypeEpisode = "episode"

// Field is one key a block carries that this format does not define, kept in
// the order read and re-emitted unchanged. It is given no meaning.
type Field struct {
	Key   string
	Value *yaml.Node
}

// Extras are the undefined keys of one mapping.
type Extras []Field

// Keys lists the extra keys in order.
func (x Extras) Keys() []string {
	out := make([]string, len(x))
	for i, f := range x {
		out[i] = f.Key
	}
	return out
}

// Episode is one EPISODE file.
//
// Canonical order: id, type, kind, occurred, standpoint, participants, place,
// label, container, ordinal, links, content, source, timestamp, then any
// extra keys in the order read, then source_status, then redacted.
type Episode struct {
	ID           string
	Type         string
	Kind         string
	Occurred     Occurred
	Standpoint   string
	Participants []string
	Place        string
	Label        string
	Container    string
	Ordinal      *int
	Links        []Link
	Content      Content
	Source       Source
	Timestamp    string
	SourceStatus *SourceStatus
	Redacted     *Redacted
	Extra        Extras

	// present records which top-level keys the file carried, so a reader can
	// tell a missing field from an empty one.
	present map[string]bool
}

// Has reports whether the file carried the top-level key k. An episode built
// in memory reports every non-zero field.
func (e *Episode) Has(k string) bool {
	if e.present != nil {
		return e.present[k]
	}
	return false
}

// Occurred is when the episode happened.
//
// Canonical order: start, end, tz, basis.
type Occurred struct {
	Start string
	End   string
	TZ    string
	Basis string
	Extra Extras
}

// Bases of an occurred time.
const (
	BasisRecorded = "recorded"
	BasisStated   = "stated"
	BasisInferred = "inferred"
)

// Bases lists the valid bases.
var Bases = []string{BasisRecorded, BasisStated, BasisInferred}

// Content is where the record lives.
//
// Canonical order: mode, ref, uid, version, hash, media_type, derivation,
// language, text.
type Content struct {
	Mode       string
	Ref        string
	UID        string
	Version    string
	Hash       string
	MediaType  string
	Derivation string
	Language   string
	// Text is nil when the file carries no text key.
	Text  *string
	Extra Extras
}

// Content modes.
const (
	ModeInline = "inline"
	ModeRef    = "ref"
	ModeNone   = "none"
)

// Modes lists the valid content modes.
var Modes = []string{ModeInline, ModeRef, ModeNone}

// Derivations of inline text.
const (
	DerivationVerbatim    = "verbatim"
	DerivationTranscribed = "transcribed"
	DerivationOCR         = "ocr"
	DerivationRendered    = "rendered"
)

// Derivations lists the valid derivations.
var Derivations = []string{DerivationVerbatim, DerivationTranscribed, DerivationOCR, DerivationRendered}

// DefaultMediaType is the inline media type when none is given.
const DefaultMediaType = "text/markdown"

// Link is one RFC 8288 typed link.
//
// Canonical order: rel, href.
type Link struct {
	Rel   string
	Href  string
	Extra Extras
}

// Source is who recorded the episode.
//
// Canonical order: author, harness, model.
type Source struct {
	Author  string
	Harness string
	Model   string
	Extra   Extras
}

// IsZero reports whether no part of the source is set.
func (s Source) IsZero() bool {
	return s.Author == "" && s.Harness == "" && s.Model == "" && len(s.Extra) == 0
}

// SourceStatus is the last observation of a reference episode's source.
//
// Canonical order: state, verified, detected.
type SourceStatus struct {
	State    string
	Verified string
	Detected string
	Extra    Extras
}

// Source states.
const (
	StateAvailable   = "available"
	StateChanged     = "changed"
	StateUnreachable = "unreachable"
	StateGone        = "gone"
)

// States lists the valid source states.
var States = []string{StateAvailable, StateChanged, StateUnreachable, StateGone}

// Redacted is the redaction block, always written last.
//
// Canonical order: timestamp, reason, source, fields.
type Redacted struct {
	Timestamp string
	Reason    string
	Source    Source
	Fields    []string
	Extra     Extras
}

// Lists returns whether the block lists path.
func (r *Redacted) Lists(path string) bool {
	if r == nil {
		return false
	}
	for _, f := range r.Fields {
		if f == path {
			return true
		}
	}
	return false
}

// Redactable is the closed set of paths a redaction may list, sorted.
var Redactable = []string{
	"content.hash", "content.language", "content.ref", "content.text",
	"content.uid", "content.version", "label", "links", "participants", "place",
}

// IsRedactable reports whether path is in the closed set.
func IsRedactable(path string) bool {
	for _, p := range Redactable {
		if p == path {
			return true
		}
	}
	return false
}

// Withdrawn reports whether the episode is an inline one whose text is
// redacted: it carries no record and is out of recall.
func (e *Episode) Withdrawn() bool {
	return e.Content.Mode == ModeInline && e.Redacted.Lists("content.text")
}

// Tombstone reports whether the episode is a reference one whose ref is
// redacted: it says that something happened, and no longer where.
func (e *Episode) Tombstone() bool {
	return e.Content.Mode == ModeRef && e.Redacted.Lists("content.ref")
}

// Clone returns a deep copy, so an amendment can be compared with the file
// as read. Extra nodes are shared: nothing mutates them.
func (e *Episode) Clone() *Episode {
	c := *e
	c.Participants = append([]string(nil), e.Participants...)
	c.Links = append([]Link(nil), e.Links...)
	if e.Ordinal != nil {
		o := *e.Ordinal
		c.Ordinal = &o
	}
	if e.Content.Text != nil {
		t := *e.Content.Text
		c.Content.Text = &t
	}
	if e.SourceStatus != nil {
		s := *e.SourceStatus
		c.SourceStatus = &s
	}
	if e.Redacted != nil {
		r := *e.Redacted
		r.Fields = append([]string(nil), e.Redacted.Fields...)
		c.Redacted = &r
	}
	if e.present != nil {
		c.present = map[string]bool{}
		for k, v := range e.present {
			c.present[k] = v
		}
	}
	return &c
}

// RemovePath removes one redactable path from the episode. It reports
// whether the episode carried it.
func (e *Episode) RemovePath(path string) bool {
	had := false
	switch path {
	case "label":
		had, e.Label = e.Label != "", ""
	case "participants":
		had, e.Participants = len(e.Participants) > 0, nil
	case "place":
		had, e.Place = e.Place != "", ""
	case "links":
		had, e.Links = len(e.Links) > 0, nil
	case "content.text":
		had, e.Content.Text = e.Content.Text != nil, nil
	case "content.ref":
		had, e.Content.Ref = e.Content.Ref != "", ""
	case "content.uid":
		had, e.Content.UID = e.Content.UID != "", ""
	case "content.version":
		had, e.Content.Version = e.Content.Version != "", ""
	case "content.hash":
		had, e.Content.Hash = e.Content.Hash != "", ""
	case "content.language":
		had, e.Content.Language = e.Content.Language != "", ""
	}
	return had
}

// CarriesPath reports whether the episode carries a redactable path.
func (e *Episode) CarriesPath(path string) bool {
	return e.Clone().RemovePath(path)
}
