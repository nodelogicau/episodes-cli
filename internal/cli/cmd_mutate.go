package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

// GitNotice is said on every redaction.
const GitNotice = "The removed text remains in version-control history and in every existing clone or earlier fetch of this workspace; redaction removes it from the current file and from what this tool reads, recalls and serves, not from git."

// writeOnly is write without emit, for the two mutations, which emit
// what Amend returns.
func (a *app) writeOnly(f actFlags, fn func(c *writeCtx) error) error {
	ws, err := a.openWorkspace()
	if err != nil {
		return err
	}
	src, err := resolveSource(ws, f)
	if err != nil {
		return err
	}
	now, fixed, err := resolveNow(f)
	if err != nil {
		return err
	}
	unlock, err := ws.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	l, err := ws.Scan()
	if err != nil {
		return err
	}
	return fn(&writeCtx{ws: ws, l: l, src: src, now: now, fixed: fixed})
}

func (a *app) observeCmd() *cobra.Command {
	var act actFlags
	var state, contentFile, version string
	cmd := &cobra.Command{
		Use:   "observe <id> (--state S | --content-file F [--version V] | --version V)",
		Short: "Record whether a reference episode's source can still be reached (source_status)",
		Long: `Write or rewrite source_status on a reference episode, the one block rewritten
in place. Either state what was observed (--state available, changed,
unreachable or gone), or give the content as fetched (--content-file) to be
hashed and compared with content.hash; with no hash, --version is compared
with content.version. Nothing else in the episode changes, and nothing is
back-filled.`,
		Args: cobra.ExactArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if state != "" && (contentFile != "" || cmd.Flags().Changed("version")) {
				return usageErr("use --state, or --content-file and/or --version, not both")
			}
			if state == "" && contentFile == "" && !cmd.Flags().Changed("version") {
				return usageErr("one of --state, --content-file or --version is required")
			}
			if state != "" && !slices.Contains(model.States, state) {
				return usageErr("--state must be one of %s", strings.Join(model.States, ", "))
			}
			var content []byte
			if contentFile != "" {
				b, err := a.readBytes(contentFile, "--content-file")
				if err != nil {
					return err
				}
				content = b
			}
			var result *model.Episode
			var changed bool
			var src model.Source
			err := a.writeOnly(act, func(c *writeCtx) error {
				src = c.src
				var err error
				result, changed, err = c.ws.Amend(id, func(ep *model.Episode) error {
					switch ep.Content.Mode {
					case model.ModeRef:
					case model.ModeInline:
						return refusedErr("%s is inline: this workspace is its only home, so there is no source to observe", id)
					default:
						return refusedErr("%s has content mode %s: there is no source to observe", id, ep.Content.Mode)
					}
					s := state
					if s == "" {
						var err error
						if s, err = compare(ep, content, contentFile != "", version, cmd.Flags().Changed("version")); err != nil {
							return err
						}
					}
					now := c.stamp()
					next := &model.SourceStatus{State: s, Verified: now, Detected: now}
					if prev := ep.SourceStatus; prev != nil {
						next.Extra = prev.Extra
						if prev.State == s && prev.Detected != "" {
							next.Detected = prev.Detected
						}
					}
					ep.SourceStatus = next
					return nil
				})
				if errors.Is(err, store.ErrNotFound) {
					return notFoundErr("%v", err)
				}
				return err
			})
			if err != nil {
				return err
			}
			ss := result.SourceStatus
			return a.emit(map[string]any{
				"id": id, "path": store.RelPath(id), "changed": changed, "source": sourceJSON(src),
				"source_status": map[string]any{"state": ss.State, "verified": ss.Verified, "detected": ss.Detected},
			}, func(w io.Writer) {
				fmt.Fprintf(w, "%s source_status: %s (verified %s, detected %s)\n", id, ss.State, ss.Verified, ss.Detected)
			})
		}),
	}
	addActFlags(cmd, &act)
	cmd.Flags().StringVar(&state, "state", "", "the state observed: available, changed, unreachable or gone")
	cmd.Flags().StringVar(&contentFile, "content-file", "", "the content as fetched now, in a file or - for stdin, hashed and compared with content.hash")
	cmd.Flags().StringVar(&version, "version", "", "the source's version now, compared with content.version when there is no hash")
	return cmd
}

