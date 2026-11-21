// query: recall by free text, the model-free baseline (change
// recall-by-query, #16).

package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/query"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

func (a *app) queryCmd() *cobra.Command {
	var since, until, tz, standpoint, kind, container, participant string
	var limit int
	cmd := &cobra.Command{
		Use:   "query <text>...",
		Short: "Find episodes whose label or inline text mentions any of the query's terms",
		Long: `Find episodes by free text, with no model. Several arguments are joined with
spaces into one query.

The query is trimmed of whitespace (Unicode White_Space) at its edges, and its
terms are its runs of non-whitespace. Leading and trailing punctuation
(Unicode category P) is stripped from each term and from the whole query;
punctuation inside a term is kept, so e-mail and auth/session stay whole. An
episode matches when the whole query, or any one term, occurs under Unicode
case folding in its label or, for an inline episode, its text. A reference
episode's content is not searched, nor are participants. A term that equals
an episode's content.uid exactly, before or after trimming and with no
folding, also matches it. No stemming, no stopwords.

Each result has a resolution: uid when a term equalled the episode's uid,
else lexical. Results are ordered by resolution (uid first), then by how many
distinct terms matched, most first, then by id, newest first. --limit applies
after ordering. A container is returned only when it matched itself, not
because a child did; each result carries its container.

--since, --until, --tz, --standpoint, --kind and --container filter as for
list: a date alone is a day in your zone (--tz, then $` + EnvZone + `, then
defaults.zone, then UTC), and --json reports it as zone.
--participant keeps episodes whose participants include the reference
exactly as written. Withdrawn episodes are never returned, and a redacted
field never matches. Every candidate file is read, so a query is linear in
the episodes the filters leave. An empty result says only that no episodes
were found: a workspace is not an exhaustive record.`,
		Args: cobra.MinimumNArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			q := strings.Join(args, " ")
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			f, zone, err := a.recallFilter(ws, since, until, tz, standpoint, kind, container)
			if err != nil {
				return err
			}
			m := query.NewMatcher(q)
			var results []query.Result
			if !m.Empty() {
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
				for _, ep := range recall(ws, l, entries, f) {
					if ep.Withdrawn() || participant != "" && !slices.Contains(ep.Participants, participant) {
						continue
					}
					if res, n := m.Match(ep); res != "" {
						results = append(results, query.Result{Episode: ep, Resolution: res, Matched: n})
					}
				}
			}
			query.SortResults(results)
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}
			items := make([]map[string]any, 0, len(results))
			for _, r := range results {
				it := listItem(r.Episode)
				it["resolution"], it["matched"] = r.Resolution, r.Matched
				items = append(items, it)
			}
			out := map[string]any{"query": q, "episodes": items, "count": len(items)}
			if zone != "" {
				out["zone"] = zone
			}
			return a.emit(out, func(w io.Writer) {
				if len(results) == 0 {
					fmt.Fprintln(w, "No episodes found.")
					return
				}
				for _, r := range results {
					fmt.Fprintf(w, "%-7s ", r.Resolution)
					listLine(w, r.Episode)
				}
			})
		}),
	}
	cmd.Flags().StringVar(&since, "since", "", "occurred on or after: a date (whole day) or a date-time")
	cmd.Flags().StringVar(&until, "until", "", "occurred on or before: a date (whole day) or a date-time")
	cmd.Flags().StringVar(&tz, "tz", "", tzFilterUsage)
	cmd.Flags().StringVar(&standpoint, "standpoint", "", "only episodes from this standpoint")
	cmd.Flags().StringVar(&kind, "kind", "", "only episodes of this kind")
	cmd.Flags().StringVar(&container, "container", "", "only episodes in this container")
	cmd.Flags().StringVar(&participant, "participant", "", "only episodes whose participants include this reference, as written")
	cmd.Flags().IntVar(&limit, "limit", 0, "at most this many, after ordering")
	return cmd
}
