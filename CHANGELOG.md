# Changelog

All notable changes to `episodes` are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Homebrew cask** (#21). `brew install nodelogicau/tap/episodes` on macOS
  or Linux, from the same tap as `particulars` and `intentions`. GoReleaser
  publishes the cask with each release, and it clears macOS's quarantine
  flag, since the binaries are not signed or notarised.
- **`episodes query <text>`: recall by free text, with no model** (#16,
  upstream change `recall-by-query`, nodelogicau/episodes#38 to #42). It
  follows the format's baseline. The query and each of its terms are trimmed
  of whitespace and of punctuation at their edges, and an episode matches
  when any of them occurs, under full Unicode case folding, in its `label` or
  its inline text. A term equal to its `content.uid` also matches. Results
  carry `resolution` (`uid` or `lexical`) and `matched`. They are ordered by
  resolution, then by distinct terms matched, then by id, newest first, and
  `--limit` applies last. A container is returned only when it matched
  itself. `list`'s filters apply, and `--participant` compares references as
  written. Withdrawn episodes are never returned, and a redacted field never
  matches. There is no cache, so a query reads every file the filters leave:
  1.15 s over 20,000 episodes, or 650 ms with `--standpoint` halving them.
  Adds the dependency `golang.org/x/text` for case folding.

## [0.1.0] - 2026-11-21

The first release. A complete, byte-stable writer for every shape of episode
in the `episodes/0.1` draft, the two permitted mutations, recall, bulk
import, sharding, the index, and a validator. It includes the upstream spec
changes behind #5 to #15. Issue numbers below are this repository's, and
`nodelogicau/episodes#N` refers to the format's.

### Added

- **Workspace.** `init` writes `episodes.yaml` (`format`, `workspace.id`,
  optional `defaults.source` and `defaults.zone`), `episodes/` and an empty
  `index.yaml`. A workspace is found by `--workspace`, then a strict
  `EPISODES_WORKSPACE`, then the nearest `episodes.yaml` or `.episodes`
  pointer. A workspace whose `format` is not `episodes/0.1` is refused with
  exit 1. `workspace` and `workspace pointer`.
- **The `EPISODE` object.** Ids are `epi_` + UUIDv7, minted with a monotonic
  counter. Fields are written in canonical order with deterministic YAML:
  quoted `occurred` bounds, literal block text, sorted `participants` and
  `links`, and UTC `Z` instants. Files are read in any order. Unknown fields
  are kept, re-emitted unchanged, and given no meaning.
- **`occurred` as RFC 9557.** The offset governs, and the zone is written
  only when it is supplied or configured. A date alone may carry `tz`.
  Writers refuse mixed forms, `planned`, and an `--occurred` without
  `--basis`. Suffixes on `--occurred` and `--end` are normalised, with a
  warning naming what was dropped (#5, #14):
  - an elective suffix other than the zone is dropped;
  - a zone that contradicts the offset, or one with no rules, is left out,
    and the offset is written alone;
  - a critical suffix other than the zone is refused;
  - a critical zone is written without the flag when it agrees with the
    offset, and refused when it doesn't.

  `--zone`, `EPISODES_ZONE` and `defaults.zone` apply only to a time taken
  from the clock. A `stated` date-time may be up to five minutes after the
  recording instant, for mail whose `Date` header runs fast, and is written
  as stated. `recorded` and `inferred` times get no allowance, and a date
  alone gets one day (#13).
- **Content.** Four modes: `inline`, `ref`, `none`, and `attested` for acts,
  which carries no other key, `language` included (#8). `derivation`. `uid`
  requires `media_type`. `--message-id` is converted to `mid:`. `sha256`
  hashes are taken over CRLF-normalised bytes. `language` accepts every tag
  the RFC 5646 §2.1 grammar accepts, in canonical case. A well-formed tag
  whose subtag the registry lacks, such as `english`, is written with a
  `language_unregistered` warning. Ill-formed tags such as `en_AU`, and
  `und`, are refused (#7). A reference episode never holds text, and an
  inline one never holds a `uid`.
- **Recording verbs.**
  - `record inline`.
  - `record ref` refuses a duplicate `(standpoint, media_type, uid)` unless
    `--allow-duplicate`. Its help text and the README say which bytes to
    hash for each `media_type`, what to do when you hold only a provider's
    rendering, and that a hash identifies one stored copy. A hash given
    without `--media-type` is written with a warning (#10).
  - `conversation start` writes the container and its first turn under one
    lock. `conversation turn` assigns the next ordinal, and a repeated
    `(container, ordinal)` is refused as `duplicate`.
  - `act` writes a self-attested act with `content: {mode: attested}`.
    `--via <episode>` names the episode that holds the attestation, and
    `--control` says the person used a dedicated logging control, which the
    binary cannot check. A harness must give one of them. The JSON result
    carries `attestation` (`via`, `control` or `author`) (#4, #8).
- **`import`: bulk creation from NDJSON** (#1). Each line is an episode to
  create, given with the episode's own fields. The binary mints the ids in
  line order and supplies `type`, `source` and one `timestamp` for the
  batch. A line carrying any of those, `source_status`, `redacted`, or a key
  the format doesn't define is refused. `"@key"` names a line, so a later
  one can use `"@<key>"` as its `container` or a link `href`, as for an
  email and its attachments in one batch. `"ordinal": "auto"` takes the
  next position under the lock. Every line is checked before any is
  written. `--on-error stop|skip` decides whether one bad line stops the
  batch, and a line whose dependency fails fails as `dependency_failed`.
  `--on-duplicate refuse|skip|allow` makes re-running a connector safe.
  Writes go in chunks of 1,000, each under the lock with one index update,
  so a capture hook waits at most one chunk. The result maps each line to
  its id, or says why it was skipped or failed, as text, `--json` or
  `--ndjson`. Self-attested acts are refused, as is a container no line
  written names.
- **Links.** RFC 8288 typed links. `record` accepts a `rel` from the
  embedded IANA Link Relations snapshot (2026-10-13) or an absolute URI.
  Episode targets must exist and be earlier. A link restating containment
  is refused. A link may carry `anchor`, only as the episode's own
  `content.ref`, and sorts after an unanchored link with the same `rel` and
  `href`. `record ref --memento <uri>` writes a `memento` link to a snapshot
  you already archived, anchored to `--ref`. Recording never archives
  anything itself (#15).
- **Accumulation.** No verb edits or deletes an episode. Creation is atomic
  and exclusive. One store function performs every rewrite and refuses any
  change beyond `source_status` and redaction. An advisory lock on
  `episodes.yaml` (`flock`, or `LockFileEx` on Windows) is held from minting
  to the index upsert.
- **`observe`** writes `source_status` from a stated state, a hash of fetched
  content, or a version. `verified` is always set to now, and `detected` only
  when the state changes.
- **`redact`** removes fields from the closed set and merges with any
  earlier block. It is irreversible, never touches another episode, and
  every result gives the git-history notice. Redacting `content.ref` also
  redacts whichever of `content.uid`, `version`, `hash` and `links` the
  episode carries, and names them in the result (`added`). It repairs an
  earlier redaction of `content.ref` that left them. A redacted uid is
  deleted from the uid cache, including when the redaction was merged in
  from another clone (#12).
- **Recall.** `show [--children]` locates the file without the index. `list`
  filters by occurred interval, standpoint, kind and container, and leaves
  out withdrawn episodes unless asked. It orders by occurred instant, or by
  `--by occurred|recorded|ordinal`. `list --container` defaults to the
  container's order: by `ordinal`, then id, with children that have no
  `ordinal` last, as `show --children` and `thread` do (#11). `thread` gives
  withdrawn turns a placeholder. Recall lists directories and parses
  whatever the index lacks, so a stale index never makes it wrong.
- **`list --ndjson [--fields full|index]`** (#1) streams one episode per
  line, in recorded (id) order by default. `--fields index` writes the seven
  indexed fields and `path` without opening any file the index covers, and
  lists withdrawn episodes too, since the index can't tell them apart.
- **Sharding and the index.** Months come from the UUIDv7 instant, with
  `legacy/` for other ids. Sealing allows a month of grace. Sealed segments
  carry a `count`, and none is written for an empty month. Late merges are
  detected by count. A create that opens no month and whose id sorts last
  appends its entry to `index.yaml`'s bytes rather than re-encoding the
  index (#2). `index` rebuilds and never rewrites a segment, and
  `index --check` compares without writing.
- **The uid cache** (#3). `record ref` and `import` find duplicates through
  a cache kept outside the workspace, in the user cache directory (or
  `$EPISODES_CACHE_DIR`), keyed by `workspace.id` and the workspace's path.
  It is reconciled with the directory listing, parsing only files it has
  not seen, and every hit is checked against its file, so a stale cache
  never refuses wrongly. A cache that can't be read is rebuilt, and one
  that can't be written is skipped.
- **`validate`** covers every error, warning and info finding the draft
  names, including:
  - `revised`, for indexed fields that disagree with the index;
  - `late_merge` and broken clocks;
  - `possible_duplicate`;
  - the act rules, which apply to every `mode: attested` episode and to no
    other (#9). A `mode: none` episode with no children is a container whose
    children are missing, and gets the warning `container_without_children`.
    `container_later` is checked only when both ids are UUIDv7;
  - a stated time more than five minutes ahead of `timestamp` (#13);
  - `link_rel_unregistered`, a warning rather than an error, for a rel of
    registered form that is missing from the snapshot (#6). A rel of
    neither that form nor an absolute URI is an error;
  - anchors that are not `content.ref` or sit on an inline episode, a
    `memento` without an anchor, and a `memento` whose `href` is an episode
    id (#15);
  - `redaction_incomplete`, for each field a redaction of `content.ref`
    left behind (#12).

  `validate` parses every file.
- **`version` and `--version`** print the binary version, the format it
  implements and the link-relation snapshot date, as text or `--json`.
- **Releases.** Archives for darwin, linux and windows on amd64 and arm64,
  with `checksums.txt`, built by GoReleaser on a `v*` tag.
- **Acceptance.** Every scenario in the upstream specs (7 capabilities, 40
  requirements, 78 scenarios) has a test, as does every scenario in this
  repository's specs.
- `SPEC-FEEDBACK.md`, with 18 items for the draft.
- `docs/benchmarks.md`:
  - a create at 20,000 open-month entries takes 52 ms (target 100 ms);
  - a duplicate lookup at 20,000 reference episodes takes 52 ms;
  - importing 10,000 references into 20,000 entries takes 5.9 s (target
    10 s);
  - the first line of `list --ndjson --fields index` over 100,000
    episodes arrives in 126 ms (target 1 s).

### Notes for files written before this release

- Builds before this release wrote acts as `content: {mode: none}`. They
  read as containers with no children, with a warning, and are not
  rewritten. A build that predates `attested` reports `invalid_mode` on an
  act written by this one.