// compare computes a state from fetched content: available when the hash
// (or, without a hash, the version) matches, changed when it does not.
func compare(ep *model.Episode, content []byte, haveContent bool, version string, haveVersion bool) (string, error) {
	switch {
	case haveContent && ep.Content.Hash != "":
		match, ok := model.HashMatches(ep.Content.Hash, content)
		if !ok {
			return "", refusedErr("%s's hash %q uses an algorithm this implementation does not compute, so the content cannot be compared; give --state instead", ep.ID, ep.Content.Hash)
		}
		if match {
			return model.StateAvailable, nil
		}
		return model.StateChanged, nil
	case haveVersion && ep.Content.Version != "":
		if version == ep.Content.Version {
			return model.StateAvailable, nil
		}
		return model.StateChanged, nil
	}
	return "", refusedErr("%s carries neither a hash nor a version to compare with; observe it with --state", ep.ID)
}

func (a *app) redactCmd() *cobra.Command {
	var act actFlags
	var fields []string
	var reason string
	cmd := &cobra.Command{
		Use:   "redact <id> --fields <path>[,<path>…] --reason R",
		Short: "Remove fields from an episode and record who removed what, when and why",
		Long: `Redact fields from one episode: each listed path is removed and a redacted
block is written last in the file. Only these paths can be redacted:
` + strings.Join(model.Redactable, ", ") + `.
links is redacted whole. A second redaction adds to the first. It is
irreversible, changes no other episode, and does not remove anything from
git history: every result says so. The reason must not restate what was
removed.`,
		Args: cobra.ExactArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			id := args[0]
			paths := splitFields(fields)
			if len(paths) == 0 {
				return usageErr("--fields is required: the paths to remove (%s)", strings.Join(model.Redactable, ", "))
			}
			if strings.TrimSpace(reason) == "" {
				return usageErr("--reason is required (and must not restate what is removed)")
			}
			for _, p := range paths {
				if !model.IsRedactable(p) {
					return refusedErr("redaction_path_invalid: %q cannot be redacted; the closed set is %s (indexed fields never can be, because sealed segments never change)", p, strings.Join(model.Redactable, ", "))
				}
			}
			var result *model.Episode
			var changed bool
			var src model.Source
			err := a.writeOnly(act, func(c *writeCtx) error {
				src = c.src
				var err error
				result, changed, err = c.ws.Amend(id, func(ep *model.Episode) error {
					for _, p := range paths {
						if !ep.CarriesPath(p) && !ep.Redacted.Lists(p) {
							return refusedErr("%s does not carry %s; redacted.fields never claims a removal that did not happen", id, p)
						}
					}
					union := append([]string(nil), paths...)
					var extra model.Extras
					if ep.Redacted != nil {
						union = append(union, ep.Redacted.Fields...)
						extra = ep.Redacted.Extra
					}
					union = splitFields(union)
					for _, p := range union {
						ep.RemovePath(p)
					}
					ep.Redacted = &model.Redacted{Timestamp: c.stamp(), Reason: reason, Source: c.src, Fields: union, Extra: extra}
					return nil
				})
				if errors.Is(err, store.ErrNotFound) {
					return notFoundErr("%v", err)
				}
				return err
			})
			if err != nil {
				return err
			}
			return a.emit(map[string]any{
				"id": id, "path": store.RelPath(id), "changed": changed, "source": sourceJSON(src),
				"redacted": map[string]any{"timestamp": result.Redacted.Timestamp, "reason": result.Redacted.Reason, "fields": result.Redacted.Fields},
				"notice":   GitNotice,
			}, func(w io.Writer) {
				fmt.Fprintf(w, "Redacted %s from %s\n%s\n", strings.Join(result.Redacted.Fields, ", "), id, GitNotice)
			})
		}),
	}
	addActFlags(cmd, &act)
	cmd.Flags().StringArrayVar(&fields, "fields", nil, "comma-separated paths to remove (repeatable)")
	cmd.Flags().StringVar(&reason, "reason", "", "why; must not restate what was removed")
	return cmd
}
