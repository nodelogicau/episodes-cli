## Why

The [Episodes Format](https://github.com/nodelogicau/episodes) is an early-draft open format for episodic memory: what happened, when and where, from whose standpoint, and where the record of it lives. It is the third member of a family with [particulars](https://github.com/nodelogicau/particulars) (what is believed) and [intentions](https://github.com/nodelogicau/intentions) (what a person means to do), and its text says plainly that no implementation exists. `episodes` is that implementation: a sibling of `particulars-cli` and `intentions-cli`, built the same way, driven by LLM harnesses, and reviewed by people through git.

Episodes differs from its siblings in one way that shapes everything else: **it accumulates and never revises.** An intention is edited and retired; a claim is retracted and superseded. An episode is written once, and after that the format allows exactly two changes to its file: an observation of whether its source can still be reached, and a redaction. The store has to enforce that, not just permit it.

This change is the foundation only. It ends with a complete, byte-stable writer for every shape of episode the draft defines, the two permitted mutations, recall by time, container and standpoint, month sharding with sealed segments, and a validator that is the format's invariant. It stops before automatic capture of conversations, the MCP server and the agent skill, because those are best built against a store that can already be trusted.

## What Changes

- New `episodes` CLI: a single statically linked Go binary, cross-compiled, with no runtime dependencies. It follows the layout and conventions of `particulars-cli` and `intentions-cli` and **copies** their code rather than sharing a module: the sharded store and provenance from `particulars-cli`, and the temporal, error and CLI scaffolding from `intentions-cli`.
- Workspace bootstrap (`init`) writing `episodes.yaml` (`format: episodes/0.1`, `workspace.id`, optional `defaults.source`), `episodes/` and `index.yaml`. Discovery is by `--workspace`, then `EPISODES_WORKSPACE`, then the nearest ancestor holding `episodes.yaml` or a `.episodes` pointer.
- The `EPISODE` object as one YAML file. It is written in canonical field order and read in any order. Unknown fields are kept and given no meaning. Ids are `epi_<uuidv7>`, minted with a monotonic counter.
- `occurred` as RFC 9557 values: the offset is required and governs, the zone is written only when known, a date alone is allowed with an optional `tz`, there is no critical flag, values are always quoted, and `basis` is one of `recorded`, `stated` or `inferred`. **Nothing is recorded before it happens** is refused at write time.
- `content` in its three modes, `inline`, `ref` and `none`, with the rules that separate them: `derivation`, `uid` with its required `media_type`, `Message-ID` to `mid:` conversion, `sha256` over CRLF-normalised bytes, BCP 47 `language` that is never guessed and never `und`, no `text` on a reference episode, and no `uid` on an inline one.
- Recording verbs: `record inline`, `record ref`, `conversation start` (writes a container and its first turn together), `conversation turn` (assigns the next ordinal), and `act` (a self-attested act, with standpoint equal to author and an optional `via` link to a same-standpoint episode).
- RFC 8288 typed links: `rel` is a registered IANA relation or an absolute URI, `href` is an earlier episode id or a URI, links are sorted and written once, and containment is never restated as a link.
- **Accumulation enforced.** No verb edits or deletes an episode. One choke point in the store refuses every rewrite except of `source_status` and `redacted`.
- `observe <id>`: record `source_status` on a reference episode, either as a stated state or by hashing content the harness fetched. `verified` and `detected` follow the draft's semantics, and nothing is back-filled.
- `redact <id> --fields … --reason …`: the closed set of redactable paths, a union with any earlier block, irreversible, never implicit, and a notice in every result that git history still holds the text.
- Recall: `show <id> [--children]`, `list` filtered by occurred interval, standpoint, kind and container, and `thread <container>` in ordinal order. Redacted fields stay absent everywhere.
- Month sharding by the UUIDv7 minting instant, `legacy/` for underivable ids, sealing with a month of grace, sealed segments with `count`, late-merge detection by count, and refusal to mint into a sealed month.
- A derived `index.yaml` with sealed segments under `index/`, mirroring exactly `id`, `kind`, `occurred`, `container`, `ordinal`, `standpoint` and `timestamp`, rebuilt with `index` and checked with `index --check`. It is treated as a cache, never as the source.
- `validate` over the whole workspace, with `error`, `warning` and `info` findings and exit 4 on any error. It covers every check the draft names, including immutable fields that disagree with their index entry.
- The agent-first contract carried over unchanged: never interactive, `--json` on every verb, stable exit codes, `--author`/`--harness`/`--model` on every writing verb, `--now` for tests, and no git operations.
- `SPEC-FEEDBACK.md` seeded with the decisions the draft forced, for raising on `nodelogicau/episodes`.

## Capabilities

### New Capabilities
- `cli-interface`: The global CLI contract: non-interactive operation, `--json`, exit codes, workspace selection, attribution flags and `EPISODES_*` environment variables, `--now`, and `version`.
- `workspace`: `init`, the `episodes.yaml` marker and configuration, refusing an unimplemented `format`, discovery, and the `.episodes` pointer.
- `object-format`: Identifiers, canonical field order, deterministic YAML, order-tolerant reading, unknown-field handling, `source` and `timestamp`, `kind`, references, `participants`, `place`, and `label`.
- `occurred-time`: RFC 9557 parsing and writing, offset/zone agreement, date-only values and `tz`, `basis`, and the rule that nothing is recorded before it happens.
- `content`: The `inline`, `ref` and `none` modes, `derivation`, `uid` and `media_type`, `mid:` conversion, hashing, `language`, and the rule against ingestion.
- `links`: RFC 8288 typed links, the registered-relation check, write-once links, `href` resolution, and no restated containment.
- `recording`: `record inline`, `record ref`, `conversation start` and `conversation turn`, containment and ordinal assignment, and duplicate detection on write.
- `self-attested-acts`: The `act` verb, recognition by shape, the standpoint and `via` rules, and timing from the act.
- `accumulation`: The append-only store: the single mutation choke point, atomic writes, and the workspace lock.
- `source-status`: The `observe` verb and the `source_status` block.
- `redaction`: The `redact` verb, the closed field set, union semantics, and the git-history notice.
- `recall`: `show`, `list` and `thread`.
- `sharding`: Paths derived from ids, `legacy/`, sealing, segment `count`, late-merge and broken-clock detection.
- `index`: `index.yaml` and its segments, entry shape, upsert on write, rebuild, check, and the rule that the file wins.
- `validation`: `validate` as the workspace invariant.

### Modified Capabilities
<!-- none: greenfield -->

## Impact

- New Go module `github.com/nodelogicau/episodes-cli`, binary `episodes`. Dependencies are `spf13/cobra`, `gopkg.in/yaml.v3`, `github.com/google/uuid`, and a blank import of `time/tzdata`. There is no RFC 8785 or RRULE dependency, because episodes have no projection hash and no cadence.
- The build and release pipeline is copied from `intentions-cli`: Makefile, mise pin (Go 1.26.7), GitHub Actions test and lint, and a cross-compile matrix for darwin, linux and windows on amd64 and arm64.
- The code is copied, not shared. A fix to the sharded store, provenance resolution or hashing in one sibling must be carried to the others by hand. This is accepted for now: the draft lists a shared "provenance spine" as open, and a module shared ahead of that answer would freeze one.
- The harness parses. The binary never reads `.eml`, `.ics` or provider APIs, and it fetches nothing. The harness supplies the fields; the binary enforces the format's rules on them, including converting `Message-ID` to `mid:`.
- Feedback goes to the upstream draft. Items are collected in `design.md` and written to `SPEC-FEEDBACK.md` as this change closes.
- Out of scope, each planned as a follow-on: automatic capture of conversation turns through harness hooks, the agent skill (`skill show|install`), the MCP server and Claude Desktop bundle, the installer and Homebrew cask, parsing helpers for mail and calendar, full-text search over inline text, and any cross-format verb. Episodes cite nothing, so "record and cite together" belongs to the harness or to `particulars`.
