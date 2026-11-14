## Why

A `Create` into a workspace whose open months hold 20,000 entries takes 357 ms, against design D14's target of 100 ms ([#2](https://github.com/nodelogicau/episodes-cli/issues/2)). The episode write itself is cheap. Most of the time goes to the root index: every create parses all 20,000 entries of `index.yaml` (about 130 ms) and re-encodes them (about 190 ms), only to add one entry at the end. Automatic capture, the next planned change, will write a turn per utterance and hold the workspace lock for the whole upsert, so this cost becomes latency for every concurrent writer.

## What Changes

- The index upsert gains a fast path. When a create opens no new month, its id sorts after every entry already in the root, and the root is in the form this binary writes, the new entry is encoded on its own and spliced onto the end of the root's bytes. The result is then written atomically, as now.
- Every other case takes the existing full path, unchanged: a create that opens a month (and so may seal one), a root that doesn't parse or holds conflict markers, an id that doesn't sort last, an empty root, or a root not in canonical form.
- The fast path and the full path must produce byte-identical roots. A property test runs random sequences of creates across month boundaries both ways and compares every root.
- `docs/benchmarks.md` is updated with the new `BenchmarkCreate20k` result.
- No change to the file format, the index's contents, `index`, `index --check` or `validate`.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `index`: adds a requirement that a create's upsert writes the same root bytes whichever way it is computed, and that it takes time independent of the number of open-month entries when no month opens.

## Impact

- `internal/store/index.go` (`upsert`), with tests in `internal/store` and the benchmark in `internal/store/bench_test.go`.
- `docs/benchmarks.md` and `CHANGELOG.md`.
- Risk is limited to the root's bytes, and the equivalence test guards that. A root the fast path declines falls back to today's code.
