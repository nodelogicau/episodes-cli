## Context

`recallFilter` (`internal/cli/cmd_recall.go`) parses `--since`/`--until` with `temporal.Parse` into `query.Filter`. `Filter.Match` (`internal/query/recall.go`) then calls `Since.Start(nil)` and `Until.End(nil)`, and `nil` means UTC. Both `list` and `query` call `recallFilter` before `openWorkspace`, so the workspace's `defaults.zone` isn't available at that point.

The writer side already has a zone order. `zoneFor` (`internal/cli/helpers.go`) uses `--zone`, then `EPISODES_ZONE`, then `defaults.zone`, never the host's. `record` and `act` take `--tz` for "the IANA zone a date alone is reckoned in". `--tz` is a usage error without a date.

## Goals / Non-Goals

**Goals:**
- A bare-date bound means the person's local day, and the result says which zone that was.
- `list` and `query` behave identically, through one shared function.
- No change to how stored values are read, because that is the format's rule.

**Non-Goals:**
- Reading the host's zone (`TZ`, `/etc/localtime`). The CLI never uses it on the writer side either, so the same workspace and environment give the same results on any machine.
- Changing `SortByOccurred` or the overlap rule for stored dates.
- A text-mode note of the zone. Text output stays as it is; `--json` and `--help` carry it.

## Decisions

**D1. Zone order: `--tz`, `EPISODES_ZONE`, `defaults.zone`, UTC.** This mirrors `zoneFor` with `--tz` in place of `--zone`. The alternative was `defaults.zone` only, as the issue proposed. `EPISODES_ZONE` is already "the person's zone" for writes, and a person who sets it would expect reads to follow. Leaving it out would make `record` and `list` disagree about what "today" is.

**D2. The flag is `--tz`, not `--zone`.** On `record`, `--zone` is the zone written for now, and `--tz` is the zone a date alone is reckoned in. A filter bound is a date alone being reckoned, so `--tz` is the matching name. As on `record`, `--tz` with no bare-date bound is a usage error. That catches `--tz` given with date-time bounds, which would otherwise be silently ignored.

**D3. `Filter` carries `Loc *time.Location`.** `Match` passes `f.Loc` to `Start` and `End` where it now passes `nil`. A nil `Loc` stays UTC, so existing callers and tests that build a `Filter` directly don't change. The alternative was converting the bounds to instants in the CLI and storing `time.Time`. That would change `Filter`'s shape and the date/date-time distinction `Match` relies on for an inclusive `--until` date-time.

**D4. `recallFilter` takes the workspace and the `--tz` value.** Its signature becomes `recallFilter(ws, since, until, tz, standpoint, kind, container) (query.Filter, string, error)`. The string is the zone name to report, or `""` when no bound is a bare date. `list` and `query` open the workspace before building the filter. `list` does flag-only usage checks (`--ndjson`/`--json`, `--fields`, `--by`) before opening the workspace, and those stay first. Parse errors for `--since`/`--until` can then come after a workspace error, which is acceptable since both exit with a usage-type failure.

**D5. Errors name the source.** A shared helper resolves the zone and returns `usageErr("--tz: …")` or `usageErr("EPISODES_ZONE: …")`. `defaults.zone` is already validated when the config loads (`store/config.go`), so it can't be invalid here. `zoneFor` keeps its own `--zone:` wording. Unifying the two is out of scope.

**D6. `zone` is reported only when a bare date was given.** It goes in the JSON envelope of `list` (`{episodes, count, zone}`) and `query` (`{query, episodes, count, zone}`). It's left out otherwise, so output for date-time-only or unfiltered calls doesn't change. `--ndjson` has no envelope, so it carries no `zone`. Its `--help` says how the zone is chosen.

## Risks / Trade-offs

- **[Silent behaviour change]** Anyone with `defaults.zone` or `EPISODES_ZONE` set, filtering by bare dates, gets different results. → Pre-1.0. The new results are what they meant. The CHANGELOG entry says so under a "Changed" heading, and `zone` in the JSON makes the reckoning visible.
- **[DST transitions]** A local day can be 23 or 25 hours. → `time.Date` in the location handles this, and the next-midnight end is computed the same way.
- **[Stored UTC dates vs local bounds]** A stored date with no `tz` is a UTC day, so it can overlap a local-day bound on either side. → This is correct under overlap semantics and follows the format. No mitigation needed, but a test pins it so it isn't "fixed" by accident later.
