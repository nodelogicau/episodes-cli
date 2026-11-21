package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nodelogicau/episodes-cli/internal/apperr"
	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/query"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

// importChunkSize is how many lines one lock writes (design D2 of
// bulk-import). Tests lower it to exercise several chunks.
var importChunkSize = 1000

// importBeforeChunk, when set, runs before each phase-2 chunk takes the
// lock: a test's stand-in for a concurrent writer.
var importBeforeChunk func(chunk int)

// Line outcomes.
const (
	lineWritten = "written"
	lineSkipped = "skipped"
	lineFailed  = "failed"
)

// importLine is one input line, decoded, and its outcome.
type importLine struct {
	n   int    // physical line number, from 1
	key string // its @key, if any

	ep           *model.Episode
	auto         bool         // ordinal "auto"
	containerKey string       // container given as @<key>
	links        []model.Link // as given, an href possibly @<key>
	deps         []int        // indexes of the lines its @<key>s name

	status, code, message string
	existing              string // skipped: the episode it duplicates
	path                  string
}

func (ln *importLine) fail(code, format string, args ...any) {
	ln.status, ln.code, ln.message = lineFailed, code, fmt.Sprintf(format, args...)
}

func (ln *importLine) failErr(err error) {
	var probs model.Problems
	switch {
	case errors.As(err, &probs):
		ln.fail("refused", "%s", probs.Error())
	case errors.Is(err, store.ErrSealedMonth):
		ln.fail("refused", "%v", err)
	default:
		ee := classify(err)
		ln.fail(ee.ErrCode, "%s", ee.Err.Error())
	}
}

// id is the episode a reference to this line names: the one it duplicates
// when it was skipped, else its own.
func (ln *importLine) id() string {
	if ln.status == lineSkipped {
		return ln.existing
	}
	return ln.ep.ID
}

// refusedKeys are the keys the binary supplies, or that no new episode
// carries: import is not a copy tool (design D1).
var refusedKeys = map[string]string{
	"id":            "import mints every id",
	"type":          "the binary writes type",
	"source":        "source comes from --author, --harness and --model (or their environment and defaults), once for the batch",
	"timestamp":     "the binary writes one timestamp for the batch",
	"source_status": "a new episode carries no observation of its source; observe one after it is written",
	"redacted":      "a new episode carries no redaction",
}

// jsonNode reads one JSON value as a YAML node, keeping key order and
// refusing a repeated key, so a line decodes by the same code as a file.
func jsonNode(dec *json.Decoder) (*yaml.Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			seen := map[string]bool{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				if seen[k] {
					return nil, fmt.Errorf("duplicate key %q", k)
				}
				seen[k] = true
				v, err := jsonNode(dec)
				if err != nil {
					return nil, err
				}
				m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, v)
			}
			_, err := dec.Token()
			return m, err
		case '[':
			s := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for dec.More() {
				v, err := jsonNode(dec)
				if err != nil {
					return nil, err
				}
				s.Content = append(s.Content, v)
			}
			_, err := dec.Token()
			return s, err
		}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: t}, nil
	case json.Number:
		tag := "!!float"
		if _, err := strconv.Atoi(t.String()); err == nil {
			tag = "!!int"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: t.String()}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(t)}, nil
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	return nil, fmt.Errorf("unexpected %v", tok)
}

