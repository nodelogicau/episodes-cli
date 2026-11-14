## 1. Implementation

- [x] 1.1 Add `entryBytes(e Entry)`: encode a one-entry root with `EncodeRoot` and return the entry's block lines
- [x] 1.2 Add `spliceable(root []byte, newID string, l *Listing)` implementing D2's four byte-level checks
- [x] 1.3 In `upsert`, read the root's bytes once; when `spliceable`, write `root + entryBytes(e)` with `writeAtomic`, otherwise run the existing full path unchanged
- [x] 1.4 Add a test-only switch (unexported package variable) that forces the full path

## 2. Tests

- [x] 2.1 Property test (D4): about 200 creates across several months, including out-of-order ids and creates after sealing, applied in two workspaces (fast and forced-full), with the root bytes compared after every create
- [x] 2.2 Fallback tests: conflict markers, `entries: []`, a new id that sorts before the last entry, a first entry in a sealed month, a root missing its final newline. Each takes the full path and gives the full path's result
- [x] 2.3 Scenario *A malformed middle is not detected*: a hand-edited root with an unparseable middle entry and a valid head and tail is extended on the fast path, `index --check` reports it as unreadable, and `index` rebuilds it
- [x] 2.4 Run the existing store, CLI and upstream suites unchanged

## 3. Benchmark and docs

- [x] 3.1 Run `BenchmarkCreate20k`, confirm under 100 ms, and update `docs/benchmarks.md` with the result and a profile summary
- [x] 3.2 `CHANGELOG.md`: an Unreleased entry referencing #2
