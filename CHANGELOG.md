# Changelog

All notable changes to `episodes` are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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
