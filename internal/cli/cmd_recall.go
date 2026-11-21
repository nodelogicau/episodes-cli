package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/query"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

// readEpisode loads one episode by the path its id derives, never the index.
func readEpisode(ws *store.Workspace, id string) (*model.Episode, []byte, error) {
	ep, data, err := ws.Read(id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, notFoundErr("%v", err)
	}
	return ep, data, err
}

// recall resolves entries to episodes, the file winning: each survivor of
// the index-field filter is parsed, filtered again on what the file says,
// and returned.
func recall(ws *store.Workspace, l *store.Listing, entries []store.Entry, f query.Filter) []*model.Episode {
	byID := map[string]store.FileRef{}
	for _, file := range l.Files {
		if !file.Misplaced() {
			byID[file.ID] = file
		}
	}
	var out []*model.Episode
	for _, e := range entries {
		if !f.Match(e) {
			continue
		}
		file, ok := byID[e.ID]
		if !ok {
			continue
		}
		ld := ws.LoadFile(file)
		if ld.Err != nil || ld.Episode.ID != e.ID || !f.Match(store.EntryFor(ld.Episode)) {
			continue
		}
		out = append(out, ld.Episode)
	}
	return out
}

func entriesOf(eps []*model.Episode) []store.Entry {
	out := make([]store.Entry, len(eps))
	for i, ep := range eps {
		out[i] = store.EntryFor(ep)
	}
	return out
}

func (a *app) showCmd() *cobra.Command {
	var children bool
	cmd := &cobra.Command{
		Use:   "show <id> [--children]",
		Short: "Print an episode (and, with --children, the episodes it contains, in order)",
		Args:  cobra.ExactArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			ep, _, err := readEpisode(ws, args[0])
			if err != nil {
				return err
			}
			var kids []*model.Episode
			if children {
				l, err := ws.Scan()
				if err != nil {
					return err
				}
				kids = recall(ws, l, childEntries(ws, l, ep.ID), query.Filter{Container: ep.ID})
				byID := map[string]*model.Episode{}
				for _, k := range kids {
					byID[k.ID] = k
				}
				ordered := entriesOf(kids)
				query.SortChildren(ordered)
				kids = kids[:0]
				for _, e := range ordered {
					kids = append(kids, byID[e.ID])
				}
			}
			out := map[string]any{"episode": episodeJSON(ep), "path": store.RelPath(ep.ID)}
			if children {
				items := make([]map[string]any, 0, len(kids))
				for _, k := range kids {
					items = append(items, episodeJSON(k))
				}
				out["children"] = items
			}
			return a.emit(out, func(w io.Writer) {
				data, _ := model.Encode(ep)
				fmt.Fprintf(w, "# %s\n%s", store.RelPath(ep.ID), data)
				for _, k := range kids {
					d, _ := model.Encode(k)
					fmt.Fprintf(w, "---\n# %s\n%s", store.RelPath(k.ID), d)
				}
			})
		}),
	}
	cmd.Flags().BoolVar(&children, "children", false, "also print the episodes naming it as container, by ordinal then id")
	return cmd
}