// decodeImportLine reads one line strictly (design D1): one JSON object with
// the episode's own fields, by the names and shapes of the YAML file, plus
// @key and an ordinal of "auto". It refuses the keys the binary supplies and
// any key the format does not define.
func decodeImportLine(n int, raw []byte) *importLine {
	ln := &importLine{n: n}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	root, err := jsonNode(dec)
	if err == nil {
		if _, terr := dec.Token(); terr != io.EOF {
			err = errors.New("more than one JSON value on the line")
		}
	}
	if err != nil {
		ln.fail("invalid", "line %d is not one JSON object: %v", n, err)
		return ln
	}
	if root.Kind != yaml.MappingNode {
		ln.fail("invalid", "line %d is not a JSON object", n)
		return ln
	}
	var refused []string
	kept := root.Content[:0:0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i].Value, root.Content[i+1]
		switch {
		case k == "@key":
			if v.Kind != yaml.ScalarNode || v.Tag != "!!str" || strings.TrimSpace(v.Value) == "" {
				ln.fail("refused", "@key must be a non-empty string")
				return ln
			}
			ln.key = v.Value
			continue
		case refusedKeys[k] != "":
			refused = append(refused, fmt.Sprintf("%s (%s)", k, refusedKeys[k]))
			continue
		case k == "ordinal" && v.Kind == yaml.ScalarNode && v.Tag == "!!str" && v.Value == "auto":
			ln.auto = true
			continue
		}
		kept = append(kept, root.Content[i], v)
	}
	if len(refused) > 0 {
		ln.fail("refused", "refused: import is not a copy tool, and a line carries none of id, type, source, timestamp, source_status or redacted; this line has %s", strings.Join(refused, "; "))
		return ln
	}
	root.Content = kept
	ep, probs, err := model.DecodeNode(root)
	if err != nil {
		ln.fail("invalid", "%v", err)
		return ln
	}
	if len(probs) > 0 {
		ln.failErr(model.Problems(probs))
		return ln
	}
	var unknown []string
	for _, x := range []struct {
		at string
		ex model.Extras
	}{{"", ep.Extra}, {"occurred.", ep.Occurred.Extra}, {"content.", ep.Content.Extra}} {
		for _, k := range x.ex.Keys() {
			unknown = append(unknown, x.at+k)
		}
	}
	for i, l := range ep.Links {
		for _, k := range l.Extra.Keys() {
			unknown = append(unknown, fmt.Sprintf("links[%d].%s", i, k))
		}
	}
	if len(unknown) > 0 {
		ln.fail("refused", "refused: %s %s not defined by the format", strings.Join(unknown, ", "), map[bool]string{true: "is", false: "are"}[len(unknown) == 1])
		return ln
	}
	if ep.Content.Mode == model.ModeAttested {
		ln.fail("refused", "refused: a self-attested act (content mode attested) is recorded one at a time with `episodes act`, on the person's explicit request; import refuses it")
		return ln
	}
	if ln.auto && ep.Container == "" {
		ln.fail("refused", "refused: ordinal \"auto\" is the next position within container; give container too")
		return ln
	}
	if k, ok := strings.CutPrefix(ep.Container, "@"); ok {
		ln.containerKey, ep.Container = k, ""
	}
	ln.links, ep.Links = ep.Links, nil
	ln.ep = ep
	return ln
}

// importOccurred is the occurred block as record would write it (D1): a
// supplied time needs its basis and is put in the form a writer writes; one
// left out, or given only as basis recorded, is now in the zone, except on a
// reference episode, whose time is always supplied.
func importOccurred(ep *model.Episode, now time.Time, zone string, notes *[]string) error {
	o := ep.Occurred
	nowForm := o.Start == "" && o.End == "" && o.TZ == "" && (o.Basis == "" || o.Basis == model.BasisRecorded)
	if !ep.Has("occurred") || nowForm {
		if ep.Content.Mode == model.ModeRef {
			return refusedErr("occurred: a reference episode needs occurred with start and basis: when the happening took place, as its source states it or as worked out")
		}
		start, err := temporal.RecordedNow(now, zone)
		if err != nil {
			return err
		}
		ep.Occurred = model.Occurred{Start: start, Basis: model.BasisRecorded}
		return nil
	}
	f := occurredFlags{start: o.Start, end: o.End, tz: o.TZ, basis: o.Basis}
	switch {
	case f.start == "":
		// Left for the episode check, which names the missing field.
		return nil
	case f.basis == "":
		return refusedErr("occurred.basis: a supplied time needs basis (stated or inferred, or recorded if the harness observed it as it happened); the basis is never defaulted for a supplied time")
	}
	occ, err := buildOccurred(f, false, now, zone, notes)
	if err != nil {
		return err
	}
	ep.Occurred = occ
	return nil
}

