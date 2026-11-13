package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/query"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

// writeCtx is what a writing verb has under the lock.
type writeCtx struct {
	ws    *store.Workspace
	l     *store.Listing
	src   model.Source
	now   time.Time
	fixed bool
}

func (c *writeCtx) mint() string { return mint(c.now, c.fixed) }

func (c *writeCtx) stamp() string { return temporal.FormatTimestamp(c.now) }

// write runs a writing verb: attribution and time are resolved, then the
// workspace lock is held from minting until the index upsert completes.
func (a *app) write(f actFlags, fn func(c *writeCtx) (map[string]any, func(io.Writer), error)) error {
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
	out, text, err := fn(&writeCtx{ws: ws, l: l, src: src, now: now, fixed: fixed})
	if err != nil {
		return err
	}
	out["source"] = sourceJSON(src)
	return a.emit(out, text)
}

// episodeFlags are the fields every recording verb may set.
type episodeFlags struct {
	kind, standpoint     string
	participants         []string
	place, label         string
	container            string
	ordinal              int
	links                []string
	text, textFile       string
	mediaType, language  string
	derivation, zone     string
	occ                  occurredFlags
	cmd                  *cobra.Command
	defaultKind          string
	withContainerOrdinal bool
}

func addEpisodeFlags(cmd *cobra.Command, f *episodeFlags, kindDefault string) {
	f.cmd = cmd
	f.defaultKind = kindDefault
	kindHelp := "what sort of happening: a short lowercase word (note, call, email, event, transcript, …)"
	if kindDefault != "" {
		kindHelp += " (default " + kindDefault + ")"
	}
	cmd.Flags().StringVar(&f.kind, "kind", "", kindHelp)
	cmd.Flags().StringVar(&f.standpoint, "standpoint", "", "whose act, utterance or record this is: a URI where known, else a name (required)")
	cmd.Flags().StringArrayVar(&f.participants, "participant", nil, "a reference to someone present or addressed (repeatable)")
	cmd.Flags().StringVar(&f.place, "place", "", "a reference to where it happened")
	cmd.Flags().StringVar(&f.label, "label", "", "a short name captured with it, such as a subject or title; never a summary")
	cmd.Flags().StringArrayVar(&f.links, "link", nil, "a typed link rel=href to an earlier episode or a URI (repeatable), e.g. via=epi_…")
	cmd.Flags().StringVar(&f.language, "language", "", "BCP 47 tag of the content, only when the source states it or the harness set it; never detected")
}

func addContainerFlags(cmd *cobra.Command, f *episodeFlags) {
	f.withContainerOrdinal = true
	cmd.Flags().StringVar(&f.container, "container", "", "the earlier episode this one is part of (an email for its attachment, a thread for its message)")
	cmd.Flags().IntVar(&f.ordinal, "ordinal", 0, "position within --container, from 1, when order is meaningful")
}

func addInlineFlags(cmd *cobra.Command, f *episodeFlags) {
	cmd.Flags().StringVar(&f.text, "text", "", "the text, which this workspace is the only home of")
	cmd.Flags().StringVar(&f.textFile, "text-file", "", "read the text from a file, or - for piped stdin")
	cmd.Flags().StringVar(&f.mediaType, "media-type", model.DefaultMediaType, "media type of the text")
	cmd.Flags().StringVar(&f.derivation, "derivation", model.DerivationVerbatim, "how the text came to exist: verbatim, transcribed, ocr or rendered")
	cmd.Flags().StringVar(&f.zone, "zone", "", "IANA zone written when the time is now (default: $"+EnvZone+", then defaults.zone; never the host's)")
}

