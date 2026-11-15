## Context

`record ref` with a `uid` and no `--allow-duplicate` runs, under the workspace lock:

```go
query.Duplicates(c.ws.LoadAll(c.l), ep.Standpoint, mediaType, uid)
```

`LoadAll` reads and decodes every listed file. At 20,000 reference episodes this takes 815 ms (`BenchmarkDuplicateLookup20k`), nearly all of it in `model.Decode`. The listing (`Scan`) costs about 14 ms of that and is already taken for `Create`. `uid` is kept out of the index on purpose, because the index is shared, reviewed state and the format does not define a `uid` entry field.

Issue #3 left one question open: where the cache lives. **Decision:** the user cache directory, keyed by the workspace (D1).

Facts the design relies on:
- Episode file names are their ids, and an id is never reused. Two files with the same name in the same place are the same episode.
- By the accumulation rule, an episode's `standpoint`, `media_type` and `uid` never change after it is written. The only allowed change to them is `redact` removing `content.uid` (it is in the closed set).
- `query` imports `store`, so the matching rule (`uid` compared exactly, `media_type` compared without ASCII case) stays in `query.Duplicates`.

## Goals / Non-Goals

**Goals:**
- `BenchmarkDuplicateLookup20k` with a warm cache under 100 ms.
- A cold or missing cache gives the same result as a full parse.
- A stale cache never causes a false refusal, and never misses an episode that was merged in or created by another process.
- Nothing new in the workspace tree. No format change.

**Non-Goals:**
- Speeding up `validate` (its `possible_duplicate` check is the invariant and keeps parsing every file) or recall.
- Detecting episodes revised in place. That is `validate`'s and review's job.
- Cleaning up caches of workspaces that no longer exist.

## Decisions

### D1. The cache lives in the user cache directory

Path: `<base>/episodes/uid/<workspace.id>-<h>.json`, where:
- `<base>` is `$EPISODES_CACHE_DIR` if set, else `os.UserCacheDir()`;
- `<h>` is the first 12 hex digits of the SHA-256 of the workspace root's absolute path, after `filepath.EvalSymlinks`.

The path is part of the key because two clones of one workspace share `workspace.id` but hold different files. The file also records the path and id it was built for, and a cache whose recorded path or id differs from the workspace is treated as missing.

If neither `EPISODES_CACHE_DIR` nor `os.UserCacheDir()` gives a directory, there is no cache, and the lookup is today's full parse.

*Alternative considered:* `.episodes-cache/` inside the workspace with a self-ignoring `.gitignore`. It was rejected because it adds a directory to a tree people review, `git clean -x` wipes it, and a tool that performs no git operations shouldn't rely on git's ignore rules.

### D2. Contents: per directory, the names parsed and the refs found

```json
{
  "cache": "episodes-uid/1",
  "workspace": {"id": "…", "root": "/abs/path"},
  "dirs": {
    "2026-11": {
      "names": ["epi_…", "…"],
      "refs": [{"id": "epi_…", "standpoint": "…", "media_type": "message/rfc822", "uid": "mid:…"}]
    }
  }
}
```

- Directories are keyed as the listing keys them (`Listing.ByDir`: the month shard, or the legacy shard). `names` holds every file in the directory that parsed. It includes files with no `uid`, so they are not parsed again.
- `refs` holds the reference episodes that carry a `uid`. `media_type` is stored lowercased.
- A file that fails to parse is left out of `names`, so it is retried on every lookup until it parses or is removed.
- The cache is JSON, written with `encoding/json`. It is a private, machine-local file, and `encoding/json` decodes 20,000 entries several times faster than yaml.v3. A different `cache` value, or a file that doesn't decode, is treated as missing.

### D3. Freshness comes from the listing, not from timestamps

Each lookup reconciles the cache with the listing the caller already holds:
- each listed directory missing from the cache is parsed;
- each listed file whose name isn't in its directory's `names` is parsed and added;
- each cached name no longer listed is dropped, along with its refs;
- each cached directory no longer listed is dropped.

The cost is a set difference over names that are already in memory, plus a parse of each new file only. There is no `stat` per file and no use of modification times, so a clock, a checkout or a copy can't mislead it.

Given D4, this listing check is enough. A file can change without its name changing in two ways:
- `redact` removes `uid`. D4 drops that file's stale ref.
- A hand edit adds or changes `uid`, `standpoint` or `media_type`. That is a revision, which accumulation forbids. `validate` reports it in sealed months, and `possible_duplicate` reports any duplicate it creates. This is the one case the cache can miss, and the spec states it.

### D4. Every candidate is verified against its file

`store` returns candidate ids whose cached `(standpoint, lower(media_type), uid)` matches. The CLI loads each candidate's file with `LoadFile` and passes only those to `query.Duplicates`, which applies today's rule. As a result:
- a cached ref whose file was redacted, or is unreadable, is not reported;
- what is reported is exactly what a full parse would report, restricted to the candidates.

So a stale cache can produce a false negative only through D3's revision case, and can never produce a false refusal. Verifying candidates costs one parse per existing duplicate, which is normally zero or one.

### D5. Writes happen under the workspace lock, and failures are ignored

The lookup runs inside `a.write`, so it already holds the lock. After reconciling, the cache is saved if anything changed. After a successful `Create`, the CLI adds the new episode's name and ref, and saves again.

Saving writes a temporary file in the cache directory and renames it over the cache, with no `fsync` (the cache is disposable). Two writers of the same workspace can't overlap, because both hold the workspace lock. A different clone uses a different file (D1).

An error creating the directory, reading, decoding or writing the cache never fails the verb. Reading or decoding errors become a rebuild, and write errors are dropped. A rebuild costs one full parse, the same as today.

### D6. API shape

In `internal/store/uidcache.go`:

```go
// UIDCandidates returns the listed files whose cached standpoint,
// media_type and uid match, after reconciling the cache with l. It never
// returns an error: on any cache failure it falls back to parsing.
func (w *Workspace) UIDCandidates(l *Listing, standpoint, mediaType, uid string) []FileRef
// NoteUID adds a just-created episode to the cache, if this process loaded it.
func (w *Workspace) NoteUID(ep *model.Episode)
```

The cache is loaded once per process and held on the `Workspace` as an unexported field. Candidates are returned as `FileRef`s from the listing, so the CLI can verify them with `LoadFile` even when a file's name and its `id` disagree.

`cmd_record.go` replaces `LoadAll` with `UIDCandidates`, then `LoadFile`, then `query.Duplicates`, and calls `NoteUID` after `c.create` succeeds. *As built:* `NoteUID` does nothing when this process didn't load the cache (`--allow-duplicate`, or no `uid`), because loading it only to add one name would cost more than the next lookup's parse of that one new file.

## Risks / Trade-offs

- **[Revised-in-place files missed]** → This is stated in the spec. It can only arise from a hand edit that breaks accumulation. `validate` still parses everything and reports `possible_duplicate` and `revised`.
- **[Caches accumulate in the user cache dir]** → They are small (about 1 MB per 20,000 episodes), the OS may purge them, and the README says deleting the directory is always safe.
- **[The first lookup after a large merge is slow]** → It parses only the new files. A fresh clone pays today's cost once.
- **[Tests writing to the real user cache]** → The CLI test harness sets `EPISODES_CACHE_DIR` to a temporary directory, alongside the other `EPISODES_*` variables it already clears.
- **[A symlinked or moved workspace]** → A move gives a new key and a cold cache: correct, just slower once.

## Migration Plan

None. There is no cache before this change, so the first lookup builds one. Rolling back leaves an unused file in the user cache directory.

## Open Questions

None. The cache location was decided as D1.
