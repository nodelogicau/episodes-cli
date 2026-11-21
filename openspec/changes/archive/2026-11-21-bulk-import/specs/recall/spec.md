## ADDED Requirements

### Requirement: Stream episodes as NDJSON
`episodes list --ndjson [--fields full|index]` SHALL write one JSON object per episode per line. It SHALL NOT be combined with `--json`. `--fields full` (the default) SHALL write whole episodes, leaving out withdrawn ones unless `--include-withdrawn`. `--fields index` SHALL write exactly the seven indexed fields and `path`, SHALL parse no episode file the index already covers, and SHALL NOT filter out withdrawn episodes, because the index cannot tell them apart. Both SHALL follow *Recall is never wrong for a stale index*. With `--ndjson`, the default order SHALL be `--by recorded`. Memory SHALL grow with the number of entries, not with the size of their content.

#### Scenario: Index fast path
- **WHEN** `list --ndjson --fields index` is run over 100,000 indexed episodes
- **THEN** the first line is written in under 1 s and no episode file is opened

#### Scenario: Not mixed with JSON
- **WHEN** `list --ndjson --json` is run
- **THEN** it exits with a usage error

#### Scenario: Stale index streamed
- **WHEN** a merged file is missing from `index.yaml` and `list --ndjson --fields index` is run
- **THEN** that file is parsed and its entry appears in the stream