// base builds the fields every episode shares, refusing what is missing.
func (f *episodeFlags) base(c *writeCtx, id string, occ model.Occurred) (*model.Episode, error) {
	kind := firstNonEmpty(f.kind, f.defaultKind)
	if kind == "" {
		return nil, usageErr("--kind is required: what sort of happening this is (note, call, email, …)")
	}
	if strings.TrimSpace(f.standpoint) == "" {
		return nil, usageErr("--standpoint is required: whose act, utterance or record this is; it is never defaulted from --author")
	}
	links, err := parseLinks(f.links)
	if err != nil {
		return nil, err
	}
	ep := &model.Episode{
		ID: id, Type: model.TypeEpisode, Kind: kind, Occurred: occ,
		Standpoint:   f.standpoint,
		Participants: model.SortParticipants(f.participants),
		Place:        f.place, Label: f.label,
		Links:     model.SortLinks(links),
		Source:    c.src,
		Timestamp: c.stamp(),
	}
	if f.withContainerOrdinal {
		ep.Container = f.container
		if f.cmd.Flags().Changed("ordinal") {
			if f.container == "" {
				return nil, usageErr("--ordinal is a position within --container; give --container too")
			}
			o := f.ordinal
			ep.Ordinal = &o
		}
	}
	return ep, nil
}

func canonicalLanguage(tag string) string {
	if tag != "" && model.LanguageProblem(tag) == nil {
		return model.CanonicalLanguageCase(tag)
	}
	return tag
}

// inline builds inline content from the flags.
func (a *app) inline(f *episodeFlags) (model.Content, error) {
	text, ok, err := a.readText(f.text, f.textFile, f.cmd.Flags().Changed("text"), "text")
	if err != nil {
		return model.Content{}, err
	}
	if !ok {
		return model.Content{}, usageErr("--text or --text-file is required: the text this episode holds")
	}
	return model.Content{Mode: model.ModeInline, MediaType: f.mediaType, Derivation: f.derivation, Language: canonicalLanguage(f.language), Text: &text}, nil
}

// create checks an episode against every rule and the workspace, then
// writes it.
func (c *writeCtx) create(ep *model.Episode) (string, error) {
	if err := checkProblems(ep); err != nil {
		return "", err
	}
	if err := checkWorkspace(c.ws, c.l, ep); err != nil {
		return "", err
	}
	return c.ws.Create(c.l, ep)
}

func written(ep *model.Episode, rel string) map[string]any {
	return map[string]any{"id": ep.ID, "path": rel, "episode": episodeJSON(ep)}
}

func (a *app) recordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record an inline or reference episode",
	}
	cmd.AddCommand(a.recordInlineCmd(), a.recordRefCmd())
	return cmd
}

func (a *app) recordInlineCmd() *cobra.Command {
	var f episodeFlags
	var act actFlags
	var uid string
	cmd := &cobra.Command{
		Use:   "inline --kind K --standpoint S (--text T | --text-file F)",
		Short: "Record an episode whose content has no other home: a note, a transcript the harness made",
		Args:  cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			if uid != "" {
				return refusedErr("uid_on_inline: an inline episode must not carry uid; its identity is its episode id")
			}
			return a.write(act, func(c *writeCtx) (map[string]any, func(io.Writer), error) {
				zone, err := zoneFor(c.ws, f.zone)
				if err != nil {
					return nil, nil, err
				}
				occ, err := buildOccurred(f.occ, true, c.now, zone)
				if err != nil {
					return nil, nil, err
				}
				ep, err := f.base(c, c.mint(), occ)
				if err != nil {
					return nil, nil, err
				}
				if ep.Content, err = a.inline(&f); err != nil {
					return nil, nil, err
				}
				rel, err := c.create(ep)
				if err != nil {
					return nil, nil, err
				}
				return written(ep, rel), func(w io.Writer) { fmt.Fprintf(w, "Recorded %s\n  %s\n", ep.ID, rel) }, nil
			})
		}),
	}
	addEpisodeFlags(cmd, &f, "")
	addContainerFlags(cmd, &f)
	addInlineFlags(cmd, &f)
	addOccurredFlags(cmd, &f.occ, false)
	addActFlags(cmd, &act)
	cmd.Flags().StringVar(&uid, "uid", "", "refused: an inline episode carries no uid")
	_ = cmd.Flags().MarkHidden("uid")
	return cmd
}

