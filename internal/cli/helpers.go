// Origin: nodelogicau/intentions-cli internal/cli/helpers.go @ b72a395454aa
// (actFlags, addActFlags, firstNonEmpty, resolveNow and readDescription,
// renamed for episodes; source resolution moved to internal/prov)

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/prov"
	"github.com/nodelogicau/episodes-cli/internal/query"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

// Environment variables beyond attribution.
const (
	EnvNow  = "EPISODES_NOW"
	EnvZone = "EPISODES_ZONE"
)

// actFlags are the flags every writing verb shares: attribution and time.
type actFlags struct {
	author, harness, model string
	now                    string
}

func addActFlags(cmd *cobra.Command, f *actFlags) {
	cmd.Flags().StringVar(&f.author, "author", "", "on whose behalf this is recorded (default: $"+prov.EnvAuthor+", then defaults.source.author)")
	cmd.Flags().StringVar(&f.harness, "harness", "", "the harness recording it (default: $"+prov.EnvHarness+", then defaults.source.harness)")
	cmd.Flags().StringVar(&f.model, "model", "", "the model behind the harness (default: $"+prov.EnvModel+", then defaults.source.model)")
	cmd.Flags().StringVar(&f.now, "now", "", "test-facing: the current time as RFC 3339, which sets the timestamp, the minting instant and any occurred defaulted to now (default: $"+EnvNow+", then the clock)")
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// resolveSource applies flag, then environment, then episodes.yaml, and
// requires an author or a harness.
func resolveSource(ws *store.Workspace, f actFlags) (model.Source, error) {
	src := prov.Resolve(ws.Config.Defaults.Source.Source(), prov.Explicit{Author: f.author, Harness: f.harness, Model: f.model})
	return src, prov.Require(src)
}

// resolveNow applies --now, then EPISODES_NOW, then the clock. fixed is true
// when the time was given, so the minting instant follows it too.
func resolveNow(f actFlags) (now time.Time, fixed bool, err error) {
	s := firstNonEmpty(f.now, os.Getenv(EnvNow))
	if s == "" {
		return time.Now().UTC().Truncate(time.Second), false, nil
	}
	t, err := temporal.ParseTimestamp(s)
	if err != nil {
		return time.Time{}, false, usageErr("--now: %v", err)
	}
	return t, true, nil
}

// mint returns a new id: at now when the time was fixed for tests, else from
// the clock with google/uuid's monotonic sequence.
func mint(now time.Time, fixed bool) string {
	if fixed {
		return model.NewIDAt(now)
	}
	return model.NewID()
}

// readText returns prose from --text or --text-file (- for piped stdin),
// never from a terminal.
func (a *app) readText(text, file string, textSet bool, flag string) (string, bool, error) {
	switch {
	case textSet && file != "":
		return "", false, usageErr("use either --%s or --%s-file, not both", flag, flag)
	case file == "-":
		if a.stdinIsTerminal != nil && a.stdinIsTerminal() {
			return "", false, usageErr("--%s-file - requires piped stdin; refusing to read from a terminal", flag)
		}
		b, err := io.ReadAll(a.stdin)
		if err != nil {
			return "", false, err
		}
		return string(b), true, nil
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return "", false, usageErr("--%s-file: %v", flag, err)
		}
		return string(b), true, nil
	}
	return text, textSet, nil
}

// readBytes reads a file argument, - meaning piped stdin.
func (a *app) readBytes(file, flag string) ([]byte, error) {
	if file == "-" {
		if a.stdinIsTerminal != nil && a.stdinIsTerminal() {
			return nil, usageErr("%s - requires piped stdin; refusing to read from a terminal", flag)
		}
		return io.ReadAll(a.stdin)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, usageErr("%s: %v", flag, err)
	}
	return b, nil
}

// occurredFlags set occurred explicitly.
type occurredFlags struct {
	start, end, tz, basis string
}

func addOccurredFlags(cmd *cobra.Command, f *occurredFlags, required bool) {
	help := "when it happened: an RFC 3339 date-time with offset, optionally [IANA zone], or a date (default: now, basis recorded)"
	if required {
		help = "when it happened: an RFC 3339 date-time with offset, optionally [IANA zone], or a date (required)"
	}
	cmd.Flags().StringVar(&f.start, "occurred", "", help)
	cmd.Flags().StringVar(&f.end, "end", "", "when it ended, in the same form as --occurred; leave it out while it is still going on")
	cmd.Flags().StringVar(&f.tz, "tz", "", "the IANA zone a date alone is reckoned in (only with a date)")
	cmd.Flags().StringVar(&f.basis, "basis", "", "how the time is known: stated (a source gave it) or inferred (worked out); required with --occurred")
}

// zoneFor is the zone a verb writes when it supplies occurred itself:
// --zone, then EPISODES_ZONE, then defaults.zone; never the host's.
func zoneFor(ws *store.Workspace, flag string) (string, error) {
	z := firstNonEmpty(flag, os.Getenv(EnvZone), ws.Config.Defaults.Zone)
	if z == "" {
		return "", nil
	}
	if _, err := temporal.LoadZone(z); err != nil {
		return "", usageErr("--zone: %v", err)
	}
	return z, nil
}

