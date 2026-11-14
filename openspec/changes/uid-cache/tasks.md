## 1. Cache

- [x] 1.1 `internal/store/uidcache.go`: the cache path (D1: `$EPISODES_CACHE_DIR` or `os.UserCacheDir()`, `episodes/uid/<workspace.id>-<sha256(abs root)[:12]>.json`), and the JSON shape with `cache: episodes-uid/1` and the workspace id and root (D2)
- [x] 1.2 Load: treat missing, undecodable, wrong-version or foreign-workspace files as empty; hold the loaded cache on `Workspace` for the process
- [x] 1.3 Reconcile with a `Listing` (D3): parse unseen names per directory, drop gone names and directories, leave unparseable files out of `names`
- [x] 1.4 `UIDCandidates(l, standpoint, mediaType, uid)`: reconcile, match on `(standpoint, lower(media_type), uid)`, save if changed; never return an error
- [x] 1.5 `NoteUID(ep, rel)`: add a created episode's name (and its ref when it carries `uid`) and save
- [x] 1.6 Save atomically in the cache directory without `fsync`; drop every cache I/O error (D5)

## 2. CLI

- [x] 2.1 `cmd_record.go`: replace `LoadAll` with `UIDCandidates` → `LoadFile` per candidate → `query.Duplicates` (D4); call `NoteUID` after a successful create, with or without `--allow-duplicate`
- [x] 2.2 CLI test harness: set `EPISODES_CACHE_DIR` to a temp dir for every test, alongside the `EPISODES_*` variables it clears

## 3. Tests

- [x] 3.1 Store: cold cache equals a full parse over a random mix of ref, inline and uid-less episodes across months
- [x] 3.2 Store: a file written behind the cache's back (simulated merge) is found; a removed file is dropped
- [x] 3.3 Store/CLI: a cached ref whose `uid` was redacted is not reported
- [x] 3.4 Store: corrupt JSON, wrong `cache` version, and a cache built for another root are each rebuilt
- [x] 3.5 CLI: an unwritable `EPISODES_CACHE_DIR` still gives the full-parse result; nothing new appears in the workspace tree
- [x] 3.6 Existing CLI, store and upstream suites pass unchanged

## 4. Benchmark and docs

- [x] 4.1 `BenchmarkDuplicateLookup20k`: warm (cache primed) and cold variants measuring list, then lookup, then verify; confirm warm is under 100 ms
- [x] 4.2 `docs/benchmarks.md`: results; `README.md`: `EPISODES_CACHE_DIR` and where the cache lives, and that deleting it is safe; `CHANGELOG.md`: Unreleased entry referencing #3