type dupKey struct{ standpoint, mediaType, uid string }

// batchCheck runs the workspace rules over lines against one listing: in
// phase 1 a snapshot of the whole batch, in phase 2 the fresh listing of
// one chunk (design D2).
type batchCheck struct {
	ws     *store.Workspace
	l      *store.Listing
	lines  []*importLine
	keys   map[string]int
	own    map[string]bool // every id minted for the batch
	onDup  string
	phase1 bool

	uids   *store.UIDSet
	listed map[string]bool
	read   map[string]error
	kids   map[string][]store.Entry // container → children, from viewMin on
	view   bool
	viewAt int

	ords map[string]map[int]string // ordinals this pass assigned
	dups map[dupKey]int            // phase 1: the line first claiming each uid
}

func newBatchCheck(ws *store.Workspace, l *store.Listing, lines []*importLine, keys map[string]int, own map[string]bool, onDup string, phase1 bool) *batchCheck {
	return &batchCheck{ws: ws, l: l, lines: lines, keys: keys, own: own, onDup: onDup, phase1: phase1,
		read: map[string]error{}, ords: map[string]map[int]string{}, dups: map[dupKey]int{}}
}

// earlier is resolveEarlier with each file read once per pass. A batch
// line's id needs no read: line order made it earlier, and phase 2 wrote it
// or is writing it.
func (c *batchCheck) earlier(ref, id, what string) error {
	if c.own[ref] {
		return nil
	}
	err, ok := c.read[ref]
	if !ok {
		_, _, err = c.ws.Read(ref)
		if errors.Is(err, store.ErrNotFound) {
			err = notFoundErr("%s %s does not exist in this workspace", what, ref)
		}
		c.read[ref] = err
	}
	if err != nil {
		return err
	}
	if ref >= id {
		return refusedErr("%s %s was not recorded before the new episode %s; a %s names an earlier episode", what, ref, id, what)
	}
	return nil
}

func (c *batchCheck) onDisk(id string) bool {
	if c.listed == nil {
		c.listed = make(map[string]bool, len(c.l.Files))
		for _, f := range c.l.Files {
			c.listed[f.ID] = true
		}
	}
	return c.listed[id]
}

// children are the container's children in the listing, from an entry
// view built once per pass from the earliest container's month on.
func (c *batchCheck) children(container string) []store.Entry {
	if !c.onDisk(container) {
		return nil
	}
	m, ok := store.IDMonth(container)
	if !ok {
		m = 0
	}
	if !c.view || m < c.viewAt {
		c.kids, c.view, c.viewAt = map[string][]store.Entry{}, true, m
		_ = c.ws.EachEntry(c.l, m, m > 0, func(e store.Entry, _ store.FileRef) error {
			if e.Container != "" {
				c.kids[e.Container] = append(c.kids[e.Container], e)
			}
			return nil
		})
	}
	return c.kids[container]
}

// resolve writes the line's @<key>s as the ids of the lines they name.
func (c *batchCheck) resolve(ln *importLine) {
	if ln.containerKey != "" {
		ln.ep.Container = c.lines[c.keys[ln.containerKey]].id()
	}
	links := make([]model.Link, len(ln.links))
	for i, l := range ln.links {
		if k, ok := strings.CutPrefix(l.Href, "@"); ok {
			if j, ok := c.keys[k]; ok {
				l.Href = c.lines[j].id()
			}
		}
		links[i] = l
	}
	ln.ep.Links = model.SortLinks(links)
	if ln.auto {
		ln.ep.Ordinal = nil
	}
}