// buildOccurred is the occurred block from flags, or now with basis
// recorded when the verb may default it.
func buildOccurred(f occurredFlags, mayDefault bool, now time.Time, zone string) (model.Occurred, error) {
	if f.start == "" {
		switch {
		case !mayDefault:
			return model.Occurred{}, usageErr("--occurred is required: when the happening took place, as its source states it or as worked out")
		case f.end != "" || f.tz != "" || f.basis != "":
			return model.Occurred{}, usageErr("--end, --tz and --basis need --occurred; without it the time is now and the basis recorded")
		}
		start, err := temporal.RecordedNow(now, zone)
		if err != nil {
			return model.Occurred{}, usageErr("--zone: %v", err)
		}
		return model.Occurred{Start: start, Basis: model.BasisRecorded}, nil
	}
	switch f.basis {
	case "":
		return model.Occurred{}, refusedErr("basis: --occurred needs --basis (stated or inferred, or recorded if the harness observed it as it happened); the basis is never defaulted for a supplied time")
	case "planned":
		return model.Occurred{}, refusedErr("basis: planned is not a basis; occurred comes from the happening, never from a plan, schedule or time set aside")
	}
	return model.Occurred{Start: f.start, End: f.end, TZ: f.tz, Basis: f.basis}, nil
}

// parseLinks reads repeated --link rel=href.
func parseLinks(raw []string) ([]model.Link, error) {
	var out []model.Link
	for _, r := range raw {
		rel, href, ok := strings.Cut(r, "=")
		if !ok || rel == "" || href == "" {
			return nil, usageErr("--link takes rel=href, e.g. via=epi_…; got %q", r)
		}
		out = append(out, model.Link{Rel: rel, Href: href})
	}
	return out, nil
}

// checkProblems refuses a write that breaks any rule, naming every rule.
func checkProblems(ep *model.Episode) error {
	if errs := model.Check(ep, model.ForWrite).Errors(); len(errs) > 0 {
		return errs
	}
	return nil
}

// resolveEarlier requires that ref names an episode in the workspace minted
// before id: exit 3 when it does not exist, exit 2 when it is not earlier.
func resolveEarlier(ws *store.Workspace, ref, id, what string) (*model.Episode, error) {
	ep, _, err := ws.Read(ref)
	if errors.Is(err, store.ErrNotFound) {
		return nil, notFoundErr("%s %s does not exist in this workspace", what, ref)
	}
	if err != nil {
		return nil, err
	}
	if ref >= id {
		return nil, refusedErr("%s %s was not recorded before the new episode %s; a %s names an earlier episode", what, ref, id, what)
	}
	return ep, nil
}

// checkWorkspace applies the write-time rules that need the workspace: the
// container resolves and is earlier, (container, ordinal) is free, and every
// episode href resolves and is earlier.
func checkWorkspace(ws *store.Workspace, l *store.Listing, ep *model.Episode) error {
	if ep.Container != "" {
		if _, err := resolveEarlier(ws, ep.Container, ep.ID, "container"); err != nil {
			return err
		}
		if ep.Ordinal != nil {
			children := query.Children(childEntries(ws, l, ep.Container), ep.Container)
			if holder, ok := query.HolderOf(children, *ep.Ordinal); ok {
				return dupErr("duplicate: %s already holds ordinal %d in container %s", holder, *ep.Ordinal, ep.Container)
			}
		}
	}
	for _, link := range ep.Links {
		if model.IsID(link.Href) {
			if _, err := resolveEarlier(ws, link.Href, ep.ID, "link target"); err != nil {
				return err
			}
		}
	}
	return nil
}

// childEntries is the recall view from the container's month on: children
// are minted after their container, so earlier months cannot hold one.
func childEntries(ws *store.Workspace, l *store.Listing, container string) []store.Entry {
	m, ok := store.IDMonth(container)
	return ws.Entries(l, m, ok)
}

// splitFields reads --fields a,b --fields c.
func splitFields(raw []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range raw {
		for _, f := range strings.Split(r, ",") {
			if f = strings.TrimSpace(f); f != "" && !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	sort.Strings(out)
	return out
}

func sourceJSON(s model.Source) map[string]any {
	m := map[string]any{}
	if s.Author != "" {
		m["author"] = s.Author
	}
	if s.Harness != "" {
		m["harness"] = s.Harness
	}
	if s.Model != "" {
		m["model"] = s.Model
	}
	return m
}

func describeSource(s model.Source) string {
	var parts []string
	if s.Author != "" {
		parts = append(parts, "author "+s.Author)
	}
	if s.Harness != "" {
		parts = append(parts, "harness "+s.Harness)
	}
	if s.Model != "" {
		parts = append(parts, "model "+s.Model)
	}
	return fmt.Sprint(strings.Join(parts, ", "))
}
