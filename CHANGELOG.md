# Changelog

All notable changes to `episodes` are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- **Anchored links and the `memento` relation** (#15, upstream change
  `web-content`, nodelogicau/episodes#13 and #16). A link may carry RFC
  8288's `anchor`, in one form only: the episode's own `content.ref`. Links
  are sorted with an unanchored link before an anchored one with the same
  `rel` and `href`. `record ref --memento <uri>` writes a `memento` link to
  an archived snapshot you already made. Recording never archives
  anything itself. `validate` rejects an anchor that is not `content.ref`
  or is on an inline episode, a `memento` without an anchor, and a
  `memento` whose `href` is an episode id.
- **`record ref` says which bytes to hash** (#10, upstream
  nodelogicau/episodes#24). The help text and README give the bytes for
  each `media_type`, what to do when you hold only a provider's rendering
  (no hash, use `--version`), and that a hash identifies one stored copy.
  A hash given without `--media-type` is still written, with a warning.
- **`validate` warns, rather than reporting an error, on an unregistered
  rel of registered form** (#6, upstream nodelogicau/episodes#26). A rel
  such as `answers` that has the form of a registered relation but is
  missing from this build's registry snapshot is valid, with the warning
  `link_rel_unregistered`. That way two validators carrying different
  copies of the registry differ only in warnings. A rel of neither form
  (`Answers`, `my relation`) is still an error. `record` still refuses an
  unregistered rel that isn't an absolute URI.
- **Redacting `content.ref` also redacts `uid`, `version`, `hash` and
  `links`** (#12, upstream nodelogicau/episodes#37). Each of these could
  lead back to the content. `redact` adds whichever the episode carries and
  names them in its result (`added`). It does the same on any redaction of
  an episode whose earlier block already lists `content.ref`, which repairs
  older redactions. `validate` reports `redaction_incomplete` for each field
  left behind. Redacting `content.uid` now deletes it from the uid cache,
  and a lookup that finds a cached uid its file no longer carries (a
  redaction merged in) deletes it too (nodelogicau/episodes#18).
- **A stated time may be up to five minutes ahead of `timestamp`** (#13,
  upstream nodelogicau/episodes#22). Mail whose `Date` header runs slightly
  fast can now be recorded, and the time is written as stated. Beyond five
  minutes it is still refused, and `validate` warns only beyond the
  allowance. `recorded` and `inferred` times get no allowance, and a date
  alone keeps its one day.
- **RFC 9557 suffixes on input are normalised instead of refused** (#5,
  upstream nodelogicau/episodes#28). Every verb that takes `--occurred` or
  `--end` drops an elective suffix other than the zone, and warns. A
  critical suffix other than the zone is refused, naming it. A critical
  zone is checked against the offset: the value is written without the
  flag when they agree, and refused when they don't.
- **A zone that contradicts the offset, or one with no rules, is left
  out** (#14, upstream nodelogicau/episodes#27). The offset is written
  alone, with a warning, where the value used to be refused. `--zone`,
  `EPISODES_ZONE` and `defaults.zone` still apply only to a time taken from
  the clock.
- **BREAKING (file output): `act` writes `content: {mode: attested}`** (#8,
  upstream change `acts-and-containers`). Before, it wrote `{mode: none}`.
  `attested` is a fourth content mode, and it carries no other key,
  `language` included. Every reader accepts it. An older build reports
  `invalid_mode` on an act written by this one. Existing files are not
  rewritten.
- **`validate` recognises acts by their mode, not by their shape** (#9). The
  act rules apply to every `mode: attested` episode, even one with
  children, and to no other episode. A `mode: none` episode with no
  children is a container whose children are missing: it is valid whatever
  its standpoint, and gets the warning `container_without_children` (which
  used to be info, and only for conversations and threads). Acts written
  with `mode: none` by an earlier build read this way. `container_later` is
  checked only when both ids are UUIDv7.
- **`record ref`'s duplicate lookup reads a uid cache** (#3, change
  `uid-cache`). The cache is kept outside the workspace, in the user cache
  directory (or `$EPISODES_CACHE_DIR`), keyed by `workspace.id` and the
  workspace's path. Each lookup reconciles it with the directory listing,
  parsing only files it has not seen, and checks every hit against its
  file, so a stale cache never refuses wrongly. A cache that can't be read
  is rebuilt, and one that can't be written is skipped. At 20,000 reference
  episodes the lookup takes 52 ms, down from 815 ms. `validate` still
  parses every file.
- **Creating an episode no longer re-encodes the whole index** (#2, change
  `fast-index-upsert`). When a create opens no month and its id sorts last,
  its index entry is appended to `index.yaml`'s bytes. The bytes are the
  same as before, which a property test checks against the full upsert. At
  20,000 open-month entries a create takes 52 ms, down from 357 ms.
- **`act` names how an act was attested** (#4, change `act-attestation-flags`).
  `--control` says the person used a dedicated logging control, so no episode
  holds the attestation. It is the caller's statement, which the binary
  cannot check, and it can't be combined with `--via`. A harness, whether
  from `--harness`, `EPISODES_HARNESS` or `defaults.source.harness`, must give
  `--via` or `--control`. Before, it always needed `--via`, which refused a
  person's own act in a workspace with a default harness. The JSON result
  carries `attestation` (`via`, `control` or `author`). The episode file is
  unchanged.

### Fixed

- **`content.language` accepts every tag the RFC 5646 §2.1 grammar
  accepts** (#7, upstream nodelogicau/episodes#36). A primary subtag of four
  to eight letters, such as `english`, is no longer rejected. `record`
  writes it, and `validate` reports it as valid with a
  `language_unregistered` warning, because the registry holds no such
  subtag. Ill-formed tags such as `en_AU`, and `und`, are still rejected.
- **`list --container` lists children in the container's order** (#11,
  upstream nodelogicau/episodes#19). That is by `ordinal`, then by id, with
  children that have no `ordinal` last. Before, it ordered them by when they
  occurred. `--by ordinal` names this order, and `--by occurred` and
  `--by recorded` still work. `show --children` and `thread` already used
  this order, and a test now pins it.

## [0.1.0] - unreleased

The foundation (`bootstrap-episodes-cli`): a complete, byte-stable writer for
every shape of episode in the `episodes/0.1` draft, the two permitted
mutations, recall, sharding, the index, and a validator.

### Added

- **Workspace.** `init` writes `episodes.yaml` (`format`, `workspace.id`,
  optional `defaults.source` and `defaults.zone`), `episodes/` and an empty
  `index.yaml`. Discovery by `--workspace`, a strict `EPISODES_WORKSPACE`,
  then the nearest `episodes.yaml` or `.episodes` pointer. A workspace whose
  `format` is not `episodes/0.1` is refused with exit 1. `workspace` and
  `workspace pointer`.
- **The `EPISODE` object.** Ids are `epi_` + UUIDv7, minted with a monotonic
  counter. Fields are written in canonical order with deterministic YAML:
  quoted `occurred` bounds, literal block text, sorted `participants` and
  `links`, and UTC `Z` instants. Files are read in any order. Unknown fields
  are kept, re-emitted unchanged, and given no meaning.
- **`occurred` as RFC 9557.** The offset governs and the zone is written
  only when it is supplied or configured. A date alone may carry `tz`. Writers
  refuse critical and elective suffixes, offset–zone disagreement, mixed
  forms, `planned`, an `--occurred` without `--basis`, and anything later
  than the recording instant.
- **Content.** Three modes, `inline`, `ref` and `none`. `derivation`. `uid`
  requires `media_type`. `--message-id` is converted to `mid:`. `sha256`
  hashes are taken over CRLF-normalised bytes. `language` is checked as a
  well-formed BCP 47 tag in canonical case, and `und` is refused. A
  reference episode never holds text, and an inline one never holds a `uid`.
- **Recording verbs.** `record inline`, `record ref` (refuses a duplicate
  `(standpoint, media_type, uid)` unless `--allow-duplicate`), `conversation
  start` (container and first turn under one lock), `conversation turn`
  (assigns the next ordinal; a repeated `(container, ordinal)` is refused as
  `duplicate`), and `act` (self-attested; `--via` is required when a harness
  records it).
- **Links.** RFC 8288 typed links whose `rel` is checked against an embedded
  IANA Link Relations snapshot (2026-10-13) or must be an absolute URI.
  Episode targets must exist and be earlier. A link restating containment is
  refused.
- **Accumulation.** No verb edits or deletes an episode. Creation is atomic
  and exclusive. One store function performs every rewrite and refuses any
  change beyond `source_status` and redaction. An advisory lock on
  `episodes.yaml` (`flock`, or `LockFileEx` on Windows) is held from minting
  to the index upsert.
- **`observe`** writes `source_status` from a stated state, a hash of fetched
  content, or a version. `verified` is always set to now, and `detected` only
  when the state changes.
- **`redact`** removes fields from the closed set, merges with any earlier
  block, is irreversible, never cascades, and every result gives the
  git-history notice.
- **Recall.** `show [--children]` locates the file without the index.
  `list` filters by occurred interval, standpoint, kind and container, orders
  by occurred instant, and leaves out withdrawn episodes unless asked.
  `thread` gives withdrawn turns a placeholder. Recall lists directories and
  parses whatever the index lacks, so a stale index never makes it wrong.
- **Sharding and the index.** Months come from the UUIDv7 instant, with
  `legacy/` for others. Sealing allows a month of grace. Sealed segments
  carry a `count`, and none is written for an empty month. Late merges are
  detected by count. `index` rebuilds and never rewrites a segment, and
  `index --check` compares without writing.
- **`validate`** covers every error, warning and info finding the draft
  names, including `revised` for indexed fields that disagree with the
  index, `late_merge`, broken clocks, act rules recognised by shape, and
  `possible_duplicate`.
- **Acceptance.** Every scenario in the upstream specs (7 capabilities, 40
  requirements, 78 scenarios) has a test, as does every scenario in this
  change's specs.
- `SPEC-FEEDBACK.md` with 18 items for the draft.
- `docs/benchmarks.md`. `Create` at 20,000 open-month entries takes 357 ms,
  against a 100 ms target. Duplicate lookup at 20,000 reference episodes
  takes 815 ms. Both remedies are follow-ons (#2, #3).