func (a *app) recordRefCmd() *cobra.Command {
	var f episodeFlags
	var act actFlags
	var ref, uid, messageID, mediaType, version, hash, hashFile, text string
	var allowDup bool
	cmd := &cobra.Command{
		Use:   "ref --kind K --standpoint S --ref R --occurred V --basis B",
		Short: "Record a trace of content that lives elsewhere: an email, a calendar arrangement, a file",
		Long: `Record a reference episode: a pointer to content that has a home elsewhere,
with enough to stand as a record after the content is gone. It never holds
the content's text. For a calendar event, --occurred is when the event was
arranged (its creation time, --basis stated), never its scheduled time.`,
		Args: cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("text") {
				return refusedErr("text_on_ref: a reference episode must not carry text; it is a trace of the content, not a copy. Whatever cites it carries its own quote")
			}
			if ref == "" {
				return usageErr("--ref is required: whatever can fetch the content (a provider id or URI)")
			}
			if uid != "" && messageID != "" {
				return usageErr("use either --uid or --message-id, not both")
			}
			if hash != "" && hashFile != "" {
				return usageErr("use either --hash or --hash-file, not both")
			}
			if messageID != "" {
				m, err := model.MidFromMessageID(messageID)
				if err != nil {
					return usageErr("--message-id: %v", err)
				}
				uid = m
			}
			if hashFile != "" {
				b, err := a.readBytes(hashFile, "--hash-file")
				if err != nil {
					return err
				}
				hash = model.HashBytes(b)
			} else if hash != "" {
				if err := model.HashWriteProblem(hash); err != nil {
					return refusedErr("hash_invalid: %v", err)
				}
			}
			return a.write(act, func(c *writeCtx) (map[string]any, func(io.Writer), error) {
				occ, err := buildOccurred(f.occ, false, c.now, "")
				if err != nil {
					return nil, nil, err
				}
				ep, err := f.base(c, c.mint(), occ)
				if err != nil {
					return nil, nil, err
				}
				ep.Content = model.Content{Mode: model.ModeRef, Ref: ref, UID: uid, Version: version, Hash: hash, MediaType: mediaType, Language: canonicalLanguage(f.language)}
				if err := checkProblems(ep); err != nil {
					return nil, nil, err
				}
				if uid != "" && !allowDup {
					if ids := query.Duplicates(c.ws.LoadAll(c.l), ep.Standpoint, mediaType, uid); len(ids) > 0 {
						return nil, nil, dupErr("duplicate: %s already record%s uid %s (%s) from standpoint %s; cite the existing episode, or pass --allow-duplicate for a distinct occurrence that shares the uid",
							strings.Join(ids, ", "), map[bool]string{true: "s", false: ""}[len(ids) == 1], uid, mediaType, ep.Standpoint)
					}
				}
				rel, err := c.create(ep)
				if err != nil {
					return nil, nil, err
				}
				return written(ep, rel), func(w io.Writer) { fmt.Fprintf(w, "Recorded %s\n  %s\n", ep.ID, rel) }, nil
			})
		}),
	}
	addEpisodeFlags(cmd, &f, "")
	addContainerFlags(cmd, &f)
	addOccurredFlags(cmd, &f.occ, true)
	addActFlags(cmd, &act)
	cmd.Flags().StringVar(&ref, "ref", "", "where to fetch the content: a URI or an id meaningful to whatever can fetch it (never an episode id; use --link)")
	cmd.Flags().StringVar(&uid, "uid", "", "the content's own identifier, verbatim (an iCalendar UID); requires --media-type")
	cmd.Flags().StringVar(&messageID, "message-id", "", "a mail Message-ID header value, written as a mid: URI")
	cmd.Flags().StringVar(&mediaType, "media-type", "", "media type of the content (required with --uid or --message-id)")
	cmd.Flags().StringVar(&version, "version", "", "the source's own version id (a calendar SEQUENCE or ETag, a file revision)")
	cmd.Flags().StringVar(&hash, "hash", "", "sha256:<64 lowercase hex> of the content as fetched, CRLF normalised to LF")
	cmd.Flags().StringVar(&hashFile, "hash-file", "", "compute the hash from the content as fetched, in a file or - for stdin")
	cmd.Flags().BoolVar(&allowDup, "allow-duplicate", false, "record even though a reference episode from this standpoint already has this uid (a distinct occurrence sharing it)")
	cmd.Flags().StringVar(&text, "text", "", "refused: a reference episode carries no text")
	_ = cmd.Flags().MarkHidden("text")
	return cmd
}

