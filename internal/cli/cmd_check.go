package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/query"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

func (a *app) validateCmd() *cobra.Command {
	var now string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check every episode, episodes.yaml, the index and every segment (exit 4 on any error)",
		Args:  cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			at, _, err := resolveNow(actFlags{now: now})
			if err != nil {
				return err
			}
			l, err := ws.Scan()
			if err != nil {
				return err
			}
			r := query.Validate(query.Workspace{WS: ws, Listing: l, Loaded: ws.LoadAll(l), Now: at})
			if err := a.emit(map[string]any{"valid": r.Errors == 0, "episodes": r.Episodes, "errors": r.Errors, "warnings": r.Warnings, "infos": r.Infos, "findings": r.Findings}, func(w io.Writer) {
				for _, f := range r.Findings {
					where := f.ID
					if where == "" {
						where = f.Path
					}
					field := ""
					if f.Field != "" {
						field = " (" + f.Field + ")"
					}
					fmt.Fprintf(w, "%-7s %s %s%s: %s\n", f.Severity, f.Code, where, field, f.Message)
				}
				fmt.Fprintf(w, "%d episode(s): %d error(s), %d warning(s), %d info\n", r.Episodes, r.Errors, r.Warnings, r.Infos)
			}); err != nil {
				return err
			}
			if r.Errors > 0 {
				return checkFailed()
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&now, "now", "", "test-facing: the current time as RFC 3339, for the future-id and future-timestamp checks (default: $"+EnvNow+", then the clock)")
	return cmd
}

func (a *app) indexCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "index [--check]",
		Short: "Rebuild index.yaml from the files and write any missing segment; --check compares without writing",
		Long: `Rebuild the derived index from the episode files: the root index.yaml,
replacing whatever was there (git conflict markers included), and any sealed
segment not yet written. An existing segment is never rewritten: a sealed
month whose file count differs from its segment is a late merge, reported
with the files and the remedy, and the command exits 4. --check compares the
committed index with the files, writes nothing, and exits 4 on any
difference.`,
		Args: cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			if check {
				d, err := ws.CheckIndex()
				if err != nil {
					return err
				}
				if err := a.emit(map[string]any{"clean": d.Clean(), "diff": d}, func(w io.Writer) {
					if d.Clean() {
						fmt.Fprintln(w, "index matches the files")
						return
					}
					if d.RootUnreadable != "" {
						fmt.Fprintf(w, "root unreadable: %s\n", d.RootUnreadable)
					}
					for _, l := range []struct {
						name string
						ids  []string
					}{{"missing", d.Missing}, {"extra", d.Extra}, {"changed", d.Changed}, {"segment missing", d.SegmentsMissing}, {"segment extra", d.SegmentsExtra}, {"count changed", d.CountsChanged}} {
						for _, id := range l.ids {
							fmt.Fprintf(w, "%s: %s\n", l.name, id)
						}
					}
					lateText(w, d.Late)
				}); err != nil {
					return err
				}
				if !d.Clean() {
					return checkFailed()
				}
				return nil
			}
			unlock, err := ws.Lock()
			if err != nil {
				return err
			}
			defer unlock()
			res, err := ws.Rebuild()
			if err != nil {
				return err
			}
			out := map[string]any{"entries": res.Entries, "segments_written": res.Segments, "late_merges": res.Late}
			if err := a.emit(out, func(w io.Writer) {
				fmt.Fprintf(w, "Rebuilt %s: %d open-month entr%s\n", store.IndexFile, res.Entries, map[bool]string{true: "y", false: "ies"}[res.Entries == 1])
				for _, s := range res.Segments {
					fmt.Fprintf(w, "  wrote %s\n", s)
				}
				lateText(w, res.Late)
			}); err != nil {
				return err
			}
			if len(res.Late) > 0 {
				return checkFailed()
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&check, "check", false, "compare the committed index with the files; write nothing")
	return cmd
}

func lateText(w io.Writer, late []store.LateMerge) {
	for _, l := range late {
		fmt.Fprintf(w, "late merge: %s counts %d, its month holds %d files; not in the segment: %s\n", l.Segment, l.Count, l.Present, strings.Join(l.Files, ", "))
	}
	if len(late) > 0 {
		fmt.Fprintf(w, "remedy: %s\n", store.LateMergeRemedy)
	}
}
