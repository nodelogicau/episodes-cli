## Context

Single writes go through `store.Create(l, ep)`. Each one takes the lock, mints an id, checks the workspace (`checkWorkspace`: the container and link targets resolve and are earlier, and `(container, ordinal)` is free), looks up duplicates through `UIDCandidates`, writes the file with `O_EXCL`, and upserts the root index (spliced, after `fast-index-upsert`).

`model.NewID()` uses google/uuid's monotonic sequence, so ids minted in one process sort in the order they were minted. `model.NewIDAt` is used when `--now` fixes the clock for tests.

Batch constraints from #1:
- every rule that applies to a single record applies to each record;
- the lock is held per chunk of 1,000, not for the whole batch;
- the duplicate set is built once and caught up between chunks;
- the index is upserted once per chunk.

## Goals / Non-Goals

**Goals:**
- 10,000 reference records into a workspace with 20,000 entries in under 10 s.
- The same file contents a sequence of `record` calls would produce, apart from `timestamp` and id timing.
- A connector can re-run a batch safely (`--on-duplicate skip`) and map each input line to the id it got.
- `list --ndjson --fields index` gives its first line in under 1 s at 100,000 episodes.

**Non-Goals:**
- Copying episodes between workspaces. Import mints ids and supplies `source` and `timestamp`, and refuses lines that carry them.
- Importing self-attested acts.
- A transaction across chunks. Episodes are never revised or deleted, so a chunk that has been written stays written (see Risks).
- `source` per line, or accepting raw content bytes for hashing (Open Questions).

## Decisions

### D1. Record shape

A line is a JSON object with the episode's own fields, using the same names and shapes as the YAML file. On top of those:
- `@key` is a string unique within the batch;
- `container` or a link `href` may be `@<key>`, naming an earlier line;
- `ordinal` may be `"auto"`;
- `occurred` may omit `basis`; it then takes the same defaults as `record`, including `defaults.zone` for `recorded`.

These keys are refused, naming the line and field: `id`, `type`, `source`, `timestamp`, `source_status`, `redacted`, and any key the format doesn't define. Lines are decoded strictly, with unknown fields as errors.

### D2. Two phases: check everything, then write in chunks

**Phase 1 (no lock).** Every line is parsed and given a provisional id from `NewID()` in line order. `@key`s are resolved. A `@key` that is undefined, defined twice, or defined only on a later line is an error. Then the same model checks `record` runs (`checkProblems`) are run on each line. Workspace checks are run against a snapshot `Scan`: container and link targets resolve, `(container, ordinal)` is free across the workspace and the batch, and duplicates are looked up through `UIDCandidates` plus a map of the batch's own `(standpoint, lower(media_type), uid)`. The container rule is checked too: a `mode: none` line must be named by a later line.
- With `--on-error stop`, any failure writes nothing and exits 2, listing every failing line, not just the first.
- With `--on-error skip`, failing lines are dropped. Any line whose `@key` reference names a dropped line is dropped too, with code `dependency_failed`, recursively.

**Phase 2 (per chunk).** For each chunk of up to 1,000 surviving lines, in order:
1. Take the lock, re-`Scan`, and re-run the workspace checks for the chunk against the fresh listing. Another writer may have taken an ordinal or recorded a uid since phase 1.
2. Resolve `"auto"` ordinals: the highest in the container from the fresh listing, then counting earlier lines in the batch.
3. Call `CreateBatch`, then release the lock.

If a phase-2 recheck fails, the line is handled according to `--on-error`, and so is everything that depends on it. With `stop`, import stops before that chunk is written, reports what has already been written, and exits 2.

Ids minted in phase 1 are kept. They are monotonic and earlier than any id another process mints after phase 1 began, so "the container is earlier" still holds.

### D3. `CreateBatch`