func (a *app) conversationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conversation",
		Short: "Record a conversation: a container and its turns",
	}
	cmd.AddCommand(a.conversationStartCmd(), a.conversationTurnCmd())
	return cmd
}

func (a *app) conversationStartCmd() *cobra.Command {
	var f episodeFlags
	var act actFlags
	var containerKind, containerStandpoint string
	cmd := &cobra.Command{
		Use:   "start --standpoint S (--text T | --text-file F)",
		Short: "Write a conversation container and its first turn together, under one lock",
		Args:  cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			return a.write(act, func(c *writeCtx) (map[string]any, func(io.Writer), error) {
				zone, err := zoneFor(c.ws, f.zone)
				if err != nil {
					return nil, nil, err
				}
				occ, err := buildOccurred(f.occ, true, c.now, zone)
				if err != nil {
					return nil, nil, err
				}
				cs := firstNonEmpty(containerStandpoint, c.src.Author)
				if cs == "" {
					return nil, nil, usageErr("--container-standpoint is required when the source has no author: whose conversation the container records")
				}
				containerID := c.mint()
				turnID := c.mint()
				turn, err := f.base(c, turnID, occ)
				if err != nil {
					return nil, nil, err
				}
				if turn.Content, err = a.inline(&f); err != nil {
					return nil, nil, err
				}
				one := 1
				turn.Container, turn.Ordinal = containerID, &one
				container := &model.Episode{
					ID: containerID, Type: model.TypeEpisode, Kind: firstNonEmpty(containerKind, "conversation"),
					Occurred:   model.Occurred{Start: occ.Start, TZ: occ.TZ, Basis: occ.Basis},
					Standpoint: cs,
					Content:    model.Content{Mode: model.ModeNone},
					Source:     c.src, Timestamp: c.stamp(),
				}
				if err := checkProblems(container); err != nil {
					return nil, nil, err
				}
				if err := checkProblems(turn); err != nil {
					return nil, nil, err
				}
				for _, link := range turn.Links {
					if model.IsID(link.Href) {
						if _, err := resolveEarlier(c.ws, link.Href, containerID, "link target"); err != nil {
							return nil, nil, err
						}
					}
				}
				crel, err := c.ws.Create(c.l, container)
				if err != nil {
					return nil, nil, err
				}
				trel, err := c.ws.Create(c.l, turn)
				if err != nil {
					return nil, nil, err
				}
				out := map[string]any{
					"container": map[string]any{"id": containerID, "path": crel},
					"turn":      map[string]any{"id": turnID, "path": trel, "ordinal": 1},
				}
				text := func(w io.Writer) {
					fmt.Fprintf(w, "Started %s\n  %s\nTurn 1 %s\n  %s\n", containerID, crel, turnID, trel)
				}
				return out, text, nil
			})
		}),
	}
	addEpisodeFlags(cmd, &f, "message")
	cmd.Flags().Lookup("kind").Usage = "the first turn's kind (default message)"
	addInlineFlags(cmd, &f)
	addOccurredFlags(cmd, &f.occ, false)
	addActFlags(cmd, &act)
	cmd.Flags().StringVar(&containerKind, "container-kind", "", "the container's kind (default conversation)")
	cmd.Flags().StringVar(&containerStandpoint, "container-standpoint", "", "whose conversation the container records (default: the resolved source.author)")
	return cmd
}

