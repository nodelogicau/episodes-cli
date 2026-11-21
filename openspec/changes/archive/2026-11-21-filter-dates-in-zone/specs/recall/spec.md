## MODIFIED Requirements

### Requirement: List episodes
`episodes list [--since V] [--until V] [--tz Z] [--standpoint S] [--kind K] [--container C] [--include-withdrawn] [--by occurred|recorded|ordinal] [--limit N]` SHALL list matching episodes. By default it SHALL order them by the `occurred.start` instant (a date alone counted as the start of that day in `tz`, or else UTC), with ties broken by id. `--by recorded` SHALL order by id. With `--container`, the default SHALL be `--by ordinal`: ascending `ordinal`, then by id compared by Unicode code point, with children that have no `ordinal` after those with one, ordered by id. `--by ordinal` without `--container` SHALL be a usage error. `--since` and `--until` SHALL select episodes whose occurred interval overlaps the range, each bound reckoned as *Date bounds in the reader's zone* says.

#### Scenario: What Ada said last week
- **WHEN** `list --standpoint https://example.com/ada --since 2026-11-02 --until 2026-11-09` is run
- **THEN** only Ada's episodes that occurred in that range are listed, in occurred order

#### Scenario: A container's children
- **WHEN** a container has children with ordinals 1, 3, 5 and 5 (two branches merged) and one with no ordinal, and `list --container` is run
- **THEN** they are listed 1, 3, then the two 5s by id, then the one with no ordinal

## ADDED Requirements

### Requirement: Date bounds in the reader's zone
A `--since` or `--until` given as a date alone SHALL be reckoned in the reader's zone. That zone SHALL be `--tz`, then `EPISODES_ZONE`, then the workspace's `defaults.zone`, then UTC. `--since D` SHALL begin at the start of day `D` in that zone, and `--until D` SHALL end at the start of the next day in that zone. A bound given as a date-time SHALL be its own instant whatever the zone. `--tz` given when neither bound is a date alone SHALL be a usage error. A zone that is not a known IANA zone SHALL be a usage error that names where the zone came from (`--tz` or `EPISODES_ZONE`). When either bound is a date alone, the JSON result SHALL carry `zone`, the IANA name of the zone used, or `UTC`; otherwise it SHALL NOT carry `zone`. `--ndjson` output, which has no envelope, SHALL NOT carry it. Stored `occurred` values SHALL be read as before: a stored date with no `tz` SHALL still be a UTC day.

#### Scenario: Since a local day
- **WHEN** a workspace has `defaults.zone: Australia/Melbourne`, an episode occurred at `2026-11-16T09:12:00+11:00`, and `list --since 2026-11-16` is run
- **THEN** the episode is listed, and the JSON result carries `zone: Australia/Melbourne`

#### Scenario: Until a local day
- **WHEN** a workspace has `defaults.zone: Australia/Melbourne`, an episode occurred at `2026-11-17T08:00:00+11:00` (21:00 UTC on the 16th), and `list --until 2026-11-16` is run
- **THEN** the episode is not listed

#### Scenario: --tz overrides the workspace
- **WHEN** the same Melbourne workspace runs `list --since 2026-11-16 --tz UTC`
- **THEN** the episode at `2026-11-16T09:12:00+11:00` is not listed, and `zone` is `UTC`

#### Scenario: EPISODES_ZONE before defaults.zone
- **WHEN** `EPISODES_ZONE` is `UTC` in the Melbourne workspace and `list --since 2026-11-16` is run
- **THEN** the bound is reckoned in UTC

#### Scenario: No zone anywhere
- **WHEN** no `--tz`, `EPISODES_ZONE` or `defaults.zone` is set and `list --since 2026-11-16` is run
- **THEN** the bound begins at `2026-11-16T00:00:00Z` and `zone` is `UTC`

#### Scenario: A date-time bound ignores the zone
- **WHEN** the Melbourne workspace runs `list --since 2026-11-16T00:00:00Z`
- **THEN** the bound is that instant, and the JSON result carries no `zone`

#### Scenario: --tz without a date
- **WHEN** `list --tz Australia/Melbourne` or `list --since 2026-11-16T00:00:00Z --tz Australia/Melbourne` is run
- **THEN** it is a usage error

#### Scenario: Unknown zone
- **WHEN** `list --since 2026-11-16 --tz Mars/Olympus` is run, or `EPISODES_ZONE` is `Mars/Olympus`
- **THEN** it is a usage error naming `--tz` or `EPISODES_ZONE`
