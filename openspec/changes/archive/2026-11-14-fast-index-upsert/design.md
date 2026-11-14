## Context

`Workspace.upsert` in `internal/store/index.go` reads `index.yaml` with `ReadRoot` (yaml.v3, every entry through `model.DecodeNode`), seals any month the new id seals, appends the new entry, and writes the root with `EncodeRoot`. Measured at 20,000 open-month entries (4 MB root, Apple M2 Pro):

| Step | Time |
|---|---|
| `Scan` (directory listing) | 23 ms |
| read the root's bytes | 0.5 ms |
| parse the root | 130 ms |
| encode the root | about 190 ms (profile) |
| atomic write of 4 MB | 8 ms |

Parsing and encoding make up about 90% of the 357 ms. Ids are UUIDv7 minted under the workspace lock, so a new id is almost always larger than every id already in the root.

## Goals / Non-Goals

**Goals:**
- `BenchmarkCreate20k` under 100 ms.
- Root bytes identical to today's for every sequence of creates.
- No change to the format, the entry shape, `index`, `index --check` or `validate`.

**Non-Goals:**
- Making the full path faster (sealing, rebuild, `index`). These are rare.
- Recall's cost (`list`, `thread` parse the root too). That is a separate concern, but a later change could reuse the same tail-reading idea.
- Batching for bulk import (#1).

## Decisions

### D1. Splice one encoded entry onto the root's bytes

On the fast path, the new entry is encoded on its own: a one-entry root through the existing `EncodeRoot`, with the entry's block lines taken from its `entries:` sequence. Those lines are then appended to the root's bytes. Using the same encoder means the entry's bytes are what a full encode would produce. yaml.v3 emits each block-sequence item independently of its neighbours, at a fixed indent.

**Alternative rejected:** a hand-written emitter for the seven-field entry (issue #2's option 1). It removes the encode cost but not the parse, and it must re-implement yaml.v3's quoting rules for `standpoint` and `kind` exactly. The splice needs neither.

### D2. The fast path's preconditions, checked on bytes

The fast path is taken only when all of these hold, checked without parsing the entries:

1. Nothing seals: every sealed month that has files already has its segment listed in the root. `Create` adds the new file to the listing before the upsert, so this is checked against the listing as it now stands. A create that opens a month leaves the months it seals unlisted, so it fails this check. (An earlier draft of this item compared the new id's month with the listing's newest month, which always passes after `Create` updates the listing.)
2. The root's head (everything before `entries:`) is byte-identical to what `EncodeRoot` writes for the segments it lists, its `format` is `episodes/0.1`, and the root holds no line beginning `<<<<<<<`, `=======` or `>>>>>>>`.
3. Its `entries:` is a block sequence (not `[]`), the file ends with a newline, and the last entry's id (from the last `\n  - id: ` line) sorts before the new id.
4. The first entry's id (from the first `\n  - id: ` line) is in an open month, so the full path would drop nothing.

If any check fails, the full path runs. An empty root (`entries: []`) takes the full path: it is cheap, and it changes the sequence's flow form to block.

### D3. What "canonical form" means, and what it leaves out

Every root the binary writes satisfies D2, so the property test (D4) compares the paths over the binary's own roots. A root that passes the byte checks but was not written by the binary might differ: for example, one with unsorted entries in the middle, entries in a non-canonical quoting style, or an unparseable entry in the middle, which the full path would have answered with a rebuild. The full path would re-sort or re-quote such a root, and the fast path appends to it. The spec scopes equivalence to the canonical form for this reason. Such a root is still a valid cache, and `index` or the next sealing create canonicalises it.

### D4. Equivalence is tested, not argued

The package gets a test-only switch that forces the full path. A property test seeds a workspace, then applies about 200 creates with ids drawn across several months. Some ids are out of order, and some fall in the newest month after it seals. Each create runs in two workspaces, one per path, and the test compares the root bytes after every step. It also covers each D2 fallback: conflict markers, an empty root, an id that sorts first, and a root whose first entry is in a sealed month.

### D5. No change to locking or atomicity

The fast path reads the root's bytes once, builds the new bytes in memory, and writes them with `writeAtomic`, all under the same lock. It never appends to the file in place, so an interrupted write still leaves the old root.

## Risks / Trade-offs

- [The spliced bytes differ from a full encode in some case the test misses] → D4's property test runs on every CI build. The fallback conditions are conservative, and `index --check` would report any drift.
- [The 23 ms directory listing becomes the larger cost] → Still within target. A later change can list only the newest month's directory when the root is trusted.
- [A hand-edited root takes the fast path and stays non-canonical] → D3: it is still valid, and the next full upsert or `index` canonicalises it.

## Migration Plan

None. Roots are byte-identical. Rollback is reverting the commit.

## Open Questions

None.
