## 1. Store

- [x] 1.1 `CreateBatch(l, eps)`: write each file as `Create` does, then upsert the index once, splicing many entries when `spliceable` holds for the run and otherwise falling back to one `fullUpsert` (D3)
- [x] 1.2 `NoteUID` batching: add every episode, then save once
- [x] 1.3 Stream entries month by month, from segments and root plus parsing unindexed files, without building the full slice (D6)
- [x] 1.4 Store tests: `CreateBatch` gives the same files and index as N `Create`s; a batch that opens a new month seals the old one; a partial failure returns the paths written

## 2. Import

- [x] 2.1 `internal/cli/cmd_import.go`: read NDJSON from a file or stdin, decoding strictly; refuse `id`, `type`, `source`, `timestamp`, `source_status`, `redacted` and unknown keys (D1)
- [x] 2.2 Phase 1: mint ids in line order, resolve `@key`s, run `checkProblems` and the workspace checks against a snapshot, check duplicates against `UIDCandidates` and the batch map, and enforce the act and childless-container rules (D2)
- [x] 2.3 `--on-error stop|skip`, with `dependency_failed` propagated along `@key` references
- [x] 2.4 `--on-duplicate refuse|skip|allow` (D4)
- [x] 2.5 Phase 2: chunks of 1,000 under the lock; re-scan and re-check, resolve `"auto"` ordinals, `CreateBatch` (D2)
- [x] 2.6 Result: `--json`, `--ndjson` and text; exit codes (D5)
- [x] 2.7 Register the verb; flags for source resolution shared with `record`

## 3. list --ndjson

- [x] 3.1 `--ndjson` and `--fields full|index`, usage error with `--json`, default `--by recorded` (D6)
- [x] 3.2 `--fields index` streams entries with no file parse for indexed ids; `--fields full` parses and writes per entry

## 4. Tests

- [x] 4.1 Every scenario in `specs/import/spec.md` and `specs/recall/spec.md`
- [x] 4.2 An imported episode is byte-identical to the one `record` would write with the same inputs, `--now` and id
- [x] 4.3 Concurrency: a `record` running alongside a multi-chunk import completes, and both are indexed
- [x] 4.4 A phase-2 recheck failure (a simulated concurrent ordinal grab) under `stop` reports the chunks already written

## 5. Benchmarks and docs

- [x] 5.1 `BenchmarkImport10kInto20k` (target < 10 s), `BenchmarkImportChunk` (the lock hold time), and `BenchmarkListNDJSONIndex100k`, measuring time to first line (target < 1 s)
- [x] 5.2 `docs/benchmarks.md`, `README.md` (import record shape, keys, flags, splitting very large batches, withdrawn episodes in index mode) and `CHANGELOG.md` referencing #1
