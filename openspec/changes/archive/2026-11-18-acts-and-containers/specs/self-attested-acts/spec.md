## MODIFIED Requirements

### Requirement: Record a self-attested act
`episodes act [--kind K] --occurred V --basis stated|inferred [--end V] [--label L] [--place P] [--via epi_T | --control]` SHALL write an episode with `content: {mode: attested}`, `kind` defaulting to `activity`, and `standpoint` equal to the resolved `source.author`. The JSON result SHALL carry `attestation`: `via` when `--via` was given, `control` when `--control` was given, and otherwise `author`. Nothing about the attestation other than the `via` link SHALL be written to the file.

#### Scenario: Deep work logged after the fact
- **WHEN** at 10:31 `act --occurred "…T09:00:00+11:00[Australia/Melbourne]" --end "…T10:30:00+11:00[Australia/Melbourne]" --basis stated --label "Deep work on the budget draft" --via epi_T` is run
- **THEN** the file carries `kind: activity`, `content: {mode: attested}`, `links: [{rel: via, href: epi_T}]`, and a standpoint equal to `source.author`, and the result carries `attestation: via`

#### Scenario: A control writes no marker
- **WHEN** `act … --control` succeeds
- **THEN** the file is byte-identical to the one the same act would produce without `--control` and with no harness, apart from `source` and the id, and it carries no `links`

#### Scenario: An act as a container
- **WHEN** `record inline --container <act id> …` is run
- **THEN** the write succeeds, and the act is still an act

## REMOVED Requirements

### Requirement: Recognised by shape in validation
**Reason**: Upstream change `acts-and-containers` replaced recognition by shape with the `attested` mode. A childless container is no longer read as an act.
**Migration**: Acts written with `mode: none` stay as they are, and now read as containers with no children: valid, with a `container_without_children` warning. See *Recognised by mode in validation*.

## ADDED Requirements

### Requirement: Recognised by mode in validation
`validate` SHALL apply the self-attestation rules to every episode with `content.mode: attested`, whatever its `kind` and whether or not it has children, and to no other episode. It SHALL report an error when an act's `standpoint` differs from its `source.author`, or when its `via` link resolves to an episode with a different standpoint. An unresolved `via` SHALL be info only.

#### Scenario: Act with a mismatched standpoint
- **WHEN** a hand-written `mode: attested` episode has `standpoint` Jane and `source.author` Ada
- **THEN** `validate` reports the error `act_standpoint_mismatch`

#### Scenario: An act with children is still an act
- **WHEN** a `mode: attested` episode with `standpoint` Jane and `source.author` Ada is named as `container` by another episode
- **THEN** `validate` still reports `act_standpoint_mismatch`

### Requirement: A childless container warns
An episode with `content.mode: none` that no episode in the workspace names as `container` SHALL be valid whatever its `standpoint` and `kind`. `validate` SHALL warn `container_without_children`, naming the episode, and SHALL NOT apply the act rules to it.

#### Scenario: Conversation whose first turn was lost
- **WHEN** a `kind: conversation` episode with `mode: none`, `standpoint` Ada and `source.author` Claude has no children
- **THEN** `validate` exits valid with the warning `container_without_children`, naming `conversation turn` as the remedy, and no `act_standpoint_mismatch`

#### Scenario: An act under the earlier draft
- **WHEN** an `activity` written with `mode: none` by an earlier build is validated
- **THEN** it is valid, with the warning `container_without_children`
