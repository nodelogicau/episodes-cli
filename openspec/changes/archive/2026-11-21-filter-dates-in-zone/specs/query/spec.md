## MODIFIED Requirements

### Requirement: Query filters and withdrawn episodes
`query` SHALL accept `--since`, `--until`, `--tz`, `--standpoint`, `--kind` and `--container` with the meanings they have for `list`, including *Date bounds in the reader's zone*, and `--participant P`, which keeps episodes whose `participants` include `P` as written. Filters SHALL be decided from the index and the episode files, never from a cache. A withdrawn episode SHALL never be returned.

#### Scenario: Withdrawn episode never returned
- **WHEN** a note whose `content.text` matched has had it redacted
- **THEN** `query` does not return the note, and no flag makes it do so

#### Scenario: Participant filter
- **WHEN** `query invoice --participant https://example.com/ada` is run
- **THEN** only matching episodes listing that participant exactly are returned

#### Scenario: Index lagging after a merge
- **WHEN** a merged branch added a matching episode without updating `index.yaml`
- **THEN** `query` returns it

#### Scenario: Since a local day
- **WHEN** a workspace has `defaults.zone: Australia/Melbourne`, a note reading "invoice" occurred at `2026-11-16T09:12:00+11:00`, and `query invoice --since 2026-11-16` is run
- **THEN** the note is returned, and the JSON result carries `zone: Australia/Melbourne`
