package cli

import (
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
	var since, until, standpoint, kind, container, by string
	var withdrawn bool
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List episodes by when they occurred, whose they are, kind, or container",
		Long: `List episodes whose occurred interval overlaps --since..--until (each a date,
taken as the whole day, or a date-time), filtered by --standpoint, --kind and
--container, ordered by when they occurred (--by recorded orders by id, the
order they were recorded in). Withdrawn episodes are left out unless
--include-withdrawn. An empty result says only that no episodes were found:
a workspace is not an exhaustive record.`,
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
			if by != "occurred" && by != "recorded" {
				return usageErr("--by must be occurred or recorded")
			}
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			l, err := ws.Scan()
			if err != nil {
				return err
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
			if by == "recorded" {
				query.SortByID(ordered)
			} else {
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
	cmd.Flags().StringVar(&by, "by", "occurred", "order by occurred (the start instant) or recorded (id)")
	cmd.Flags().IntVar(&limit, "limit", 0, "at most this many")
	return cmd
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
