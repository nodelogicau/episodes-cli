## Why

`record ref` with a `uid` refuses a second reference episode with the same `standpoint`, `media_type` and `uid`. `uid` is deliberately not in the index, so the lookup lists and parses every episode file. At 20,000 reference episodes `BenchmarkDuplicateLookup20k` measures 815 ms ([#3](https://github.com/nodelogicau/episodes-cli/issues/3)). The cost grows linearly with the workspace and is paid under the write lock, so every other writer waits for it too.

## What Changes

- The duplicate lookup reads candidates from a derived `uid` cache instead of parsing every file. The cache lives outside the workspace, in the user cache directory, keyed by `workspace.id` and the workspace's absolute path. It is never committed, needs no `.gitignore`, and survives `git clean`.
- The cache is brought up to date from the directory listing on every lookup. It parses only files whose names it has not seen, and drops names that are gone. A missing, corrupt or foreign cache is rebuilt from the files.
- Every candidate the cache returns is checked against its file before it is reported, so a stale cache can never cause a false refusal. The one thing a stale cache can miss is a file revised in place to gain a matching `uid`, which accumulation forbids and `validate` still reports as `possible_duplicate`.
- After a successful `record ref`, the new episode is added to the cache.
- Failing to read or write the cache never fails the verb. The lookup falls back to a full parse.
- `EPISODES_CACHE_DIR` overrides the cache's base directory, for tests and for sandboxed harnesses.
- `validate`'s `possible_duplicate` check is unchanged and still parses every file.
- No change to the file format, the index, or any verb's output.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `recording`: adds a requirement that the duplicate lookup may use a cache kept outside the workspace, that its result agrees with a full parse for every file not revised in place, and that cache failures never fail a write.

## Impact

- New `internal/store/uidcache.go`, with tests. `internal/cli/cmd_record.go` calls it in place of `LoadAll` and `query.Duplicates`. The CLI tests set `EPISODES_CACHE_DIR` so they never touch the real user cache.
- `BenchmarkDuplicateLookup20k` gains warm and cold variants.
- `README.md` (environment and where the cache lives), `docs/benchmarks.md` and `CHANGELOG.md`.
- New files in the user cache directory (about 1 MB per 20,000 episodes). The tool never deletes caches of workspaces that no longer exist. Deleting the directory is always safe.