func (a *app) listCmd() *cobra.Command {
	var since, until, standpoint, kind, container, by, fields string
	var withdrawn, ndjson bool
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List episodes by when they occurred, whose they are, kind, or container",
		Long: `List episodes whose occurred interval overlaps --since..--until (each a date,
taken as the whole day, or a date-time), filtered by --standpoint, --kind and
--container, ordered by when they occurred (--by recorded orders by id, the
order they were recorded in). With --container the default is --by ordinal:
the container's order, by ordinal, then by id, with children that have no
ordinal last. Withdrawn episodes are left out unless
--include-withdrawn. An empty result says only that no episodes were found:
a workspace is not an exhaustive record.

--ndjson streams one JSON object per episode per line, in id order (--by
recorded) unless --by says otherwise; --by occurred and --by ordinal hold
the matching index entries, not the episodes, until they are sorted.
--fields full (the default) writes whole episodes. --fields index writes
only the seven indexed fields (id, kind, occurred, container, ordinal,
standpoint, timestamp) and path, and opens no episode file the index
already covers, so it is fast over a large workspace; it cannot tell a
withdrawn episode from any other, and lists withdrawn episodes too. Use
--fields full to leave them out.`,
		Args: cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			f := query.Filter{Standpoint: standpoint, Kind: kind, Container: container}
			for _, b := range []struct {
				flag, v string
				dst     **temporal.Value
			}{{"--since", since, &f.Since}, {"--until", until, &f.Until}} {
				if b.v == "" {
					continue
				}
				v, err := temporal.Parse(b.v)
				if err != nil {
					return usageErr("%s: %v", b.flag, err)
				}
				*b.dst = &v
			}
			switch {
			case ndjson && a.jsonOut:
				return usageErr("use either --json or --ndjson, not both")
			case fields != "" && !ndjson:
				return usageErr("--fields chooses what --ndjson writes; give --ndjson too")
			case fields == "":
				fields = "full"
			case fields != "full" && fields != "index":
				return usageErr("--fields must be full or index")
			}
			if by == "" {
				by = "occurred"
				switch {
				case ndjson:
					by = "recorded"
				case container != "":
					by = "ordinal"
				}
			}
			switch {
			case by == "ordinal" && container == "":
				return usageErr("--by ordinal is the order within --container; give --container too")
			case by != "occurred" && by != "recorded" && by != "ordinal":
				return usageErr("--by must be occurred, recorded or ordinal")
			}
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			l, err := ws.Scan()
			if err != nil {
				return err
			}
			if ndjson {
				return a.listNDJSON(ws, l, f, by, fields == "index", withdrawn, limit)
			}
			var entries []store.Entry
			if container != "" {
				entries = childEntries(ws, l, container)
			} else {
				entries = ws.Entries(l, 0, false)
			}
			eps := recall(ws, l, entries, f)
			byID := map[string]*model.Episode{}
			for _, ep := range eps {
				byID[ep.ID] = ep
			}
			ordered := entriesOf(eps)
			switch by {
			case "recorded":
				query.SortByID(ordered)
			case "ordinal":
				query.SortChildren(ordered)
			default:
				query.SortByOccurred(ordered)
			}
			var shown []*model.Episode
			for _, e := range ordered {
				ep := byID[e.ID]
				if ep.Withdrawn() && !withdrawn {
					continue
				}
				shown = append(shown, ep)
				if limit > 0 && len(shown) == limit {
					break
				}
			}
			items := make([]map[string]any, 0, len(shown))
			for _, ep := range shown {
				items = append(items, listItem(ep))
			}
			return a.emit(map[string]any{"episodes": items, "count": len(items)}, func(w io.Writer) {
				if len(shown) == 0 {
					fmt.Fprintln(w, "No episodes found.")
					return
				}
				for _, ep := range shown {
					listLine(w, ep)
				}
			})
		}),
	}
	cmd.Flags().StringVar(&since, "since", "", "occurred on or after: a date (whole day) or a date-time")
	cmd.Flags().StringVar(&until, "until", "", "occurred on or before: a date (whole day) or a date-time")
	cmd.Flags().StringVar(&standpoint, "standpoint", "", "only episodes from this standpoint")
	cmd.Flags().StringVar(&kind, "kind", "", "only episodes of this kind")
	cmd.Flags().StringVar(&container, "container", "", "only episodes in this container")
	cmd.Flags().BoolVar(&withdrawn, "include-withdrawn", false, "include inline episodes whose text was redacted")
	cmd.Flags().StringVar(&by, "by", "", "order by occurred (the start instant), recorded (id), or ordinal (within --container: ordinal, then id); default occurred, or ordinal with --container")
	cmd.Flags().IntVar(&limit, "limit", 0, "at most this many")
	cmd.Flags().BoolVar(&ndjson, "ndjson", false, "stream one JSON object per episode per line (not with --json); default order --by recorded")
	cmd.Flags().StringVar(&fields, "fields", "", "with --ndjson: full (whole episodes, the default) or index (the seven indexed fields and path, no file opened that the index covers, withdrawn episodes included)")
	return cmd
}

