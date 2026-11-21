## Context

The upstream change `recall-by-query` (nodelogicau/episodes#38–#42) defines *Recall by query* as optional. An implementation that offers it follows a baseline with no model in it. Semantic stages, caches and remote embedding are left to the implementation.

Facts the design relies on:
- `label`, `content.text` and `content.uid` are not in the index (`store.Entry` holds id, kind, occurred, container, ordinal, standpoint and timestamp). Matching therefore has to parse the files.
- `recall()` in `internal/cli/cmd_recall.go` already filters on index fields, parses each survivor, and filters again on what the file says. That is the "file wins" rule, and `query` reuses it.
- `model.Episode.Withdrawn()` and `Tombstone()` already exist, and a redacted field is absent from the decoded episode.
- Parsing every file costs about 800 ms at 20,000 episodes (`BenchmarkDuplicateLookup20k`, cold).

## Goals / Non-Goals

**Goals:**
- Follow the baseline exactly, including the upstream examples, so two implementations give the same results for the same workspace.
- Never return a withdrawn episode, and never match on a field the file no longer carries.
- No new state: nothing derived from content is kept anywhere.

**Non-Goals:**
- A semantic stage, embeddings, or any search index or cache. If one is added later, it needs its own change, covering where the cache lives (outside the workspace), proposals checked against the file, and remote embedding off unless configured.
- Making a query faster than one parse of the candidate files.
- Stemming, stopwords, phrase syntax or boolean operators.

## Decisions

### D1. Normalising the query

The order is fixed, and it lives in `query.Terms(q string) (whole string, terms []string, raw []string)`:
1. Trim the edges of `q`: drop every rune with the `White_Space` property (`unicode.White_Space`, which is wider than `unicode.IsSpace`).
2. Split on runs of `White_Space` into the raw terms.
3. Strip leading and trailing runes of general category P (`unicode.IsPunct`, which is exactly category P) from each raw term and from the whole trimmed query. Interior runes stay, so `e-mail` and `o'clock` are kept whole.
4. Drop terms that are now empty. If the whole query is empty and so is every term, the query resolves nothing and the result is an empty list with exit 0.
5. Fold the whole query and each term with `cases.Fold()` from `golang.org/x/text`.

Using full case folding, so `ß` matches `SS`, is the deciding reason for the new dependency. `strings.EqualFold` and `unicode.SimpleFold` do simple folding only, and they can't search for a substring.

### D2. Matching one episode

For each parsed candidate:
- **uid:** a hit when the episode carries `content.uid`, and a raw or trimmed term equals it with no folding. The comparison is exact because a uid is an identifier, not text.
- **lexical:** fold `label` and, when `content.mode` is `inline`, `content.text`. A hit when the folded whole query, or any folded term, occurs as a substring of either.
- `matched` is the number of distinct folded terms that occur. A whole-query hit adds nothing on top of its terms.
- `resolution` is `uid` if the uid rule hit, else `lexical` if the lexical rule hit. Otherwise the episode is not a result.

A redacted field is absent from the decoded episode, so it can't match. That covers "a match counts only while the file carries the field" without any extra check.

### D3. Candidate set and filters

The spec says filters apply after matching. Every filter is a pure predicate on the same file, so applying the index filters first and re-checking them on the file gives the same set, and parses fewer files. `query` therefore calls `recall()` with `query.Filter{Since, Until, Standpoint, Kind, Container}` over `ws.Entries(l, 0, false)`. It then:
- drops withdrawn episodes, always (there is no `--include-withdrawn`);
- applies `--participant` against `participants` exactly as written;
- matches each episode (D2).

`--container` restricts results to that container's children. A container is never added because a child matched, and a child result carries its `container` id.

### D4. Order and limit

Results are sorted by:
1. resolution (`uid` before `lexical`);
2. `matched`, descending;
3. `id`, descending by code point (`>` on Go strings compares bytes, which for UTF-8 is code-point order).

`--limit N` truncates after sorting.

### D5. Output

- JSON: `{"query": q, "episodes": [{...listItem, "resolution": "...", "matched": n}], "count": n}`. A tombstone keeps the `tombstone` marker `listItem` already gives.
- Text: one `listLine` per result, prefixed with its resolution. An empty result prints `No episodes found.`, as `list` does, and makes no claim of absence.

## Risks / Trade-offs

- **[Cost grows with the workspace]** → Every query parses every candidate file. Narrowing filters (`--since`, `--standpoint`, `--kind`) cut that down, and a benchmark records the unfiltered cost. A faster path means a cache, which is deliberately out of scope here.
- **[Full folding changes lengths]** → Folding is applied to both sides before searching for a substring, so offsets never matter. Results report no match positions.
- **[Simple vs full folding]** → If upstream turns out to mean simple folding, only `ß` and other multi-rune folds change. That is a one-line swap behind `query.fold`.

## Open Questions

- Does "Unicode case folding" upstream mean full folding (assumed here) or simple folding?
- Does the uid rule apply to the whole query when it is a single token? Here it is applied to terms only, and a one-term query's term is the query.
