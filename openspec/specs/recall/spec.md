# recall Specification

## Purpose
`show`, `list` and `thread`. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: Show an episode
`episodes show <id> [--children]` SHALL print the episode, locating its file by the path its id derives and without reading the index. `--children` SHALL also list the episodes naming it as container, in ordinal order and then by id.

#### Scenario: Show with children
- **WHEN** `show epi_C --children` is run on a conversation with three turns
- **THEN** the container and its three turns are printed, ordinals 1 to 3

### Requirement: List episodes
`episodes list [--since V] [--until V] [--standpoint S] [--kind K] [--container C] [--include-withdrawn] [--by occurred|recorded] [--limit N]` SHALL list matching episodes. By default it SHALL order them by the `occurred.start` instant (a date alone counted as the start of that day in `tz`, or else UTC), with ties broken by id. `--by recorded` SHALL order by id. `--since` and `--until` SHALL select episodes whose occurred interval overlaps the range.

#### Scenario: What Ada said last week
- **WHEN** `list --standpoint https://example.com/ada --since 2026-11-02 --until 2026-11-09` is run
- **THEN** only Ada's episodes that occurred in that range are listed, in occurred order

### Requirement: Recall is never wrong for a stale index
`list` and `thread` SHALL enumerate episode files by directory listing, use index entries only for ids the index holds, parse any file the index lacks, and drop entries whose file is absent.

#### Scenario: Index lagging after a merge
- **WHEN** a merged branch added files without updating `index.yaml` and `list` is run
- **THEN** the merged episodes appear in the result

### Requirement: Thread a container
`episodes thread <container>` SHALL print the container's children in ordinal order, then by id, each with its standpoint, occurred time and text. A withdrawn turn (its `content.text` redacted) SHALL appear only as a placeholder line at its position.

#### Scenario: Thread with a withdrawn turn
- **WHEN** turn 2 of 3 has had `content.text` redacted and `thread` is run
- **THEN** turns 1 and 3 are printed with their text, and position 2 shows a placeholder with no text

### Requirement: Withdrawn and tombstoned episodes
`list` SHALL leave out inline episodes whose `content.text` is redacted unless given `--include-withdrawn`. A reference episode whose `content.ref` is redacted SHALL be listed and marked as a tombstone. No verb SHALL output a redacted field.

#### Scenario: Withdrawn excluded
- **WHEN** a note's text is redacted and `list --kind note` is run
- **THEN** the note is not listed, and it is listed with `--include-withdrawn`

### Requirement: Absence is not reported as evidence
No verb SHALL report that something did not happen. An empty result SHALL be reported only as no episodes found.

#### Scenario: Empty interval
- **WHEN** `list --since … --until …` matches nothing
- **THEN** the result is an empty list with no statement about what happened in that interval