`store.CreateBatch(l, eps)` writes each file exactly as `Create` does: sealed guard, `O_EXCL` and temp sweep. It then upserts the index once:
- If the fast-path preconditions (`spliceable`) hold for the whole run of new entries, it splices them in one write.
- Otherwise it does one full upsert.
- If the batch opens a new month, the existing sealing path in `fullUpsert` runs once.

The `uid` cache gets the batch through `NoteUID` per episode, with one save at the end.

A failure after some files are written returns their paths. The CLI reports them, and tells the user to run `episodes index`, as `Create` already does for one file.

### D4. Duplicates

`--on-duplicate`:
- `refuse` makes the line fail with code `duplicate`, naming the existing episode;
- `skip` reports the line as `skipped` with the existing id, which counts as success;
- `allow` writes it, like `--allow-duplicate`.

A duplicate within the batch itself is handled the same way, with the earlier line counted as the existing one.

### D5. Result

- `--json`: `{"written": n, "skipped": n, "failed": n, "lines": [{"line": i, "key": "...", "status": "written|skipped|failed", "id": "...", "path": "...", "code": "...", "message": "..."}]}`.
- `--ndjson`: the same per-line objects, one per line, then a final summary object.
- Exit status: 0 when no line failed; 2 when a line was refused; 1 for an I/O failure.

### D6. `list --ndjson`

- `--ndjson` and `--json` can't be used together (usage error). `--fields index` emits exactly the seven index fields, plus `path`.
- **Fast path (`--fields index`):** entries are streamed straight from the index segments and the root, month by month, and files are parsed only for ids the index lacks (the stale-index rule). Filters run on entries.
  - Ordering by `occurred` would need every entry in memory before the first line, so `--by recorded` is the default with `--ndjson`, which streams in id order.
  - `--by occurred` is allowed, but it buffers entries (not file content) before writing.
- **`--fields full`:** entries are streamed the same way, and each survivor's file is parsed and written immediately, so memory holds entries, not episodes.
- **Withdrawn episodes.** With `--fields full`, they are left out unless `--include-withdrawn`, as `list` does today. With `--fields index`, the output is a listing of index entries. The index can't tell whether an episode is withdrawn, because it holds neither `content.mode` nor `redacted`, so withdrawn episodes are *not* filtered out. No redacted field is output, because none of the seven is redactable. The help text and README say so.
  - Alternative, rejected: in index mode, read the file of every candidate to check withdrawal. That reads every file, which defeats the fast path #1 asks for.

### D7. Concurrency with capture hooks

The lock is held only in phase 2, per chunk. A hook waiting on it waits for at most one chunk: 1,000 file writes and one index upsert, which `BenchmarkImportChunk` measures.

## Risks / Trade-offs

- **[Partial batch under `stop`]** → A phase-2 recheck can fail after earlier chunks have been written, and they can't be undone. The result lists exactly what was written, and re-running with `--on-duplicate skip` resumes the batch. That is safe for records with a `uid`. Records without one can't be recognised on re-run (Open Questions).
- **[One timestamp for the batch]** → All episodes share the `timestamp` of the start of phase 1, which is later than or equal to every id's time. `occurred_after_recording` is checked against that instant.
- **[Index mode lists withdrawn episodes]** → An index entry reveals only that an episode exists and its seven indexed fields, which `index.yaml` already shows anyone who can read the workspace. A consumer that needs withdrawn episodes left out uses `--fields full`.
- **[Memory]** → Phase 1 holds every decoded line, which is fine at the 10,000-line target. Batches far larger should be split by the connector, and the README says so.

## Open Questions

- Should import accept `source` per line, for a connector writing on behalf of several authors? It is refused for now.
- Records without a `uid` have no guard against duplication on re-run, except the id list in the result. Should `content.hash` stand in as a duplicate key for files?
- A connector supplies `hash` itself, so it has to apply the CRLF→LF rule. Should import instead accept the raw bytes (base64 in the line, or a file path) and hash them in the binary?