// indexItem is one index entry as list --ndjson --fields index writes it.
func indexItem(e store.Entry, rel string) map[string]any {
	m := map[string]any{"id": e.ID, "kind": e.Kind, "occurred": occurredJSON(e.Occurred),
		"standpoint": e.Standpoint, "timestamp": e.Timestamp, "path": rel}
	if e.Container != "" {
		m["container"] = e.Container
	}
	if e.Ordinal != nil {
		m["ordinal"] = *e.Ordinal
	}
	return m
}

// listNDJSON streams list's result one line per episode (design D6 of
// bulk-import). In id order nothing is held but the listing and the index
// of the month being read; another order holds the matching entries.
// Entries are filtered first, and with --fields full each survivor's file
// is parsed, filtered again on what it says, and written at once.
func (a *app) listNDJSON(ws *store.Workspace, l *store.Listing, f query.Filter, by string, index, withdrawn bool, limit int) error {
	enc := json.NewEncoder(a.stdout)
	enc.SetEscapeHTML(false)
	n := 0
	errLimit := errors.New("limit reached")
	emit := func(e store.Entry, file store.FileRef) error {
		if index {
			if err := enc.Encode(indexItem(e, file.Rel)); err != nil {
				return err
			}
		} else {
			ld := ws.LoadFile(file)
			if ld.Err != nil || ld.Episode.ID != e.ID || !f.Match(store.EntryFor(ld.Episode)) {
				return nil
			}
			if ld.Episode.Withdrawn() && !withdrawn {
				return nil
			}
			if err := enc.Encode(episodeJSON(ld.Episode)); err != nil {
				return err
			}
		}
		if n++; limit > 0 && n == limit {
			return errLimit
		}
		return nil
	}
	minMonth, hasMin := 0, false
	if f.Container != "" {
		minMonth, hasMin = store.IDMonth(f.Container)
	}
	type held struct {
		e    store.Entry
		file store.FileRef
	}
	var buf []held
	err := ws.EachEntry(l, minMonth, hasMin, func(e store.Entry, file store.FileRef) error {
		if file.Misplaced() || !f.Match(e) {
			return nil
		}
		if by == "recorded" {
			return emit(e, file)
		}
		buf = append(buf, held{e, file})
		return nil
	})
	if err == nil && by != "recorded" {
		es := make([]store.Entry, len(buf))
		files := make(map[string]store.FileRef, len(buf))
		for i, h := range buf {
			es[i], files[h.e.ID] = h.e, h.file
		}
		if by == "ordinal" {
			query.SortChildren(es)
		} else {
			query.SortByOccurred(es)
		}
		for _, e := range es {
			if err = emit(e, files[e.ID]); err != nil {
				break
			}
		}
	}
	if errors.Is(err, errLimit) {
		return nil
	}
	return err
}

func (a *app) threadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread <container>",
		Short: "Print a container's children in ordinal order, with standpoint, time and text",
		Args:  cobra.ExactArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			container, _, err := readEpisode(ws, args[0])
			if err != nil {
				return err
			}
			l, err := ws.Scan()
			if err != nil {
				return err
			}
			eps := recall(ws, l, childEntries(ws, l, container.ID), query.Filter{Container: container.ID})
			byID := map[string]*model.Episode{}
			for _, ep := range eps {
				byID[ep.ID] = ep
			}
			ordered := entriesOf(eps)
			query.SortChildren(ordered)
			items := make([]map[string]any, 0, len(ordered))
			for _, e := range ordered {
				items = append(items, threadItem(byID[e.ID]))
			}
			return a.emit(map[string]any{"container": listItem(container), "children": items, "count": len(items)}, func(w io.Writer) {
				fmt.Fprintf(w, "%s  %s  %s  %s\n\n", container.ID, container.Kind, container.Standpoint, container.Occurred.Start)
				if len(ordered) == 0 {
					fmt.Fprintln(w, "No episodes found in this container.")
				}
				for _, e := range ordered {
					threadLine(w, byID[e.ID])
				}
			})
		}),
	}
	return cmd
}
