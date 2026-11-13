package cli

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

// episodeJSON is an episode as a JSON-ready value, read back from its
// canonical encoding so JSON and file never disagree.
func episodeJSON(ep *model.Episode) map[string]any {
	data, err := model.Encode(ep)
	if err != nil {
		return map[string]any{"id": ep.ID}
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return map[string]any{"id": ep.ID}
	}
	return jsonable(m).(map[string]any)
}

// jsonable turns YAML's decoded values (timestamps included) into JSON ones.
func jsonable(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = jsonable(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = jsonable(x)
		}
		return t
	case interface{ Format(string) string }:
		return t.Format("2006-01-02T15:04:05Z")
	}
	return v
}

func occurredJSON(o model.Occurred) map[string]any {
	m := map[string]any{"start": o.Start, "basis": o.Basis}
	if o.End != "" {
		m["end"] = o.End
	}
	if o.TZ != "" {
		m["tz"] = o.TZ
	}
	return m
}

// listItem is one recalled episode in a list.
func listItem(ep *model.Episode) map[string]any {
	m := map[string]any{
		"id": ep.ID, "kind": ep.Kind, "occurred": occurredJSON(ep.Occurred),
		"standpoint": ep.Standpoint, "timestamp": ep.Timestamp, "path": store.RelPath(ep.ID),
		"mode": ep.Content.Mode,
	}
	if ep.Label != "" {
		m["label"] = ep.Label
	}
	if ep.Container != "" {
		m["container"] = ep.Container
	}
	if ep.Ordinal != nil {
		m["ordinal"] = *ep.Ordinal
	}
	if ep.Withdrawn() {
		m["withdrawn"] = true
	}
	if ep.Tombstone() {
		m["tombstone"] = true
	}
	if ep.SourceStatus != nil {
		m["source_state"] = ep.SourceStatus.State
	}
	return m
}

func listLine(w io.Writer, ep *model.Episode) {
	marks := ""
	switch {
	case ep.Withdrawn():
		marks = " [withdrawn]"
	case ep.Tombstone():
		marks = " [tombstone]"
	}
	if ep.SourceStatus != nil {
		marks += " [source " + ep.SourceStatus.State + "]"
	}
	label := ""
	if ep.Label != "" {
		label = "  " + ep.Label
	}
	fmt.Fprintf(w, "%s  %-12s %s  %s%s%s\n", ep.Occurred.Start, ep.Kind, ep.Standpoint, ep.ID, marks, label)
}

// threadLine renders one child of a container for `thread`.
func threadLine(w io.Writer, ep *model.Episode) {
	pos := "-"
	if ep.Ordinal != nil {
		pos = fmt.Sprint(*ep.Ordinal)
	}
	if ep.Withdrawn() {
		fmt.Fprintf(w, "[%s] (withdrawn: this turn's text was redacted)  %s\n\n", pos, ep.ID)
		return
	}
	fmt.Fprintf(w, "[%s] %s  %s  %s\n", pos, ep.Standpoint, ep.Occurred.Start, ep.ID)
	switch ep.Content.Mode {
	case model.ModeInline:
		if ep.Content.Text != nil {
			for _, line := range strings.Split(strings.TrimRight(*ep.Content.Text, "\n"), "\n") {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
	case model.ModeRef:
		if ep.Tombstone() {
			fmt.Fprintf(w, "    (reference; its location was redacted)\n")
		} else {
			fmt.Fprintf(w, "    (reference) %s\n", ep.Content.Ref)
		}
	default:
		fmt.Fprintf(w, "    (no content)\n")
	}
	fmt.Fprintln(w)
}

// threadItem is one child in a thread's JSON. A withdrawn turn keeps its
// position and nothing else of its record.
func threadItem(ep *model.Episode) map[string]any {
	if ep.Withdrawn() {
		m := map[string]any{"id": ep.ID, "withdrawn": true}
		if ep.Ordinal != nil {
			m["ordinal"] = *ep.Ordinal
		}
		return m
	}
	m := listItem(ep)
	if ep.Content.Text != nil {
		m["text"] = *ep.Content.Text
	}
	if ep.Content.Mode == model.ModeRef && ep.Content.Ref != "" {
		m["ref"] = ep.Content.Ref
	}
	return m
}
