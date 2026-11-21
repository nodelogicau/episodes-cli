## Why

`list --since 2026-11-16` reads the date as the UTC day, even in a workspace whose `defaults.zone` is `Australia/Melbourne`. So an email at 09:12 Melbourne time on the 16th is left out, though that's the first example a reader would try (#27). The format reckons a stored date with no `tz` in UTC, so that every implementation reads a record the same way. A filter flag isn't a stored value, though: it's the person's own query, and the day they mean is a day where they are.

## What Changes

- **BREAKING (pre-1.0):** a `--since` or `--until` given as a bare date is reckoned in the reader's zone: `--tz`, then `EPISODES_ZONE`, then `defaults.zone`, then UTC. A workspace with neither `EPISODES_ZONE` nor `defaults.zone` behaves exactly as before. A bound given as a date-time is unchanged, because it carries its own offset.
- `list` and `query` take `--tz Z`, the IANA zone a bare-date bound is reckoned in. This matches `--tz` on `record` and `act`, which is the zone a date alone is reckoned in. Giving `--tz` when neither bound is a bare date is a usage error, as it is for `record`. An unknown zone is a usage error that names where the zone came from.
- When a bound is a bare date, `list --json` and `query --json` report the zone used as `zone`. `--help` for both says how the zone is chosen.
- Stored values are untouched. A stored date with no `tz` is still a UTC day for overlap and for ordering, as the format requires.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `recall`: `list` takes `--tz`. A new requirement says how a bare-date `--since`/`--until` is reckoned, and that the zone used is reported.
- `query`: `query` takes `--tz` with the meaning it has for `list`.

## Impact

- `internal/cli/cmd_recall.go`: `recallFilter` resolves the zone, which needs the workspace config, and `list` gets `--tz` and the `zone` output.
- `internal/cli/cmd_query.go`: `--tz` and the `zone` output.
- `internal/query/recall.go`: `Filter` carries a location for bare-date bounds, and `Match` passes it to `Start` and `End` in place of `nil`.
- `internal/cli/helpers.go`: shared zone resolution with the source named in errors.
- README (verb table and recall section), CHANGELOG under Unreleased.
- Behaviour changes for anyone already filtering by date in a workspace that sets a zone. Their results now match local days.
