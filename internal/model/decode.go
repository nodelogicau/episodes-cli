package model

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ErrUnparseable marks a file that is not an episode document at all: not
// YAML, not one mapping, or a mapping with a duplicate key.
var ErrUnparseable = errors.New("unparseable")

// Decode reads one episode file. It accepts keys in any order and keeps keys
// this format does not define. It fails with ErrUnparseable on bad YAML, a
// document that is not one mapping, or a duplicate key; it returns problems
// (all errors) for known keys whose value has the wrong shape, leaving those
// fields empty.
func Decode(data []byte) (*Episode, []Problem, error) {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrUnparseable, err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return nil, nil, fmt.Errorf("%w: more than one YAML document", ErrUnparseable)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, nil, fmt.Errorf("%w: empty document", ErrUnparseable)
	}
	return DecodeNode(doc.Content[0])
}

// DecodeNode reads an episode, or an index entry, from one mapping node.
func DecodeNode(root *yaml.Node) (*Episode, []Problem, error) {
	d := &decoder{}
	pairs, err := d.mapping(root, "")
	if err != nil {
		return nil, nil, err
	}
	ep := &Episode{present: map[string]bool{}}
	for _, p := range pairs {
		ep.present[p.key] = true
		switch p.key {
		case "id":
			ep.ID = d.str(p.val, "id")
		case "type":
			ep.Type = d.str(p.val, "type")
		case "kind":
			ep.Kind = d.str(p.val, "kind")
		case "occurred":
			if err := d.occurred(p.val, &ep.Occurred); err != nil {
				return nil, nil, err
			}
		case "standpoint":
			ep.Standpoint = d.str(p.val, "standpoint")
		case "participants":
			ep.Participants = d.strs(p.val, "participants")
		case "place":
			ep.Place = d.str(p.val, "place")
		case "label":
			ep.Label = d.str(p.val, "label")
		case "container":
			ep.Container = d.str(p.val, "container")
		case "ordinal":
			ep.Ordinal = d.int(p.val, "ordinal")
		case "links":
			links, err := d.links(p.val)
			if err != nil {
				return nil, nil, err
			}
			ep.Links = links
		case "content":
			if err := d.content(p.val, &ep.Content); err != nil {
				return nil, nil, err
			}
		case "source":
			if err := d.source(p.val, "source", &ep.Source); err != nil {
				return nil, nil, err
			}
		case "timestamp":
			ep.Timestamp = d.str(p.val, "timestamp")
		case "source_status":
			ss := &SourceStatus{}
			if err := d.sourceStatus(p.val, ss); err != nil {
				return nil, nil, err
			}
			ep.SourceStatus = ss
		case "redacted":
			r := &Redacted{}
			if err := d.redacted(p.val, r); err != nil {
				return nil, nil, err
			}
			ep.Redacted = r
		default:
			ep.Extra = append(ep.Extra, Field{Key: p.key, Value: p.val})
		}
	}
	return ep, d.problems, nil
}

type pair struct {
	key string
	val *yaml.Node
}

type decoder struct{ problems []Problem }

func (d *decoder) bad(field, format string, args ...any) {
	d.problems = append(d.problems, Problem{Severity: SevError, Code: "field_type", Field: field, Message: fmt.Sprintf(format, args...)})
}

// mapping returns a mapping's pairs in order, failing on a non-mapping or a
// duplicate key.
func (d *decoder) mapping(n *yaml.Node, where string) ([]pair, error) {
	n = deref(n)
	if n.Kind != yaml.MappingNode {
		if where == "" {
			return nil, fmt.Errorf("%w: the document is not a mapping", ErrUnparseable)
		}
		return nil, nil
	}
	seen := map[string]bool{}
	out := make([]pair, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		if seen[k] {
			at := k
			if where != "" {
				at = where + "." + k
			}
			return nil, fmt.Errorf("%w: duplicate key %q (line %d)", ErrUnparseable, at, n.Content[i].Line)
		}
		seen[k] = true
		out = append(out, pair{k, n.Content[i+1]})
	}
	return out, nil
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func (d *decoder) str(n *yaml.Node, field string) string {
	n = deref(n)
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		d.bad(field, "%s must be a string", field)
		return ""
	}
	return n.Value
}

func (d *decoder) strs(n *yaml.Node, field string) []string {
	n = deref(n)
	if n.Kind != yaml.SequenceNode {
		d.bad(field, "%s must be a list of strings", field)
		return nil
	}
	out := make([]string, 0, len(n.Content))
	for _, c := range n.Content {
		out = append(out, d.str(c, field))
	}
	return out
}