// duplicate returns the episode the line repeats, if any: an earlier line
// of the batch, in phase 1, or a file other than the batch's own.
func (c *batchCheck) duplicate(i int) (string, []string) {
	ep := c.lines[i].ep
	k := dupKey{ep.Standpoint, strings.ToLower(ep.Content.MediaType), ep.Content.UID}
	if c.phase1 {
		if j, ok := c.dups[k]; ok {
			return c.lines[j].id(), []string{c.lines[j].id()}
		}
	}
	if c.uids == nil {
		c.uids = c.ws.UIDSet(c.l)
	}
	var cands []store.Loaded
	for _, f := range c.uids.Candidates(ep.Standpoint, ep.Content.MediaType, ep.Content.UID) {
		if c.own[f.ID] {
			continue
		}
		ld := c.ws.LoadFile(f)
		if ld.Err == nil && ld.Episode != nil && ld.Episode.Content.UID != ep.Content.UID {
			c.ws.ForgetUID(f.ID)
		}
		cands = append(cands, ld)
	}
	if ids := query.Duplicates(cands, ep.Standpoint, ep.Content.MediaType, ep.Content.UID); len(ids) > 0 {
		return ids[0], ids
	}
	return "", nil
}

// check applies every rule to line i, as record would to one episode, and
// sets its outcome: skipped as a duplicate, failed, or left to write.
func (c *batchCheck) check(i int) {
	ln := c.lines[i]
	for _, d := range ln.deps {
		if dep := c.lines[d]; dep.status == lineFailed {
			ln.fail("dependency_failed", "line %d names @%s (line %d), which failed", ln.n, dep.key, dep.n)
			return
		}
	}
	c.resolve(ln)
	ep := ln.ep
	if err := checkProblems(ep); err != nil {
		ln.failErr(err)
		return
	}
	ct := ep.Content
	if ct.Mode == model.ModeRef && ct.UID != "" && c.onDup != "allow" {
		if existing, all := c.duplicate(i); existing != "" {
			if c.onDup == "skip" {
				ln.status, ln.existing, ln.code = lineSkipped, existing, "duplicate"
				ln.message = fmt.Sprintf("%s already records uid %s (%s) from standpoint %s", existing, ct.UID, ct.MediaType, ep.Standpoint)
				return
			}
			ln.fail("duplicate", "duplicate: %s already record%s uid %s (%s) from standpoint %s; drop the line, pass --on-duplicate skip to skip lines already recorded, or --on-duplicate allow for distinct occurrences that share the uid",
				strings.Join(all, ", "), map[bool]string{true: "s", false: ""}[len(all) == 1], ct.UID, ct.MediaType, ep.Standpoint)
			return
		}
	}
	if ep.Container != "" {
		if err := c.earlier(ep.Container, ep.ID, "container"); err != nil {
			ln.failErr(err)
			return
		}
		taken := c.ords[ep.Container]
		kids := c.children(ep.Container)
		if ln.auto {
			n := query.NextOrdinal(kids)
			for o := range taken {
				if o >= n {
					n = o + 1
				}
			}
			ep.Ordinal = &n
		} else if ep.Ordinal != nil {
			holder, ok := query.HolderOf(kids, *ep.Ordinal)
			if !ok {
				holder, ok = taken[*ep.Ordinal]
			}
			if ok {
				ln.fail("duplicate", "duplicate: %s already holds ordinal %d in container %s", holder, *ep.Ordinal, ep.Container)
				return
			}
		}
	}
	for _, link := range ep.Links {
		if model.IsID(link.Href) {
			if err := c.earlier(link.Href, ep.ID, "link target"); err != nil {
				ln.failErr(err)
				return
			}
		}
	}
	if ep.Container != "" && ep.Ordinal != nil {
		if c.ords[ep.Container] == nil {
			c.ords[ep.Container] = map[int]string{}
		}
		c.ords[ep.Container][*ep.Ordinal] = ep.ID
	}
	if c.phase1 && ct.Mode == model.ModeRef && ct.UID != "" {
		k := dupKey{ep.Standpoint, strings.ToLower(ct.MediaType), ct.UID}
		if _, ok := c.dups[k]; !ok {
			c.dups[k] = i
		}
	}
}

