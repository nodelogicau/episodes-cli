# import Specification

## Purpose
`import`: creating many episodes from NDJSON in one batch. Introduced by change `bulk-import`.
## Requirements
### Requirement: Import episodes from NDJSON
`episodes import [<file>|-]` SHALL read NDJSON, one JSON object per line, each describing one episode to create. The binary SHALL supply `id`, `type`, `source` (resolved once for the batch from the usual flags, environment and defaults) and `timestamp` (one instant for the batch). Every rule that applies to a single `record` SHALL apply to each line. A line that carries `id`, `type`, `source`, `timestamp`, `source_status`, `redacted` or a key the format does not define SHALL be refused, naming the line and the key.

#### Scenario: A batch of references
- **WHEN** three `content.mode: ref` lines are imported
- **THEN** three episodes are written that share one `timestamp` and one `source`, with ids increasing in line order

#### Scenario: A copied episode refused
- **WHEN** a line carries `id`
- **THEN** that line fails with code `refused`, naming `id`

### Requirement: Batch-local keys and auto ordinals
A line MAY carry `"@key": "<name>"`, unique within the batch. A later line's `container`, or a link `href`, MAY be `"@<name>"`, and SHALL be resolved to the id minted for that line. A reference to an undefined key, a key defined twice, or a key defined only on a later line SHALL fail the line. `"ordinal": "auto"` SHALL be assigned, under the lock, one more than the highest ordinal held in the container by the workspace and by earlier lines of the batch.

#### Scenario: An email and its attachments
- **WHEN** a `mode: ref` email line with `@key: m1` is followed by two attachment lines with `container: "@m1"` and `ordinal: "auto"`
- **THEN** both attachments name the email's id as `container`, with ordinals 1 and 2

#### Scenario: Forward reference
- **WHEN** a line names `"@later"` as `container` and `@later` is defined on a following line
- **THEN** the line fails, naming the key

### Requirement: Every line checked before any is written
Import SHALL check every line before writing any. With `--on-error stop` (the default), it SHALL write nothing if any line fails, and SHALL report every failing line. With `--on-error skip`, it SHALL write the lines that pass, and SHALL fail every line whose `@key` reference names a failed line, with code `dependency_failed`.

#### Scenario: One bad line stops the batch
- **WHEN** line 7 of 10 has an invalid `occurred` and `--on-error` is `stop`
- **THEN** no episode is written and the result names line 7

#### Scenario: Skip writes the good lines
- **WHEN** the same batch is imported with `--on-error skip`
- **THEN** nine episodes are written and line 7 is reported as failed

### Requirement: Duplicates in a batch
For a repeated `(standpoint, media_type, uid)`, whether repeated within the workspace or within the batch, `--on-duplicate refuse` (the default) SHALL fail the line with code `duplicate`, naming the existing episode. `skip` SHALL report the line as skipped, naming the existing episode, without failing. `allow` SHALL write it.

#### Scenario: Idempotent re-run
- **WHEN** a batch of 100 records with `uid` is imported, and then imported again with `--on-duplicate skip`
- **THEN** the second run writes nothing, reports 100 lines skipped, each naming its existing id, and exits 0

### Requirement: Chunked writes under the lock
Import SHALL write in chunks of at most 1,000 lines. It SHALL hold the workspace lock once per chunk, re-check that chunk's workspace rules against a fresh listing under the lock, and upsert the index once per chunk. Chunks already written SHALL stay written if a later chunk fails, and the result SHALL name every line written.

#### Scenario: A hook writes between chunks
- **WHEN** a capture hook records a turn while a 5,000-line import is running
- **THEN** the hook's write completes after at most one chunk, and both the hook's turn and every imported episode are indexed

### Requirement: Import refuses acts and dangling containers
Import SHALL refuse a line with `content.mode: attested`, because a self-attested act needs explicit attestation one at a time. It SHALL refuse a line with `content.mode: none` that no later line of the batch names as `container`.

#### Scenario: Act refused
- **WHEN** a line has `content: {mode: attested}`
- **THEN** that line fails with code `refused`

#### Scenario: Childless container refused
- **WHEN** a `mode: none` conversation line has `@key: c` and no later line names `@c`
- **THEN** that line fails, naming the missing children

### Requirement: Per-line result
Import SHALL report each line's outcome: `written` with its id and path, `skipped` with the existing id, or `failed` with a code and message. It SHALL also report the counts. It SHALL exit 0 when no line failed, and 2 when any line was refused.

#### Scenario: Mapping lines to ids
- **WHEN** an import of 3 lines succeeds with `--json`
- **THEN** `lines` holds 3 objects, in input order, each with `line`, `status: written`, `id` and `path`

### Requirement: Import throughput
Importing 10,000 reference records into a workspace with 20,000 entries SHALL take under 10 s on the benchmark machine recorded in `docs/benchmarks.md`.

#### Scenario: Benchmark
- **WHEN** `BenchmarkImport10kInto20k` is run
- **THEN** it reports under 10 s per operation