func (d *decoder) int(n *yaml.Node, field string) *int {
	n = deref(n)
	if n.Kind != yaml.ScalarNode || n.Tag != "!!int" {
		d.bad(field, "%s must be an integer", field)
		return nil
	}
	v, err := strconv.Atoi(n.Value)
	if err != nil {
		d.bad(field, "%s must be an integer", field)
		return nil
	}
	return &v
}

func (d *decoder) block(n *yaml.Node, field string) ([]pair, bool, error) {
	if deref(n).Kind != yaml.MappingNode {
		d.bad(field, "%s must be a mapping", field)
		return nil, false, nil
	}
	pairs, err := d.mapping(n, field)
	return pairs, true, err
}

func (d *decoder) occurred(n *yaml.Node, o *Occurred) error {
	pairs, ok, err := d.block(n, "occurred")
	if !ok || err != nil {
		return err
	}
	for _, p := range pairs {
		switch p.key {
		case "start":
			o.Start = d.str(p.val, "occurred.start")
		case "end":
			o.End = d.str(p.val, "occurred.end")
		case "tz":
			o.TZ = d.str(p.val, "occurred.tz")
		case "basis":
			o.Basis = d.str(p.val, "occurred.basis")
		default:
			o.Extra = append(o.Extra, Field{p.key, p.val})
		}
	}
	return nil
}

func (d *decoder) links(n *yaml.Node) ([]Link, error) {
	n = deref(n)
	if n.Kind != yaml.SequenceNode {
		d.bad("links", "links must be a list")
		return nil, nil
	}
	out := make([]Link, 0, len(n.Content))
	for i, c := range n.Content {
		field := fmt.Sprintf("links[%d]", i)
		pairs, ok, err := d.block(c, field)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		var l Link
		for _, p := range pairs {
			switch p.key {
			case "rel":
				l.Rel = d.str(p.val, field+".rel")
			case "href":
				l.Href = d.str(p.val, field+".href")
			default:
				l.Extra = append(l.Extra, Field{p.key, p.val})
			}
		}
		out = append(out, l)
	}
	return out, nil
}

func (d *decoder) content(n *yaml.Node, c *Content) error {
	pairs, ok, err := d.block(n, "content")
	if !ok || err != nil {
		return err
	}
	for _, p := range pairs {
		f := "content." + p.key
		switch p.key {
		case "mode":
			c.Mode = d.str(p.val, f)
		case "ref":
			c.Ref = d.str(p.val, f)
		case "uid":
			c.UID = d.str(p.val, f)
		case "version":
			c.Version = d.str(p.val, f)
		case "hash":
			c.Hash = d.str(p.val, f)
		case "media_type":
			c.MediaType = d.str(p.val, f)
		case "derivation":
			c.Derivation = d.str(p.val, f)
		case "language":
			c.Language = d.str(p.val, f)
		case "text":
			t := d.str(p.val, f)
			c.Text = &t
		default:
			c.Extra = append(c.Extra, Field{p.key, p.val})
		}
	}
	return nil
}

func (d *decoder) source(n *yaml.Node, field string, s *Source) error {
	pairs, ok, err := d.block(n, field)
	if !ok || err != nil {
		return err
	}
	for _, p := range pairs {
		switch p.key {
		case "author":
			s.Author = d.str(p.val, field+".author")
		case "harness":
			s.Harness = d.str(p.val, field+".harness")
		case "model":
			s.Model = d.str(p.val, field+".model")
		default:
			s.Extra = append(s.Extra, Field{p.key, p.val})
		}
	}
	return nil
}

func (d *decoder) sourceStatus(n *yaml.Node, s *SourceStatus) error {
	pairs, ok, err := d.block(n, "source_status")
	if !ok || err != nil {
		return err
	}
	for _, p := range pairs {
		f := "source_status." + p.key
		switch p.key {
		case "state":
			s.State = d.str(p.val, f)
		case "verified":
			s.Verified = d.str(p.val, f)
		case "detected":
			s.Detected = d.str(p.val, f)
		default:
			s.Extra = append(s.Extra, Field{p.key, p.val})
		}
	}
	return nil
}

func (d *decoder) redacted(n *yaml.Node, r *Redacted) error {
	pairs, ok, err := d.block(n, "redacted")
	if !ok || err != nil {
		return err
	}
	for _, p := range pairs {
		f := "redacted." + p.key
		switch p.key {
		case "timestamp":
			r.Timestamp = d.str(p.val, f)
		case "reason":
			r.Reason = d.str(p.val, f)
		case "source":
			if err := d.source(p.val, f, &r.Source); err != nil {
				return err
			}
		case "fields":
			r.Fields = d.strs(p.val, f)
		default:
			r.Extra = append(r.Extra, Field{p.key, p.val})
		}
	}
	return nil
}
