## Context

The [Episodes Format](https://github.com/nodelogicau/episodes) (`episodes/0.1`, undeclared draft) defines one object type, `EPISODE`, and two blocks that may be appended to it, `source_status` and `redacted`. Each episode is one YAML file, sharded by the month its UUIDv7 id was minted, under a workspace marked by `episodes.yaml`, with a derived `index.yaml` and sealed index segments. The upstream repository carries seven OpenSpec capability specs (40 requirements, 78 scenarios: `content-identity`, `content-language`, `episode-evidence`, `episode-links`, `occurred-time`, `self-attested-acts`, `standalone-conformance`). This implementation treats them as its acceptance suite, alongside the README's normative sections.

Operating model, decided during exploration and carried over from the siblings:

- **The author is an LLM harness; a person reviews.** The harness calls the CLI from a shell and review happens through git pull requests. The CLI performs no git operations.
- **Sibling of `particulars-cli` and `intentions-cli`.** Same language, layout, build, release, CLI contract and exit codes. Someone who knows one knows all three.
- **Copy, don't share.** Code is copied from the siblings and adapted. No shared module is introduced (D2).
- **The harness parses.** The binary never reads mail, calendar files or provider APIs, and fetches nothing. It takes fields the harness extracted and enforces the format's rules on them.
- **The spec is a draft and this is its first reader.** Where the text leaves a behaviour undecided, this design decides, records the decision, and sends the item to `SPEC-FEEDBACK.md` and then upstream. Format knowledge lives in one package, so a resolution upstream is a one-package change.
- **This change is the foundation.** It ends at a store that can be trusted: everything an episode can be, the two permitted mutations, recall, sharding, index and validation. Capture, skill and MCP follow.

The repository is greenfield: an OpenSpec scaffold, a README line, a licence, and no code.

## Goals / Non-Goals

**Goals:**

- Read and write `episodes/0.1` workspaces exactly as the draft describes, for every shape of episode: inline turn, reference trace, container, child, transcript with links, and self-attested act.
- Byte-identical output for identical episodes: canonical field order, deterministic YAML, quoted `occurred` values, sorted lists.
- Accumulation enforced by construction: no verb can revise an episode, and the only code path that rewrites a file touches only `source_status` and `redacted`.
- Every writer MUST and MUST NOT in the draft refused at write time, and every validator rule enforced by `validate`. Validation is the invariant; refusal at write time is a convenience.
- Recall that is never wrong because the index is stale (it may be slower).
- Agent-pleasant: non-interactive, `--json` everywhere, stable exit codes, and attribution on every act.
- A package layout that lets `add-capture`, `add-skill-verb` and `add-mcp-server` add packages without reshaping these.

**Non-Goals:**

- Automatic capture of conversation turns from harness hooks or transcripts. This is the next change, and D7 and D12 are shaped for it.
- Parsing `.eml`, `.ics` or JSCalendar, and fetching from any source. The harness does this.
- Full-text search over inline text, embeddings, or ranking.
- Any verb that names an object of another format. Episodes cite nothing, and record-and-cite is the harness's or `particulars`'s job.
- Checking language subtags against the IANA registry (the draft says MAY warn), EDTF uncertainty, index masking, and every other item the draft lists as open.
- Agent skill, MCP server, bundle, installer, Homebrew cask.
- Any judgement inside the binary. The harness decides what happened; the tool records, refuses and reports.

## Decisions

### D1. Language and build: Go, static binary, cross-compiled

**Choice:** Go module `github.com/nodelogicau/episodes-cli`, binary `episodes`, `CGO_ENABLED=0`, mise pin `go = "1.26.7"`. The Makefile, `.golangci.yml`, `.gitattributes`, `.gitignore` and GitHub Actions are copied from `intentions-cli` with the binary and module renamed.

**Dependencies:** `spf13/cobra`, `gopkg.in/yaml.v3` (node-level control of key order and quoting), `github.com/google/uuid` (UUIDv7 with monotonic sequence), `golang.org/x/sys` (file locking on Windows, D7), and a blank import of `time/tzdata` so the static binary carries the IANA zone database. There is no RFC 8785 and no RRULE: episodes have no projection hash and no cadence.

### D2. Copy, don't share

**Choice:** Copy and adapt. There is no `nodelogicau/formatkit`.

| From | Into | What |
|---|---|---|
| `particulars-cli/internal/store` (`sharded.go`, `index.go`, `workspace.go`, `config.go`) | `internal/store` | Month derivation from UUIDv7, `legacy/`, sealing with grace, segments with `count`, late-merge detection, workspace discovery and pointer |
| `particulars-cli/internal/prov` | `internal/prov` | Source resolution from flag, environment and defaults; the placeholder check; "at least one of author or harness" |
| `particulars-cli` hashing | `internal/model/hash.go` | `sha256:` over CRLF→LF-normalised bytes, accepting any `<alg>:<digest>` on read |
| `intentions-cli/internal/apperr` | `internal/apperr` | Exit codes and error codes, unchanged |
| `intentions-cli/internal/temporal` (zone and offset handling only) | `internal/temporal` | Zone loading, offset-at-instant, RFC 3339 parsing; extended for RFC 9557 (D5) |
| `intentions-cli/internal/cli` (`root.go`, `helpers.go`, `cmd_init.go`, workspace verbs) | `internal/cli` | Cobra root, global flags, JSON and text rendering, `--now` |

**Alternatives:** A shared module would mean one fix in one place. It was rejected because the draft lists a shared "provenance spine" as an open question, and a module extracted before that is answered would fix one answer into three tools. The draft also requires episodes to be implementable with no DKF or intentions code. The cost is drift, which is accepted and listed under Risks. Each copied file carries a header comment naming its origin file and commit, so drift can be found with a grep.

### D3. Package layout: core is transport-agnostic

```
cmd/episodes/            main; wires cobra to core; blank-imports time/tzdata
internal/apperr/         exit codes and error codes shared by every front-end
internal/temporal/       pure: RFC 9557 parse/format, offset-zone agreement, date-only and tz, instant comparison
internal/model/          format layer: Episode, Content, Link, SourceStatus, Redacted; ids; canonical order;
                         YAML codec; field rules; hash; BCP 47 well-formedness and case; mid: encoding;
                         link-relation registry snapshot
internal/prov/           source resolution and checks
internal/store/          workspace, lock, path derivation, sharding, atomic writes, THE mutation choke point, index
internal/query/          validate, recall (list, thread, children), duplicate lookup: pure functions over a loaded view
internal/cli/            cobra commands, flags, rendering
```

`model` imports only `temporal`. `store` imports `model`. `query` imports both. `cli` imports everything. An MCP server later is a second front-end beside `cli`, calling the same core functions.

### D4. Identifiers, canonical order and the codec

Ids are `epi_` followed by a lowercase canonical UUIDv7 from `uuid.NewV7`, which carries a monotonic sequence within a millisecond. Reading accepts `^epi_[A-Za-z0-9-]+$`, and `validate` gives info for a non-v7 id.

The encoder writes fields in the README's order: `id, type, kind, occurred, standpoint, participants, place, label, container, ordinal, links, content, source, timestamp, source_status, redacted`. Embedded blocks are written in their documented order, unknown fields go after specified ones in the order they were read, and `redacted` is always last. Other rules: 2-space indent, literal block scalars for multi-line `text`, `occurred.start`/`end` always double-quoted (in files and in the index), recording instants in UTC with `Z` at seconds precision, `participants` sorted and de-duplicated, `links` sorted by `rel` then `href`, absent optional fields omitted, and no document markers.

The decoder works over `yaml.v3` nodes. It accepts any key order and rejects duplicate keys and non-mapping documents. It keeps unknown keys in an ordered extras map and gives them no meaning, which is how a leftover `occasion` is handled. Because no verb revises an episode, extras are re-emitted only by `observe` and `redact`, and those keep them byte-for-byte.

`kind` is checked against `^[a-z][a-z0-9-]*$`; the vocabulary is open. References (`standpoint`, `participants`, `place`, `source.author`) are non-empty strings. A reference that is not a URI is accepted, as the draft requires.

### D5. `occurred` and time

`internal/temporal` parses an RFC 3339 `date-time` with an optional bracketed suffix. It accepts one IANA zone suffix. It refuses on write, and reports invalid on read, any critical (`!`) suffix. It ignores elective suffixes it does not implement on read and refuses them on write (feedback 10). Comparisons use the instant the offset fixes.

On write, an offset that disagrees with the zone's rules at that local time is **refused**. In `validate` it is a warning, and the instant is still taken from the offset. A date alone is accepted with an optional `--tz`. `tz` beside a date-time is refused, and `start` and `end` must be the same form.

**Where a recorded time comes from.** When a verb defaults `occurred` to now (`conversation turn`, and `record inline` without `--occurred`), the offset is the process's local offset at that instant. The zone is written only if it is configured: `--zone`, then `EPISODES_ZONE`, then `defaults.zone` in `episodes.yaml`. The zone is never inferred from the host, which keeps to the draft's "never guessed" (feedback 9).

**Basis is never defaulted when a time is supplied.** A verb that takes `--occurred` requires `--basis`. When the verb supplies now itself, the basis is `recorded`. `--basis planned` is refused with a message quoting the draft's rule.

**Nothing is recorded before it happens.** At write time, `occurred.start` or `occurred.end` later than the episode's `timestamp` is refused with exit 2. The message gives the difference. There is no tolerance: the draft says MUST NOT, and the validator warning exists for skew this binary did not write (feedback 4). A date alone is compared as the draft describes: in `tz` when present, otherwise in UTC allowing one day.

### D6. Content

- **inline:** `text` is required (from `--text` or `--text-file <path|->`). `media_type` defaults to `text/markdown` and `derivation` to `verbatim`. `uid` is refused.
- **ref:** `ref` is required and must not match the episode id pattern. `text` is refused. `uid` requires `media_type`. `--message-id '<x@y>'` is converted to `mid:` by stripping the angle brackets and percent-encoding as RFC 2392 requires; given together with `--uid`, it is refused. `--hash-file <path|->` computes `sha256:` over the bytes with CRLF→LF and nothing else. `--hash` is accepted only as `sha256:<64 hex>` on write. Readers accept any algorithm and `validate` reports an unknown one as unverified. Which bytes to hash is the harness's choice (feedback 6).
- **none:** written only by `conversation start` (as the container) and by `act`. `language` is refused.
- **`language`:** checked for well-formedness against the RFC 5646 ABNF, hand-written and table-tested, then written in the case §2.1.1 recommends. `und` is refused, comparison ignores case, and there is no registry check. The flag's help text says "only when the source states it".

### D7. Accumulation, atomic writes and the lock

Writing goes through two functions in `store`:

- `Create(ep)` writes a new file at its derived path with `O_EXCL`. It refuses if the path exists, refuses if the target month is sealed, and upserts the index.
- `Amend(id, fn)` reads the file, passes it to `fn`, and then **compares every field except `source_status` and `redacted`, and the fields that `redacted.fields` removes, against the original**. It refuses the write if anything else differs, writes atomically (temp file plus rename), and leaves the index alone, because neither block is indexed.

No other code path writes an episode file. `observe` and `redact` are the only callers of `Amend`. The comparison is a programming guard that makes accumulation a property of the store rather than of each verb.

**The lock.** Every writing verb takes an exclusive advisory lock on `episodes.yaml` (`flock` on unix, `LockFileEx` on Windows) for the length of mint, check, write and index upsert. This serialises ordinal assignment (D8), duplicate lookup (D9) and the index, because the next change will run hooks that write turns concurrently. Locking `episodes.yaml` adds no file to the workspace and leaves no stale lock when a process dies. Readers do not lock.

### D8. Containment and ordinals

- **A container must resolve and be earlier.** At write time, `--container` must name an episode that exists in this workspace and whose id sorts before the new one. Both always hold for ids this binary mints. In `validate`, a container that does not resolve is a **warning** (`container_unresolved`), because an episode can be dropped or never copied. A container id that is not earlier is a warning (`container_later`) (feedback 2).
- **Ordinals are assigned under the lock.** `conversation turn <container>` assigns one more than the highest ordinal among the container's children. Children can only be minted in the container's month or later, so the search covers those months only. Sealed months whose `count` matches are read from their segment; open months come from a directory listing plus parsing any file the index lacks. `--ordinal N` is accepted.
- **`(container, ordinal)` is unique.** A write that repeats one is refused with exit 2 code `duplicate`, naming the episode that holds it. In `validate` a repeat is a warning (`ordinal_repeated`), because two branches can each assign the same ordinal and merge cleanly (feedback 1).
- **`conversation start`** writes the container (`kind` from `--kind`, default `conversation`, `content: {mode: none}`, `standpoint` from `--container-standpoint` or else the resolved `source.author`, see feedback 16) and its first turn (ordinal 1) under one lock. The container is renamed into place first, then the turn. A crash between the two leaves a childless container, which the draft's recognition-by-shape rule reads as a self-attested act. `validate` follows the draft and applies the act rules to it. When its `kind` is `conversation` or `thread`, it also adds an info finding, `container_without_children`, naming the remedy: `conversation turn <id>` appends a child, which makes it a container again and changes no existing file. The draft should settle the ambiguity, not the implementation (feedback 3).
- Other containment (an email and its attachments) uses `record inline|ref --container X [--ordinal N]`. `ordinal` is optional there, because the draft requires it only "when order is meaningful".

### D9. Duplicates on write

Recording the same reference twice cannot be undone, because episodes are never revised. So `record ref` with a `uid` looks for existing reference episodes with the same `standpoint`, `media_type` and `uid`, and if it finds any, **refuses** with exit 2 code `duplicate`, listing their ids. The usual right move for a harness is to cite the existing episode. `--allow-duplicate` records anyway, for a distinct occurrence that shares a `UID`, such as an instance of a recurring event. Episodes from different standpoints that share a `uid` are never duplicates.

The lookup parses every reference episode, because `uid` is deliberately not in the index. That is O(n), which is acceptable at bootstrap and is listed under Risks. `validate` reports such sets at info level (`possible_duplicate`) (feedback 5).

### D10. Links

`--link rel=href` can be repeated. `rel` must be either a lowercase name in an embedded snapshot of the IANA Link Relations registry (dated, and reported by `version --json` as `link_relations`) or an absolute URI. `href` is an episode id or an absolute URI. An episode `href` must resolve and be earlier at write time. In `validate`, one that does not resolve is info (`link_unresolved`) and never makes the episode invalid.

An `up` or `collection` link whose target is the episode's own container is refused on write and warned on in `validate`. A rel that is in neither the snapshot nor a URI is an error in `validate`, as the draft says. `show` and `list` never reject an episode for one, because readers MUST NOT. That split between validator and reader, and the staleness of any snapshot, is feedback 8.

### D11. Self-attested acts

`act` writes `content: {mode: none}`, with `kind` defaulting to `activity`, `--occurred` and `--basis` (`stated` or `inferred`) required, and `standpoint` taken from the resolved `source.author`. A `--standpoint` that differs is refused. `--via <epi>` must resolve to an episode with the same standpoint.

**When the source carries a harness, `--via` is required.** A harness that records an act must have recorded the turn in which the person asked for it, and must name that turn. This turns the draft's "only on explicit attestation" from a rule nobody can check into one the store can. It is stricter than the draft, which allows no `via` for a dedicated control. A person acting through a control with no harness can still omit it (feedback 7).

`validate` recognises acts by shape, as the draft says: content none, and named as container by no episode. See D8 for the childless-container case.

### D12. The index and segments

The entry shape is exactly `id, kind, occurred, container, ordinal, standpoint, timestamp`. The root `index.yaml` holds `format`, `segments` (`path`, `count`) and the entries for the open months, sorted by id. Each `Create` upserts the root. A `Create` that opens a new month, and so seals one, moves the sealed month's entries into `index/<YYYY-MM>.yaml` and records its `count` in the same locked write.

`index` rebuilds the root from the files and writes any missing segment. It **never rewrites an existing segment.** If a sealed month's directory count differs from its segment, `index` exits 4 naming the extra files and the remedy (re-mint the branch before merging), and writes nothing for that month (feedback 12). `index --check` compares without writing.

**Recall is not wrong when the index is stale.** `list` and `thread` take a directory listing of every month (cheap), use index entries for ids the index has, and parse files the index lacks. Index entries whose file is gone are dropped. `show <id>` derives the path from the id and never reads the index.

**What can be checked.** In a sealed month, a file whose indexed fields disagree with its segment entry has been revised, and `validate` reports it as an error (`revised`). In an open month, the same disagreement is an error too, but running `index` rewrites the root and removes the evidence. Only git review protects open months, and unindexed fields such as `content.text` are protected only by git anywhere. The README says so plainly.

The cost of the root is a rewrite of every open-month entry on every write. That is accepted at bootstrap with a benchmark target (D14). If capture's turn rate exceeds it, the fix is in `store` and needs no format change.

### D13. `observe` and `redact`

`observe <id>` applies only to reference episodes; on inline ones it is refused. It takes either `--state available|changed|unreachable|gone`, or `--content-file <path|->`, which hashes the content and compares it with `content.hash` to give `available` or `changed`. If the episode has no hash, `--version V` is compared with `content.version` instead. If neither can be compared, it is refused with a message saying so. `verified` is always now. `detected` is now when the state differs from the recorded one, and otherwise kept. The rest of the episode is never touched, and nothing is back-filled.

`redact <id> --fields <path>[,<path>…] --reason R` accepts only the closed set (`label, participants, place, links, content.text, content.ref, content.uid, content.version, content.hash, content.language`). Anything else is refused. A path the episode does not carry, and that no earlier block already lists, is refused, so `fields` never claims a removal that did not happen (feedback 11). A later redaction unions `fields` and replaces `timestamp`, `reason` and `source`. Every result, in text and in JSON as `notice`, says that the text remains in git history and in any clone. A redaction never cascades.

Recall treats an inline episode whose `content.text` is redacted as **withdrawn**. `list` leaves it out unless given `--include-withdrawn`. `thread` keeps its ordinal position as a placeholder line, so the order of turns stays readable (feedback 13). A reference episode whose `content.ref` is redacted is shown as a tombstone.

### D14. Agent-facing CLI contract

Carried over unchanged from the siblings:

- It never prompts and never reads a TTY. Prose comes from `--text` or `--text-file <path|->`, and `--label`.
- `--json` emits exactly one object on stdout. Errors in JSON mode are `{"error": {"code", "message"}}` on stderr.
- Exit codes: `0` ok · `1` runtime · `2` usage, including every write refusal and `duplicate` · `3` not found · `4` check failed (`validate`, `index`, `index --check`) · `5` no workspace.
- Attribution: `--author`, `--harness`, `--model` → `EPISODES_AUTHOR`, `EPISODES_HARNESS`, `EPISODES_MODEL` → `defaults.source`. At least one of author or harness is required.
- `--now <RFC 3339>` and `EPISODES_NOW` are test-facing and set both the timestamp and the default `occurred`.
- Verbs: `init`, `workspace [pointer]`, `record inline`, `record ref`, `conversation start`, `conversation turn`, `act`, `observe`, `redact`, `show`, `list`, `thread`, `validate`, `index`, `version`.
- Benchmark: `Create` into a workspace whose open months hold 20,000 entries completes in under 100 ms on a laptop. It is measured, not asserted in CI.

## Risks / Trade-offs

- [The draft renames fields or changes the layout before `episodes/0.1`] → All format knowledge is in `model` and `temporal`. Accept the churn; that is the point of a first reader.
- [Copied code drifts from the siblings] → Each copied file has an origin header. Drift is found with a grep and fixed by hand. This is the price of D2, revisited if the spine question is settled.
- [The root index rewrite per turn becomes the bottleneck under capture] → Benchmarked in D14. The remedy (an append-friendly root or batched upserts) is internal to `store`.
- [Duplicate lookup is O(n) parses] → Fine for on-demand reference recording. A `uid` cache outside the format, gitignored, is a possible later remedy.
- [The link-relation snapshot goes stale and `validate` errors on a newly registered rel] → The snapshot is dated and reported. Updating it is a one-file change, and the reader never rejects.
- [The strict future-time refusal rejects mail whose `Date` is seconds ahead of the local clock] → The message names the skew. The harness can wait and retry, or record with `basis: inferred` and a corrected time stated as inferred. Raised as feedback 4 asking for a stated tolerance.
- [Requiring `--via` for harness acts is stricter than the draft] → Raised as feedback 7. Loosening it later is easy; tightening it after workspaces exist is not.
- [An open-month revision is laundered by `index`] → Documented. Git review is the guard, as it is for every unindexed field.

## Migration Plan

Greenfield; nothing to migrate. Rollback is deleting the binary. The YAML is readable by anything.

## Open Questions

- **Should `conversation start` admit a reference first child?** For example a thread container whose first message is an email. This change admits only an inline first turn, so a thread of reference messages has no container verb yet: a `mode: none` container is only ever written together with its first child, and `record ref` cannot write one. Left until capture shows whether it is needed.
- **Is `defaults.zone` in `episodes.yaml` acceptable as an implementation key?** The draft ignores unknown keys, so it is legal. Whether it belongs in the format is feedback 9.
- **Should `list` order by `occurred` or by id?** This design orders by the `occurred.start` instant (a date alone counted as the start of that day in `tz`, or UTC), with ties broken by id, because recall is about when things happened. `--by recorded` orders by id.

## Spec Feedback (to raise upstream)

1. **`(container, ordinal)` uniqueness and assignment.** State that a writer MUST NOT repeat a pair, and that a validator warns on one, since branches can each assign the same ordinal.
2. **A `container` that does not resolve.** The draft says what happens to an unresolved link and nothing about an unresolved container. Propose the same treatment (reported, not invalid), plus a rule that a container is recorded before its children.
3. **A childless container is read as a self-attested act.** Recognition by shape, combined with "SHOULD write a container with its first child", means a crash or a partial copy turns a harness's conversation container into a self-attested act that fails the act rules. Propose recognition by shape *and* the absence of a container-only `kind`, or a marker field.
4. **A tolerance for "nothing is recorded before it happens".** The writer side is an unqualified MUST NOT, while the validator warns because clocks disagree. Propose a stated tolerance (for example two minutes) that writers may apply to a time a source states.
5. **What a writer does on a second `(standpoint, media_type, uid)`.** This implementation refuses unless told otherwise. Propose SHOULD NOT record without an explicit override.
6. **Which bytes `hash` covers, per media type.** `message/rfc822`: the raw message as the mailbox returns it. `text/calendar`: the VCALENDAR object. Otherwise two harnesses recording one email write two hashes.
7. **A harness recording a self-attested act should name the attesting turn.** Propose MUST link `via` when `source.harness` is present.
8. **The validator needs a registry snapshot.** "A rel that is neither is invalid" requires every validator to carry the IANA registry and to agree on its date, while readers must not reject. Propose that unregistered lowercase rels are a warning, not invalid.
9. **Where a writer learns the zone.** "The zone is written whenever it is known" has no configured source. Propose `defaults.zone` in `episodes.yaml`.
10. **Elective suffixes on input.** Writers MUST NOT write them. Say whether a writer given one strips it or refuses.
11. **Redacting a field the episode never carried.** Say whether `fields` may list one. This implementation refuses.
12. **Rebuilding when a sealed segment's count is wrong.** "A segment never changes once written" plus "the index MUST be fully reconstructible" leaves `index`'s behaviour on a late merge unstated. Propose: the rebuild fails and names the files.
13. **A withdrawn turn in a conversation.** It is "out of recall", but its position is part of the conversation. Say whether a reader may show its place.
14. **Informative note on what immutability can be checked.** Only indexed fields in sealed months are verifiable without the store's history. The draft could say so.
15. **A sealed month with no files.** "Each sealed month has a segment" is unclear for a month nothing was minted in. This implementation writes no segment for an empty month. Propose that the text say so, so that two implementations agree on the root's `segments`.
16. **A container's standpoint.** The draft says every episode is someone's, but a `mode: none` conversation container records no one's act. This implementation defaults it to `source.author` (overridable with `--container-standpoint`). Say whose a container is.