// settle fails, until nothing changes, every line naming a failed line
// (dependency_failed) and every container no line still to be written
// names (design D2's container rule).
func settle(lines []*importLine) {
	for changed := true; changed; {
		changed = false
		named := map[int]bool{}
		for _, ln := range lines {
			if ln.status != "" {
				continue
			}
			for _, d := range ln.deps {
				if lines[d].status == lineFailed {
					ln.fail("dependency_failed", "line %d names @%s (line %d), which failed", ln.n, lines[d].key, lines[d].n)
					changed = true
					break
				}
			}
		}
		for _, ln := range lines {
			if ln.status == "" && ln.containerKey != "" {
				for _, d := range ln.deps {
					if lines[d].key == ln.containerKey {
						named[d] = true
					}
				}
			}
		}
		for i, ln := range lines {
			if ln.status == "" && ln.ep.Content.Mode == model.ModeNone && !named[i] {
				what := "no later line names it as container"
				if ln.key == "" {
					what = "it has no @key, so no later line can name it as container"
				}
				ln.fail("refused", "refused: line %d is a container (content mode none) with no children in the batch: %s; give its children as later lines with container \"@<its key>\"", ln.n, what)
				changed = true
			}
		}
	}
}

func (a *app) importCmd() *cobra.Command {
	var act actFlags
	var zone, onError, onDup string
	var ndjson bool
	cmd := &cobra.Command{
		Use:   "import [<file>|-]",
		Short: "Create many episodes from NDJSON, one line per episode, checked together and written in chunks",
		Long: `Create episodes from NDJSON: one JSON object per line, each an episode to
create, with the episode's own fields under the names and shapes of the YAML
file (kind, standpoint, occurred, participants, place, label, container,
ordinal, links, content). The binary mints each id, in line order, and
supplies type, source (from --author, --harness and --model, their
environment variables or defaults.source, once for the batch) and one
timestamp for the batch. A line carrying id, type, source, timestamp,
source_status, redacted or a key the format does not define is refused:
import is not a copy tool. Every rule record applies to one episode applies
to each line, and a self-attested act (content mode attested) is refused.

"@key": "<name>" names a line within the batch. A later line may give
"@<name>" as its container or as a link href, for an email and its
attachments or a conversation and its turns. "ordinal": "auto" is one more
than the highest ordinal the container holds, in the workspace and earlier
in the batch. A container (content mode none) must be named by a later line.
Without occurred, or with only basis recorded, an inline line's time is now
(in --zone, $` + EnvZone + ` or defaults.zone); a reference line always
gives occurred, with its basis.

Every line is checked before any is written. --on-error stop (the default)
writes nothing if any line fails, and lists every failing line; skip writes
the lines that pass, failing those that name a failed line
(dependency_failed). A repeated (standpoint, media_type, uid), in the
workspace or in the batch, is refused by default; --on-duplicate skip
reports it skipped with the existing id, which makes re-running a connector
safe, and allow writes it.

Lines are written in chunks of 1,000, each under the lock with its rules
checked again and the index updated once, so another writer waits at most
one chunk. A chunk written stays written if a later one fails. The result
names every line's outcome (written with id and path, skipped with the
existing id, or failed with a code): one object with --json, one line each
and a summary with --ndjson. It exits 0 when no line failed, 2 when any did,
and 1 on an I/O failure. Split batches far beyond 10,000 lines: every line
is held in memory until the check is done.`,
		Args: cobra.MaximumNArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			if ndjson && a.jsonOut {
				return usageErr("use either --json or --ndjson, not both")
			}
			if onError != "stop" && onError != "skip" {
				return usageErr("--on-error must be stop or skip")
			}
			if onDup != "refuse" && onDup != "skip" && onDup != "allow" {
				return usageErr("--on-duplicate must be refuse, skip or allow")
			}
			file := "-"
			if len(args) == 1 {
				file = args[0]
			}
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			src, err := resolveSource(ws, act)
			if err != nil {
				return err
			}
			now, fixed, err := resolveNow(act)
			if err != nil {
				return err
			}
			z, err := zoneFor(ws, zone)
			if err != nil {
				return err
			}
			in, closeIn, err := a.importInput(file)
			if err != nil {
				return err
			}
			lines, err := readImportLines(in)
			closeIn()
			if err != nil {
				return err
			}
			imp := &importer{a: a, ws: ws, src: src, now: now, fixed: fixed, zone: z, onError: onError, onDup: onDup, lines: lines}
			ioErr := imp.run()
			if err := a.importResult(lines, src, ndjson); err != nil {
				return err
			}
			if ioErr != nil {
				return ioErr
			}
			for _, ln := range lines {
				if ln.status == lineFailed {
					return &ExitError{Code: apperr.ExitUsage, ErrCode: "refused", Err: errQuiet}
				}
			}
			return nil
		}),
	}
	addActFlags(cmd, &act)
	cmd.Flags().StringVar(&zone, "zone", "", "IANA zone written when a line's time is now (default: $"+EnvZone+", then defaults.zone; never the host's)")
	cmd.Flags().StringVar(&onError, "on-error", "stop", "stop: write nothing if any line fails; skip: write the lines that pass")
	cmd.Flags().StringVar(&onDup, "on-duplicate", "refuse", "a repeated (standpoint, media_type, uid): refuse (fail the line), skip (report it skipped with the existing id) or allow (write it)")
	cmd.Flags().BoolVar(&ndjson, "ndjson", false, "write one JSON object per line's outcome, then a summary object, instead of --json's single object")
	return cmd
}

