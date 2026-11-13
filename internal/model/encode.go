package model

import (
	"bytes"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Encode writes an episode in canonical form: canonical field order, extra
// keys after the specified ones and before source_status and redacted,
// redacted last, 2-space indentation, double-quoted occurred bounds, literal
// block scalars for multi-line text, participants sorted and de-duplicated,
// links sorted by rel then href with exact duplicates removed, absent fields
// omitted, and no document markers. Equal episodes encode to equal bytes.
func Encode(e *Episode) ([]byte, error) {
	m := newMap()
	m.str("id", e.ID)
	m.str("type", e.Type)
	m.str("kind", e.Kind)
	if o := OccurredNode(e.Occurred, false); o != nil {
		m.add("occurred", o)
	}
	m.str("standpoint", e.Standpoint)
	if ps := SortParticipants(e.Participants); len(ps) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, p := range ps {
			seq.Content = append(seq.Content, strNode(p))
		}
		m.add("participants", seq)
	}
	m.str("place", e.Place)
	m.str("label", e.Label)
	m.str("container", e.Container)
	if e.Ordinal != nil {
		m.add("ordinal", intNode(*e.Ordinal))
	}
	if ls := SortLinks(e.Links); len(ls) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, l := range ls {
			lm := newMap()
			lm.str("rel", l.Rel)
			lm.str("href", l.Href)
			lm.extras(l.Extra)
			seq.Content = append(seq.Content, lm.n)
		}
		m.add("links", seq)
	}
	m.add("content", contentNode(e.Content))
	if s := sourceNode(e.Source, false); s != nil {
		m.add("source", s)
	}
	m.ts("timestamp", e.Timestamp)
	m.extras(e.Extra)
	if ss := e.SourceStatus; ss != nil {
		sm := newMap()
		sm.str("state", ss.State)
		sm.ts("verified", ss.Verified)
		sm.ts("detected", ss.Detected)
		sm.extras(ss.Extra)
		m.add("source_status", sm.n)
	}
	if r := e.Redacted; r != nil {
		rm := newMap()
		rm.ts("timestamp", r.Timestamp)
		rm.str("reason", r.Reason)
		if s := sourceNode(r.Source, false); s != nil {
			rm.add("source", s)
		}
		fields := append([]string(nil), r.Fields...)
		sort.Strings(fields)
		fields = dedup(fields)
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
		for _, f := range fields {
			seq.Content = append(seq.Content, strNode(f))
		}
		rm.add("fields", seq)
		rm.extras(r.Extra)
		m.add("redacted", rm.n)
	}
	return EncodeNode(m.n)
}

// EncodeNode renders a node as a document in the house style.
func EncodeNode(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// OccurredNode is the occurred block, flow style when flow is set (as in the
// index). Bounds are always double-quoted. It returns nil for an empty block.
func OccurredNode(o Occurred, flow bool) *yaml.Node {
	m := newMap()
	if flow {
		m.n.Style = yaml.FlowStyle
	}
	if o.Start != "" {
		m.add("start", quoted(o.Start))
	}
	if o.End != "" {
		m.add("end", quoted(o.End))
	}
	m.str("tz", o.TZ)
	m.str("basis", o.Basis)
	m.extras(o.Extra)
	if len(m.n.Content) == 0 {
		return nil
	}
	return m.n
}

func contentNode(c Content) *yaml.Node {
	m := newMap()
	m.str("mode", c.Mode)
	m.str("ref", c.Ref)
	m.str("uid", c.UID)
	if c.Version != "" {
		m.add("version", quoted(c.Version)) // always quoted, as the format shows it
	}
	m.str("hash", c.Hash)
	m.str("media_type", c.MediaType)
	m.str("derivation", c.Derivation)
	m.str("language", c.Language)
	if c.Text != nil {
		n := strNode(*c.Text)
		if strings.Contains(*c.Text, "\n") {
			n.Style = yaml.LiteralStyle
		}
		m.add("text", n)
	}
	m.extras(c.Extra)
	return m.n
}

func sourceNode(s Source, flow bool) *yaml.Node {
	m := newMap()
	if flow {
		m.n.Style = yaml.FlowStyle
	}
	m.str("author", s.Author)
	m.str("harness", s.Harness)
	m.str("model", s.Model)
	m.extras(s.Extra)
	if len(m.n.Content) == 0 {
		return nil
	}
	return m.n
}

// SortParticipants returns participants sorted and de-duplicated.
func SortParticipants(ps []string) []string {
	out := append([]string(nil), ps...)
	sort.Strings(out)
	return dedup(out)
}

// SortLinks returns links sorted by rel then href, with exact duplicates (and
// no extra keys) removed.
func SortLinks(ls []Link) []Link {
	out := append([]Link(nil), ls...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rel != out[j].Rel {
			return out[i].Rel < out[j].Rel
		}
		return out[i].Href < out[j].Href
	})
	var res []Link
	for i, l := range out {
		if i > 0 && len(l.Extra) == 0 && len(out[i-1].Extra) == 0 && l.Rel == out[i-1].Rel && l.Href == out[i-1].Href {
			continue
		}
		res = append(res, l)
	}
	return res
}

func dedup(s []string) []string {
	var out []string
	for i, v := range s {
		if i > 0 && v == s[i-1] {
			continue
		}
		out = append(out, v)
	}
	return out
}

type mapNode struct{ n *yaml.Node }

func newMap() *mapNode { return &mapNode{&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}} }

func (m *mapNode) add(k string, v *yaml.Node) {
	m.n.Content = append(m.n.Content, strNode(k), v)
}

// str adds a string field, omitted when empty.
func (m *mapNode) str(k, v string) {
	if v != "" {
		m.add(k, strNode(v))
	}
}

// ts adds a recording instant. A value in the on-disk form is written plain,
// as the format's examples show it; anything else is written as a string.
func (m *mapNode) ts(k, v string) {
	if v == "" {
		return
	}
	n := strNode(v)
	if isPlainTimestamp(v) {
		n.Tag = "!!timestamp"
	}
	m.add(k, n)
}

func (m *mapNode) extras(x Extras) {
	for _, f := range x {
		m.add(f.Key, f.Value)
	}
}

func strNode(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

func quoted(v string) *yaml.Node {
	n := strNode(v)
	n.Style = yaml.DoubleQuotedStyle
	return n
}

func intNode(v int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
}

// isPlainTimestamp reports whether YAML would read v, unquoted, back as the
// same timestamp text: an RFC 3339 date-time in UTC with Z.
func isPlainTimestamp(v string) bool {
	if len(v) < 20 || v[len(v)-1] != 'Z' || v[10] != 'T' {
		return false
	}
	for i, c := range v {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		case 10:
		case 13, 16:
			if c != ':' {
				return false
			}
		default:
			if i < 19 && (c < '0' || c > '9') {
				return false
			}
		}
	}
	return true
}
