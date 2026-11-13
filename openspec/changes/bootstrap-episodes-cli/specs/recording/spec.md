## ADDED Requirements

### Requirement: Record an inline episode
`episodes record inline --kind K --standpoint S (--text T | --text-file F) [--media-type] [--derivation] [--language] [--participant P]… [--place] [--label] [--container C [--ordinal N]] [--link rel=href]… [--occurred V --basis B [--end V] [--tz Z]]` SHALL write one inline episode and print its id and path. Without `--occurred`, `occurred.start` SHALL be now with `basis: recorded`.

#### Scenario: A typed note
- **WHEN** `record inline --kind note --standpoint https://example.com/ada --text "Renewed the cert"` is run
- **THEN** one file is written under the current month's shard and the result carries its `id` and `path`

### Requirement: Record a reference episode
`episodes record ref --kind K --standpoint S --ref R --occurred V --basis B [--uid U | --message-id M] [--media-type] [--version] [--hash H | --hash-file F] [--language] [--label] [--participant P]… [--place] [--container C [--ordinal N]] [--link rel=href]… [--allow-duplicate]` SHALL write one reference episode. `--occurred` and `--basis` SHALL be required.

#### Scenario: A calendar arrangement
- **WHEN** `record ref --kind event --ref gcal:evt_7q --uid 7q2m@google.com --media-type text/calendar --version 3 --occurred "2026-11-07T16:02:00+11:00[Australia/Melbourne]" --basis stated --label "Budget review"` is run
- **THEN** the episode carries those values and no `text`

### Requirement: Start a conversation
`episodes conversation start [--kind K] --standpoint S (--text T | --text-file F) [turn flags]` SHALL, under one lock, write a container episode (`kind` default `conversation`, `content: {mode: none}`, `standpoint` from `--container-standpoint` or else the resolved `source.author`) followed by its first turn with `ordinal: 1`. It SHALL return both ids.

#### Scenario: Container and first turn together
- **WHEN** `conversation start --standpoint https://example.com/ada --text "hello"` is run
- **THEN** two files exist, the second carries `container: <first id>` and `ordinal: 1`, and the first carries `content: {mode: none}`

### Requirement: Append a turn
`episodes conversation turn <container> --standpoint S (--text T | --text-file F) [--ordinal N] [turn flags]` SHALL write one inline turn in the container. Without `--ordinal`, it SHALL assign one more than the highest ordinal among the container's existing children. `--standpoint` SHALL be required on every turn.

#### Scenario: Alternating speakers
- **WHEN** turns are appended by the person and then by `https://claude.ai`
- **THEN** they carry ordinals 2 and 3 and their respective standpoints

#### Scenario: Concurrent turns
- **WHEN** two `conversation turn` processes run against the same container at once
- **THEN** each receives a distinct ordinal

### Requirement: Containers resolve and are earlier
On write, `--container` SHALL name an existing episode minted before the new one. Otherwise the write SHALL be refused (exit 3 if it does not exist, exit 2 otherwise).

#### Scenario: Unknown container
- **WHEN** `conversation turn epi_nope …` is run
- **THEN** the command exits 3

### Requirement: Unique position in a container
A writer SHALL refuse a `(container, ordinal)` pair that an existing episode already holds, with exit 2 and error code `duplicate`, naming that episode.

#### Scenario: Replayed turn
- **WHEN** `conversation turn epi_C --ordinal 4 …` is run and a child of `epi_C` already has ordinal 4
- **THEN** the write is refused with code `duplicate`, naming the existing child

### Requirement: Duplicate references refused unless allowed
When `record ref` carries a `uid`, and an existing reference episode has the same `standpoint`, `media_type` and `uid` (with `uid` compared exactly), the write SHALL be refused with exit 2 and code `duplicate`, listing the existing ids, unless `--allow-duplicate` is given. Episodes with a different standpoint SHALL NOT count.

#### Scenario: Same email recorded twice
- **WHEN** the same `mid:` is recorded twice from Ada's standpoint
- **THEN** the second write is refused, naming the first episode

#### Scenario: Two people's copies
- **WHEN** the same `mid:` is recorded once from Ada's standpoint and once from Jane's
- **THEN** both writes succeed
