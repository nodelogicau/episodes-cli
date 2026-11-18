## 1. Matching

- [ ] 1.1 Add `golang.org/x/text` and `query.fold` (`cases.Fold()`)
- [ ] 1.2 `internal/query/text.go`: `Terms(q)` returns the whole query, the trimmed terms and the raw terms (D1: `unicode.White_Space` trim and split, `unicode.IsPunct` edge strip, empty terms dropped)
- [ ] 1.3 `Match(ep, whole, terms, raw)` returns the resolution (`uid` | `lexical` | none) and the distinct terms matched (D2)
- [ ] 1.4 `SortResults` orders by resolution, then by `matched` descending, then by id descending (D4)

## 2. CLI

- [ ] 2.1 `internal/cli/cmd_query.go`: `query <text>` with `--since`, `--until`, `--standpoint`, `--kind`, `--container`, `--participant` and `--limit`. Candidates come from `recall()` (D3), then withdrawn episodes are dropped, then `--participant` is applied, then each episode is matched
- [ ] 2.2 Output: JSON `{query, episodes, count}`, with each item `listItem` plus `resolution` and `matched`; text lines carry their resolution; an empty result prints `No episodes found.` (D5)
- [ ] 2.3 Register the verb in `root.go`

## 3. Tests

- [ ] 3.1 `Terms`: edge whitespace (including NBSP and U+2028), edge punctuation, interior punctuation kept, an empty query
- [ ] 3.2 Matching: folding (`ß` matches `SS`), label vs text, a ref episode's text not searched, an exact uid with no folding, a redacted label, text or uid not matching
- [ ] 3.3 Order: uid before lexical, more terms first, id descending, `--limit` applied last
- [ ] 3.4 CLI: a container not returned through a child, a withdrawn episode never returned, `--participant`, an index lagging after a simulated merge, every scenario in `specs/query/spec.md`

## 4. Benchmark and docs

- [ ] 4.1 `BenchmarkQuery20k` (unfiltered, and with `--standpoint`); record it in `docs/benchmarks.md`
- [ ] 4.2 `README.md`: the `query` verb and its baseline; `CHANGELOG.md`: an Unreleased entry referencing #16
