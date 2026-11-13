## ADDED Requirements

### Requirement: Observe a reference episode
`episodes observe <id> (--state available|changed|unreachable|gone | --content-file F [--version V] | --version V)` SHALL write or rewrite `source_status` on a reference episode. It SHALL be refused on an inline episode or an episode with `mode: none`.

#### Scenario: Stated state
- **WHEN** `observe epi_E --state unreachable` is run
- **THEN** the file gains `source_status: {state: unreachable, verified: <now>, detected: <now>}` and nothing else in it changes

#### Scenario: Observe an inline turn
- **WHEN** `observe` is run on an inline episode
- **THEN** the command exits 2

### Requirement: State computed from fetched content
With `--content-file`, the state SHALL be `available` when the computed hash equals `content.hash` and `changed` when it differs. If the episode has no `hash` but has `version`, `--version` SHALL be compared with it instead. If neither can be compared, the command SHALL be refused.

#### Scenario: Edited document
- **WHEN** `observe epi_D --content-file now.md` is run and the file's hash differs from `content.hash`
- **THEN** `source_status.state` is `changed`

#### Scenario: Nothing to compare
- **WHEN** `observe epi_X --content-file f` is run on an episode with neither `hash` nor `version`
- **THEN** the command exits 2

### Requirement: verified and detected
`verified` SHALL be set to now on every observation. `detected` SHALL be set to now when the state differs from the recorded state or none was recorded, and SHALL otherwise be kept.

#### Scenario: Repeated observation
- **WHEN** an episode recorded as `gone` on day 1 is observed as `gone` again on day 5
- **THEN** `verified` is day 5 and `detected` is still day 1

### Requirement: No back-fill
Observing SHALL NOT add, change or remove any content field, and SHALL NOT redact anything.

#### Scenario: Source gone
- **WHEN** an episode is observed as `gone`
- **THEN** its `content` block is byte-identical to before
