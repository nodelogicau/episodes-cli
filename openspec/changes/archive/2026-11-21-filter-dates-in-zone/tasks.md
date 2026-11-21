## 1. Filter

- [x] 1.1 Add `Loc *time.Location` to `query.Filter` and pass it to `Since.Start` and `Until.End` in `Match`; nil stays UTC (D3)
- [x] 1.2 Unit tests in `internal/query`: a bare-date `--since`/`--until` in `Australia/Melbourne` near the UTC day boundary, a date-time bound unaffected by `Loc`, and a stored date with no `tz` still read as a UTC day

## 2. Zone resolution

- [x] 2.1 Add a helper in `internal/cli/helpers.go` that resolves the reader's zone from `--tz`, `EPISODES_ZONE`, `defaults.zone`, then UTC, and returns the location and its name, with usage errors naming `--tz` or `EPISODES_ZONE` (D1, D5)
- [x] 2.2 Change `recallFilter` to take the workspace and `--tz`, set `Filter.Loc`, return the zone name when a bound is a bare date, and make `--tz` with no bare-date bound a usage error (D2, D4)

## 3. Verbs

- [x] 3.1 `list`: add `--tz`, open the workspace before building the filter (keeping the flag-only usage checks first), and add `zone` to the JSON envelope when reported, not to `--ndjson` (D6)
- [x] 3.2 `query`: add `--tz`, open the workspace before building the filter, and add `zone` to the JSON envelope when reported
- [x] 3.3 Update `--help` for `list` and `query` to say how a bare-date bound's zone is chosen

## 4. Tests

- [x] 4.1 CLI tests for each spec scenario in *Date bounds in the reader's zone*: since and until a local day, `--tz` override, `EPISODES_ZONE` before `defaults.zone`, no zone anywhere, date-time bound with no `zone`, `--tz` without a date, unknown zone from `--tz` and from `EPISODES_ZONE`
- [x] 4.2 CLI test for `query invoice --since 2026-11-16` in a Melbourne workspace, and that `--ndjson` carries no `zone`
- [x] 4.3 Run the #27 repro (`init --zone Australia/Melbourne`, `record ref … --occurred 2026-11-16T09:12:00+11:00`, `list --since 2026-11-16`) against a built binary

## 5. Docs

- [x] 5.1 README: `--tz` in the `list` and `query` rows and a sentence in the recall section on how bare dates are reckoned
- [x] 5.2 CHANGELOG: a "Changed" entry under Unreleased for the bare-date reckoning (#27), and "Added" for `--tz` on `list` and `query`
- [x] 5.3 `go test ./...` and `golangci-lint run ./...` pass
