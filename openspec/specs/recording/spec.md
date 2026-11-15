# recording Specification

## Purpose
`record inline`, `record ref`, `conversation start` and `conversation turn`, containment and ordinal assignment, and duplicate detection on write. Introduced by change `bootstrap-episodes-cli`.
## Requirements
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

### Requirement: Duplicate lookup may use a cache outside the workspace
The duplicate lookup of *Duplicate references refused unless allowed* MAY read candidates from a derived cache. If it does, the cache SHALL NOT be written inside the workspace. It SHALL live under `$EPISODES_CACHE_DIR` when that is set, otherwise under the user cache directory, and SHALL be keyed by `workspace.id` together with the workspace's absolute path. Before each lookup the cache SHALL be reconciled with the directory listing: every listed file it has not seen is parsed, and every name no longer listed is dropped. Every candidate SHALL be checked against its file before it is reported. The lookup's result SHALL then equal a full parse's, except for an episode file revised in place to gain a matching `standpoint`, `media_type` or `uid`. A cache that is missing, unreadable, not decodable, or built for a different workspace SHALL be rebuilt from the files. A failure to read or write the cache SHALL NOT fail the verb. `validate` SHALL NOT use the cache.

#### Scenario: Warm cache
- **WHEN** the cache already covers 20,000 reference episodes and the same `mid:` as one of them is recorded again from the same standpoint
- **THEN** the write is refused with code `duplicate`, naming that episode, without parsing the other files

#### Scenario: A file merged in that the cache lacks
- **WHEN** a reference episode with a given `uid` arrives through a git merge after the cache was last written, and the same `uid` is then recorded from the same standpoint
- **THEN** the merged file is parsed during reconciliation and the write is refused, naming it

#### Scenario: A redacted uid
- **WHEN** the cache lists an episode under a `uid` that a later `redact` removed from its file
- **THEN** that episode is not reported as a duplicate

#### Scenario: Cold or corrupt cache
- **WHEN** the cache file is missing or does not decode
- **THEN** the lookup gives the same result as a full parse and writes a fresh cache

#### Scenario: Unwritable cache directory
- **WHEN** `EPISODES_CACHE_DIR` names a directory that cannot be created or written
- **THEN** `record ref` still succeeds or refuses exactly as a full parse would

#### Scenario: Nothing written in the workspace
- **WHEN** any `record ref` runs
- **THEN** no file is created in the workspace tree other than the episode and the index

