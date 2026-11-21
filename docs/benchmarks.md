# Benchmarks

Measured, not asserted in CI (design D14 of `bootstrap-episodes-cli`). Run
them with:

```sh
go test ./internal/store ./internal/query -run '^$' -bench . -benchtime 10x
go test ./internal/cli -run '^$' -bench . -benchtime 3x -timeout 30m
```

## 2026-11-21, `bulk-import` (#1)

| Benchmark | Target | Result |
|---|---|---|
| `BenchmarkImport10kInto20k` | under 10 s | **5.9 s** |
| `BenchmarkImportChunk` (lock hold time of one chunk of 1,000) | | **558 ms** |
| `BenchmarkListNDJSONIndex100k`, time to first line (100,000 episodes over two years) | under 1 s | **126 ms** (1.08 s for the whole stream) |
| `BenchmarkListNDJSONIndex100kOpen`, time to first line (all 100,000 in the open months) | | **813 ms** (1.10 s for the whole stream) |

The import is 10,000 reference records with uids, run as one `import`, into
a copy of a workspace whose open months hold 20,000 reference episodes, with
its uid cache already built. Checking every line, including the duplicate
lookup against a `UIDSet` built once, takes a few hundred milliseconds.
Nearly all the rest is `CreateBatch` in file system calls: creating,
linking and removing each temporary file. The index is updated by one
splice per chunk.

Each file is synced with a plain `fsync`, and each chunk ends with one
`F_FULLFSYNC`, before the index names its files and the lock is released.
On macOS, `os.File.Sync` is `F_FULLFSYNC`, which also flushes the drive's
cache and costs about 3.4 ms a file. Paid per file, 10,000 files take 34 s
(20 s even with 32 writers in parallel), against about 0.4 ms each now.
Elsewhere `Sync` is already the full sync, and the barrier adds nothing.
Each chunk is still durable before it is reported written. A single
`record` is unchanged.

`list --ndjson --fields index` reads the root's head, then one month's
segment at a time as the ids reach it, and parses the root's entries only
when it reaches an id that no segment holds. When the episodes span two
years, the first line waits only on the directory listing and the first
segment. When all 100,000 are in the open months, it waits on parsing the
whole 20 MB root, which is still under the target.

## 2026-11-14, after `uid-cache` (#3)

| Benchmark | Target | Before | After |
|---|---|---|---|
| `BenchmarkDuplicateLookup20k` (cache built) | under 100 ms | 815 ms | **52 ms** |
| `BenchmarkDuplicateLookup20kCold` (no cache) | same result as a full parse | 815 ms | 922 ms |
| `BenchmarkDuplicateLookup20kFullParse` (the old lookup, for comparison) | | | 895 ms |

Each warm iteration is a fresh process's view: open the workspace, list it,
read and decode the cache (about 1 MB of JSON for 20,000 episodes),
reconcile the cached names with the listing, and verify the one candidate
against its file. A profile splits the time roughly evenly between the
directory listing, decoding the cache, and reconciling names. A cold lookup
still parses every file once and then writes the cache, so the first
lookup in a fresh clone costs what every lookup used to.

## 2026-11-14, after `fast-index-upsert` (#2)

| Benchmark | Target | Before | After |
|---|---|---|---|
| `BenchmarkCreate20k` | under 100 ms | 357 ms | **52 ms** |

A create that opens no month and whose id sorts last (almost every create)
now appends its one encoded entry to the root's bytes instead of parsing and
re-encoding all 20,000 entries. The full upsert still runs whenever a month
seals or the root's head or tail is not in the form this binary writes, and
a property test checks that both paths write identical bytes. A CPU profile
per create shows about 14 ms in the directory listing and 14 ms in `Create`
itself: about 4 ms checking the 4 MB root's bytes, 3 ms writing it
atomically, and the rest the episode write. The remainder of the 52 ms is
waiting on the file system, mostly the two `fsync`s. The directory listing
is now the largest CPU cost; listing only the newest month would remove most
of it.

## 2026-11-14, v0.1.0 (Apple M2 Pro, macOS, Go 1.26.7, APFS)

| Benchmark | What it measures | Target | Result |
|---|---|---|---|
| `BenchmarkCreate20k` | One `Create` (lock, directory listing, exclusive write, index upsert) into a workspace whose open months hold 20,000 entries | under 100 ms | **357 ms**: missed (fixed by #2, above) |
| `BenchmarkDuplicateLookup20k` | `record ref`'s duplicate lookup (list, parse every file, match `standpoint`, `media_type`, `uid`) over 20,000 reference episodes | follow-on if over 500 ms | **815 ms**: over (fixed by #3, above) |

### `Create`

A profile puts the time in the root index: re-encoding 20,000 entries with
`yaml.v3` (about 190 ms) and parsing them back on the next write (about
90 ms). The directory listing and the episode write itself are small. The
remedy is inside `store` and needs no format change, as D14 expects: either
a hand-written emitter for the fixed seven-field entry, or an
append-friendly root that does not rewrite every open-month entry on each
create. Until capture writes turns at a high rate, 357 ms per write is
tolerable for a CLI a harness calls once per turn.

### Duplicate lookup

`uid` is deliberately not in the index, so the lookup parses every file.
At 20,000 reference episodes this takes 815 ms, over the 500 ms threshold
task 10.2 set for opening a follow-on. The remedy the design names is a
`uid` cache kept outside the format (gitignored), rebuilt from the files and
never trusted over them.

Both are follow-ons rather than fixed here, because each touches the store's
write path and deserves its own design:
[#2](https://github.com/nodelogicau/episodes-cli/issues/2) (`Create`) and
[#3](https://github.com/nodelogicau/episodes-cli/issues/3) (duplicate lookup).
