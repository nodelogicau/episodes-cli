## Why

The format now defines *Recall by query* (upstream change `recall-by-query`, nodelogicau/episodes#38–#42). It is optional, but an implementation that offers it has to follow one model-free baseline. Today episodes-cli can only recall by when, whose, kind and container. You cannot ask "what did we say about the invoice?" without opening every file yourself ([#16](https://github.com/nodelogicau/episodes-cli/issues/16)).

## What Changes

- New verb `episodes query <text> [--since V] [--until V] [--standpoint S] [--kind K] [--container C] [--participant P] [--limit N]`.
- Baseline matching, with no model:
  - Trim `White_Space` from the edges of the query. Terms are runs of non-whitespace.
  - Strip leading and trailing characters of Unicode general category P from each term and from the whole query, before folding. Interior characters stay.
  - A query or term is a hit when it occurs, under Unicode case folding, in `label` or in an inline episode's `content.text`. Match by OR. No stemming and no stopwords.
  - A term equal to `content.uid`, exactly and with no folding, also matches. Equal before trimming or after both count.
  - An empty query resolves nothing and is not an error.
- Each result is one episode, carrying `container`. It has a `resolution` of `uid` or `lexical`, and `uid` wins when both apply. Results are ordered by resolution. Within a resolution, they're ordered by the number of distinct folded terms matched, most first, then by `id` descending by code point. `--limit` is applied last.
- A container is returned only when it matched itself, never because one of its children did.
- The filters are applied after matching, from the index and the files and never from a cache. `--participant` reads files and compares as written.
- A withdrawn episode is never returned. A match counts only while the file still carries the matched field, so a redacted `label`, `content.text` or `content.uid` never matches.
- No semantic stage, no cache and no remote embedding. These are optional upstream and out of scope here. Leaving them out also means `redact` has nothing derived to clear.

## Capabilities

### New Capabilities
- `query`: the `query` verb, its matching baseline, its result shape and order, and its filters.

### Modified Capabilities
<!-- none: recall's existing verbs are unchanged -->

## Impact

- New `internal/query/text.go` (normalising and matching terms) and `internal/cli/cmd_query.go`, with tests, including the upstream examples.
- A new dependency, `golang.org/x/text`, for full Unicode case folding (`cases.Fold`). The standard library has only simple per-rune folding.
- A query reads every episode file the index filters leave, because `label`, `content.text` and `content.uid` are not in the index. A benchmark over 20,000 episodes goes into `docs/benchmarks.md`.
- `README.md` (the verb) and `CHANGELOG.md`.