// importInput opens the file argument, - meaning piped stdin.
func (a *app) importInput(file string) (io.Reader, func(), error) {
	if file == "-" {
		if a.stdinIsTerminal != nil && a.stdinIsTerminal() {
			return nil, nil, usageErr("import reads NDJSON from a file or piped stdin; refusing to read from a terminal")
		}
		return a.stdin, func() {}, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, usageErr("import: %v", err)
	}
	return f, func() { _ = f.Close() }, nil
}

// readImportLines decodes every non-blank line, numbering lines as the input
// does.
func readImportLines(r io.Reader) ([]*importLine, error) {
	br := bufio.NewReaderSize(r, 1<<16)
	var out []*importLine
	for n := 1; ; n++ {
		raw, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(raw)) > 0 {
			out = append(out, decodeImportLine(n, raw))
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

type importer struct {
	a              *app
	ws             *store.Workspace
	src            model.Source
	now            time.Time
	fixed          bool
	zone           string
	onError, onDup string
	lines          []*importLine
	keys           map[string]int
	own            map[string]bool
	stamp          string
	notes          []string
}

// mintBatch gives every line an id in line order. A fixed clock (--now) gives
// each line its own millisecond, ending at now, so ids stay ordered however
// long the batch.
func (imp *importer) mintBatch() {
	n := len(imp.lines)
	for i, ln := range imp.lines {
		if ln.ep == nil {
			continue
		}
		if imp.fixed {
			ln.ep.ID = model.NewIDAt(imp.now.Add(-time.Duration(n-1-i) * time.Millisecond))
		} else {
			ln.ep.ID = model.NewID()
		}
		imp.own[ln.ep.ID] = true
	}
}

// prepare fills in what the binary supplies and what record defaults, and
// resolves every @key reference to a line index (design D1, D2).
func (imp *importer) prepare() {
	imp.keys = map[string]int{}
	for i, ln := range imp.lines {
		if ln.status != "" || ln.key == "" {
			continue
		}
		if j, ok := imp.keys[ln.key]; ok {
			ln.fail("refused", "refused: @key %q is already defined on line %d; a key names one line", ln.key, imp.lines[j].n)
			continue
		}
		imp.keys[ln.key] = i
	}
	for i, ln := range imp.lines {
		if ln.status != "" {
			continue
		}
		ep := ln.ep
		ep.Type, ep.Source, ep.Timestamp = model.TypeEpisode, imp.src, imp.stamp
		if ep.Content.Mode == model.ModeInline {
			if ep.Content.MediaType == "" {
				ep.Content.MediaType = model.DefaultMediaType
			}
			if ep.Content.Derivation == "" {
				ep.Content.Derivation = model.DerivationVerbatim
			}
		}
		ep.Content.Language = canonicalLanguage(ep.Content.Language)
		var notes []string
		if err := importOccurred(ep, imp.now, imp.zone, &notes); err != nil {
			ln.failErr(err)
			continue
		}
		for _, note := range notes {
			imp.notes = append(imp.notes, fmt.Sprintf("line %d: %s", ln.n, note))
		}
		refs := []string{}
		if ln.containerKey != "" {
			refs = append(refs, ln.containerKey)
		}
		for _, l := range ln.links {
			if k, ok := strings.CutPrefix(l.Href, "@"); ok {
				refs = append(refs, k)
			}
		}
		for _, k := range refs {
			j, ok := imp.keys[k]
			switch {
			case !ok:
				ln.fail("refused", "refused: line %d names @%s, which no line of the batch defines", ln.n, k)
			case j > i:
				ln.fail("refused", "refused: line %d names @%s, which is defined on line %d, after it; a line names only an earlier line", ln.n, k, imp.lines[j].n)
			case j == i:
				ln.fail("refused", "refused: line %d names its own @%s", ln.n, k)
			default:
				ln.deps = append(ln.deps, j)
				continue
			}
			break
		}
	}
}

// run is both phases: check every line against a snapshot, then write the
// survivors in chunks under the lock. It returns only an I/O failure; each
// line's outcome is on the line.
func (imp *importer) run() error {
	imp.stamp = temporal.FormatTimestamp(imp.now)
	imp.own = map[string]bool{}
	imp.mintBatch()
	imp.prepare()

	// Phase 1: no lock.
	l, err := imp.ws.Scan()
	if err != nil {
		return err
	}
	c := newBatchCheck(imp.ws, l, imp.lines, imp.keys, imp.own, imp.onDup, true)
	for i, ln := range imp.lines {
		if ln.status == "" {
			c.check(i)
		}
	}
	settle(imp.lines)
	imp.a.warnings = append(imp.a.warnings, imp.notes...)
	if imp.anyFailed() && imp.onError == "stop" {
		imp.forgetUnwritten()
		return nil
	}

	// Phase 2: chunks of surviving lines, each under the lock.
	var todo []int
	for i, ln := range imp.lines {
		if ln.status == "" {
			todo = append(todo, i)
		}
	}
	for chunk := 0; len(todo) > 0; chunk++ {
		n := min(importChunkSize, len(todo))
		part := todo[:n]
		todo = todo[n:]
		if importBeforeChunk != nil {
			importBeforeChunk(chunk)
		}
		stop, err := imp.writeChunk(part)
		if err != nil || stop {
			imp.forgetUnwritten()
			return err
		}
	}
	return nil
}

func (imp *importer) anyFailed() bool {
	for _, ln := range imp.lines {
		if ln.status == lineFailed {
			return true
		}
	}
	return false
}

// forgetUnwritten clears the outcome of lines neither written, skipped nor
// failed, which a stopped import reports nothing for.
func (imp *importer) forgetUnwritten() {
	for _, ln := range imp.lines {
		if ln.status == "" {
			ln.code, ln.message = "", ""
		}
	}
}

// writeChunk re-checks one chunk against a fresh listing under the lock,
// resolves its auto ordinals and writes it with one index upsert. stop is
// set when a line failed under --on-error stop, and nothing of the chunk
// was written.
func (imp *importer) writeChunk(part []int) (stop bool, err error) {
	unlock, err := imp.ws.Lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	l, err := imp.ws.Scan()
	if err != nil {
		return false, err
	}
	c := newBatchCheck(imp.ws, l, imp.lines, imp.keys, imp.own, imp.onDup, false)
	failed := false
	for _, i := range part {
		c.check(i)
		if imp.lines[i].status == lineFailed {
			failed = true
		}
	}
	if failed && imp.onError == "stop" {
		return true, nil
	}
	var eps []*model.Episode
	var idx []int
	for _, i := range part {
		if ln := imp.lines[i]; ln.status == "" {
			eps = append(eps, ln.ep)
			idx = append(idx, i)
		}
	}
	if len(eps) == 0 {
		return false, nil
	}
	paths, err := imp.ws.CreateBatch(l, eps)
	for k, p := range paths {
		ln := imp.lines[idx[k]]
		ln.status, ln.path = lineWritten, p
	}
	imp.ws.NoteUIDs(eps[:len(paths)])
	if err != nil && len(paths) == 0 && errors.Is(err, store.ErrSealedMonth) {
		return false, refusedErr("%v", err)
	}
	return false, err
}

// lineJSON is one line's outcome (design D5).
func lineJSON(ln *importLine) map[string]any {
	m := map[string]any{"line": ln.n, "status": ln.status}
	if ln.key != "" {
		m["key"] = ln.key
	}
	switch ln.status {
	case lineWritten:
		m["id"], m["path"] = ln.ep.ID, ln.path
	case lineSkipped:
		m["id"], m["path"] = ln.existing, store.RelPath(ln.existing)
		m["code"], m["message"] = ln.code, ln.message
	case lineFailed:
		m["code"], m["message"] = ln.code, ln.message
	}
	return m
}

// importResult writes every decided line's outcome, in input order, and the
// counts: one object with --json, one per line and a summary with --ndjson,
// or text.
func (a *app) importResult(lines []*importLine, src model.Source, ndjson bool) error {
	counts := map[string]int{lineWritten: 0, lineSkipped: 0, lineFailed: 0}
	items := []map[string]any{}
	var decided []*importLine
	for _, ln := range lines {
		if ln.status == "" {
			continue
		}
		counts[ln.status]++
		items = append(items, lineJSON(ln))
		decided = append(decided, ln)
	}
	summary := map[string]any{"written": counts[lineWritten], "skipped": counts[lineSkipped], "failed": counts[lineFailed], "source": sourceJSON(src)}
	if ndjson {
		enc := json.NewEncoder(a.stdout)
		enc.SetEscapeHTML(false)
		for _, it := range items {
			if err := enc.Encode(it); err != nil {
				return err
			}
		}
		if len(a.warnings) > 0 {
			summary["warnings"] = a.warnings
		}
		return enc.Encode(summary)
	}
	summary["lines"] = items
	return a.emit(summary, func(w io.Writer) {
		fmt.Fprintf(w, "Imported %d written, %d skipped, %d failed\n", counts[lineWritten], counts[lineSkipped], counts[lineFailed])
		sort.SliceStable(decided, func(i, j int) bool { return decided[i].n < decided[j].n })
		for _, ln := range decided {
			switch ln.status {
			case lineWritten:
				fmt.Fprintf(w, "  line %d: written %s\n", ln.n, ln.ep.ID)
			case lineSkipped:
				fmt.Fprintf(w, "  line %d: skipped, %s\n", ln.n, ln.message)
			default:
				fmt.Fprintf(w, "  line %d: failed (%s) %s\n", ln.n, ln.code, ln.message)
			}
		}
	})
}
