# Benchmarks

Measured, not asserted in CI (design D14 of `bootstrap-episodes-cli`). Run
them with:

```sh
go test ./internal/store ./internal/query -run '^$' -bench . -benchtime 10x
```

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
| `BenchmarkDuplicateLookup20k` | `record ref`'s duplicate lookup (list, parse every file, match `standpoint`, `media_type`, `uid`) over 20,000 reference episodes | follow-on if over 500 ms | **815 ms**: over |

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
