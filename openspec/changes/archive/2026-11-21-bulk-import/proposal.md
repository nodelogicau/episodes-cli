## Why

Episodic memory can arrive as a firehose of mail, calendar and file traces. Recording a trace is parsing, not interpretation, so deterministic connectors with no model in them should be able to write episodes at volume ([#1](https://github.com/nodelogicau/episodes-cli/issues/1)).

Today every episode costs a process, a lock, a duplicate lookup and an index upsert. `fast-index-upsert` and `uid-cache` made the last two cheap, but 10,000 records still means 10,000 processes, and an email with its attachments can't be written in one go. Selectors and consolidators downstream also need to read a large workspace as a stream, not as one JSON document.

## What Changes

- New verb `episodes import [<file>|-]`. It reads NDJSON, one record per episode to create.
  - A record carries the episode's own fields: `kind`, `standpoint`, `occurred`, `label`, `container`, `ordinal`, `participants`, `links` and `content`.
  - The binary supplies `id`, `type`, `source` (from the usual flags, once per batch) and `timestamp` (one instant for the batch).
  - Every rule that applies to a single `record` applies to each record.
- `"@key": "<name>"` on a record names it within the batch. A later record can use `"@<name>"` as its `container` or as a link `href`, for an email with its attachments or a past conversation with its turns.
- `"ordinal": "auto"` assigns one more than the highest ordinal in the container, under the lock, counting earlier records in the same batch.
- Every line is checked before any line is written:
  - `--on-error stop|skip` (default `stop`). With `stop`, nothing is written if any line fails. With `skip`, the good lines are written.
  - `--on-duplicate refuse|skip|allow` (default `refuse`) for a repeated `(standpoint, media_type, uid)`. `skip` makes re-running a connector idempotent.
- Writes happen in chunks of 1,000 records. The lock is held once per chunk, so concurrent capture hooks wait at most one chunk. Workspace checks are re-run for each chunk under its lock. The index is upserted once per chunk.
- Import refuses:
  - self-attested acts (`content.mode: attested`), which need explicit attestation one at a time;
  - lines that carry `id`, `type`, `source`, `timestamp`, `source_status` or `redacted`, because import is not a copy tool;
  - a `content.mode: none` container that no later line in the batch names as its `container`.
- The result names each line's outcome (written with its id and path, skipped with a reason, or failed with a code), so a connector can map its records to episode ids.
- `list --ndjson` writes one episode per line and can't be combined with `--json`. `--fields full|index` (default `full`) chooses whole episodes or the seven indexed fields. `index` is a fast path that parses no file the index already covers. Memory grows with the number of entries, not with their content.
- Benchmarks:
  - `import` of 10,000 reference records into a workspace with 20,000 entries takes under 10 s;
  - `list --ndjson --fields index` over 100,000 episodes gives its first output in under 1 s.

## Capabilities

### New Capabilities
- `import`: the `import` verb, its record shape, batch-local keys, auto ordinals, all-or-nothing checking, chunked writes and the per-line result.

### Modified Capabilities
- `recall`: `list` gains `--ndjson` and `--fields full|index`.

## Impact

- New `internal/cli/cmd_import.go`, with tests.
- `internal/store`:
  - `CreateBatch(l, eps)` writes many files and upserts the index once, splicing many entries at once. The month-sealing path still runs when the batch opens a new month.
  - `ListStream`, with no change to `Entries`' semantics.
- The duplicate check uses `UIDCandidates`, plus an in-memory set for the batch itself.
- New benchmarks in `internal/store` and `internal/cli`, recorded in `docs/benchmarks.md`. `README.md` and `CHANGELOG.md`.
- No change to the file format or the index format.
