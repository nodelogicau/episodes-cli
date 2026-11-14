## MODIFIED Requirements

### Requirement: Record a self-attested act
`episodes act [--kind K] --occurred V --basis stated|inferred [--end V] [--label L] [--place P] [--via epi_T | --control]` SHALL write an episode with `content: {mode: none}`, `kind` defaulting to `activity`, and `standpoint` equal to the resolved `source.author`. The JSON result SHALL carry `attestation`: `via` when `--via` was given, `control` when `--control` was given, and otherwise `author`. Nothing about the attestation other than the `via` link SHALL be written to the file.

#### Scenario: Deep work logged after the fact
- **WHEN** at 10:31 `act --occurred "…T09:00:00+11:00[Australia/Melbourne]" --end "…T10:30:00+11:00[Australia/Melbourne]" --basis stated --label "Deep work on the budget draft" --via epi_T` is run
- **THEN** the file carries `kind: activity`, `content: {mode: none}`, `links: [{rel: via, href: epi_T}]`, and a standpoint equal to `source.author`, and the result carries `attestation: via`

#### Scenario: A control writes no marker
- **WHEN** `act … --control` succeeds
- **THEN** the file is byte-identical to the one the same act would produce without `--control` and with no harness, apart from `source` and the id, and it carries no `links`

## REMOVED Requirements

### Requirement: A harness names the attestation
**Reason**: Whether a harness field is present is not how the act was attested. The rule refused a person's own act when a harness came from `EPISODES_HARNESS` or `defaults.source.harness`, and a dedicated control built as a connector, which the draft allows to omit `via` (issue #4, nodelogicau/episodes#25).
**Migration**: Callers that pass `--via` are unchanged. A harness or connector recording an act from a dedicated control passes `--control`. See *The attestation is named*.

## ADDED Requirements

### Requirement: The attestation is named
`act` SHALL accept at most one of `--via <epi>`, meaning the attestation is that episode, and `--control`, meaning the attestation was a dedicated control whose only purpose is to log the act and no episode holds it. Giving both SHALL be refused with exit 2. When the resolved source carries `harness` and neither is given, the write SHALL be refused with exit 2, and the message SHALL name both `--via` and `--control`. When the resolved source carries no `harness`, neither SHALL be required. `--control` is the caller's statement and SHALL be documented as one that the binary cannot check.

#### Scenario: Harness with neither flag
- **WHEN** `act … --harness claude` is run without `--via` or `--control`
- **THEN** the write is refused with exit 2, and the message names `--via` and `--control`

#### Scenario: Harness from the workspace defaults with a control
- **WHEN** `act … --control` is run in a workspace initialised with `--harness claude`, with no `--harness` flag and no `EPISODES_HARNESS`
- **THEN** the write succeeds, the file carries no `links`, and the result carries `attestation: control`

#### Scenario: Connector with a control
- **WHEN** `act … --harness run-logger --control` is run
- **THEN** the write succeeds and the file's `source.harness` is `run-logger`

#### Scenario: Both flags refused
- **WHEN** `act … --via epi_T --control` is run
- **THEN** the write is refused with exit 2 and nothing is written

#### Scenario: Person's own act
- **WHEN** `act …` is run with an author, no harness, and neither flag
- **THEN** the write succeeds and the result carries `attestation: author`