func (a *app) conversationTurnCmd() *cobra.Command {
	var f episodeFlags
	var act actFlags
	cmd := &cobra.Command{
		Use:   "turn <container> --standpoint S (--text T | --text-file F)",
		Short: "Append a turn to a conversation, assigned the next ordinal under the lock",
		Args:  cobra.ExactArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			container := args[0]
			return a.write(act, func(c *writeCtx) (map[string]any, func(io.Writer), error) {
				if !c.ws.Exists(container) {
					return nil, nil, notFoundErr("container %s does not exist in this workspace", container)
				}
				zone, err := zoneFor(c.ws, f.zone)
				if err != nil {
					return nil, nil, err
				}
				occ, err := buildOccurred(f.occ, true, c.now, zone)
				if err != nil {
					return nil, nil, err
				}
				f.container = container
				ep, err := f.base(c, c.mint(), occ)
				if err != nil {
					return nil, nil, err
				}
				if ep.Ordinal == nil {
					n := query.NextOrdinal(query.Children(childEntries(c.ws, c.l, container), container))
					ep.Ordinal = &n
				}
				if ep.Content, err = a.inline(&f); err != nil {
					return nil, nil, err
				}
				rel, err := c.create(ep)
				if err != nil {
					return nil, nil, err
				}
				out := written(ep, rel)
				out["ordinal"] = *ep.Ordinal
				return out, func(w io.Writer) { fmt.Fprintf(w, "Turn %d %s\n  %s\n", *ep.Ordinal, ep.ID, rel) }, nil
			})
		}),
	}
	addEpisodeFlags(cmd, &f, "message")
	f.withContainerOrdinal = true
	cmd.Flags().IntVar(&f.ordinal, "ordinal", 0, "the turn's position (default: one more than the highest so far)")
	addInlineFlags(cmd, &f)
	addOccurredFlags(cmd, &f.occ, false)
	addActFlags(cmd, &act)
	return cmd
}

func (a *app) actCmd() *cobra.Command {
	var f episodeFlags
	var act actFlags
	var via string
	cmd := &cobra.Command{
		Use:   "act --occurred V --basis stated|inferred [--via epi_T]",
		Short: "Record a self-attested act: something the author did that left no record, on their explicit request",
		Long: `Record a self-attested act. Its standpoint is the resolved source.author, its
content is {mode: none}, and its time is when the act took place. Record one
only when the person explicitly asks for it, never from what is inferred.
When a harness records it, --via must name the episode (usually the turn)
in which the person asked. An act still going on is recorded with its start
and no --end.`,
		Args: cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			if f.language != "" {
				return refusedErr("language_on_none: a self-attested act has content mode none, which carries no language")
			}
			return a.write(act, func(c *writeCtx) (map[string]any, func(io.Writer), error) {
				if c.src.Author == "" {
					return nil, nil, refusedErr("source: a self-attested act needs source.author, whose act it is: pass --author, set EPISODES_AUTHOR, or set defaults.source.author")
				}
				if f.standpoint != "" && f.standpoint != c.src.Author {
					return nil, nil, refusedErr("act_standpoint_mismatch: a self-attested act is the author's own (%s); %s's act is recorded in the episode in which they say it", c.src.Author, f.standpoint)
				}
				if c.src.Harness != "" && via == "" {
					return nil, nil, refusedErr("via: a harness records an act only on the person's explicit attestation; record the attesting turn first and name it with --via")
				}
				if f.occ.basis == model.BasisRecorded {
					return nil, nil, refusedErr("basis: an act's time is stated (the person gave it) or inferred (worked out), not recorded")
				}
				occ, err := buildOccurred(f.occ, false, c.now, "")
				if err != nil {
					return nil, nil, err
				}
				f.standpoint = c.src.Author
				if via != "" {
					f.links = append(f.links, "via="+via)
				}
				ep, err := f.base(c, c.mint(), occ)
				if err != nil {
					return nil, nil, err
				}
				ep.Content = model.Content{Mode: model.ModeNone}
				if err := checkProblems(ep); err != nil {
					return nil, nil, err
				}
				if via != "" {
					t, err := resolveEarlier(c.ws, via, ep.ID, "via episode")
					if err != nil {
						return nil, nil, err
					}
					if t.Standpoint != ep.Standpoint {
						return nil, nil, refusedErr("act_via_standpoint: via %s has standpoint %s; an act is attested by its own standpoint (%s)", via, t.Standpoint, ep.Standpoint)
					}
				}
				rel, err := c.create(ep)
				if err != nil {
					return nil, nil, err
				}
				return written(ep, rel), func(w io.Writer) { fmt.Fprintf(w, "Recorded act %s\n  %s\n", ep.ID, rel) }, nil
			})
		}),
	}
	addEpisodeFlags(cmd, &f, "activity")
	cmd.Flags().Lookup("standpoint").Usage = "refused unless it equals the resolved source.author (default: that author)"
	addOccurredFlags(cmd, &f.occ, true)
	addActFlags(cmd, &act)
	cmd.Flags().StringVar(&via, "via", "", "the episode in which the act was attested, such as the turn asking for it (required when a harness records it)")
	return cmd
}
