# self-attested-acts Specification

## Purpose
The `act` verb, recognition by shape, the standpoint and `via` rules, and timing from the act. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: Record a self-attested act
`episodes act [--kind K] --occurred V --basis stated|inferred [--end V] [--label L] [--place P] [--via epi_T]` SHALL write an episode with `content: {mode: none}`, `kind` defaulting to `activity`, and `standpoint` equal to the resolved `source.author`.

#### Scenario: Deep work logged after the fact
- **WHEN** at 10:31 `act --occurred "…T09:00:00+11:00[Australia/Melbourne]" --end "…T10:30:00+11:00[Australia/Melbourne]" --basis stated --label "Deep work on the budget draft" --via epi_T` is run
- **THEN** the file carries `kind: activity`, `content: {mode: none}`, `links: [{rel: via, href: epi_T}]`, and a standpoint equal to `source.author`

### Requirement: Self-attestation only
An act SHALL be refused when no `source.author` is resolved, when `--standpoint` is given and differs from it, or when `--via` names an episode whose `standpoint` differs from it.

#### Scenario: Someone else's act refused
- **WHEN** `act --standpoint https://example.com/jane …` is run with author Ada
- **THEN** the write is refused with exit 2

#### Scenario: Via the assistant's turn refused
- **WHEN** `act --via epi_T` names a turn whose standpoint is `https://claude.ai`
- **THEN** the write is refused with exit 2

### Requirement: A harness names the attestation
When the resolved source carries `harness`, `act` SHALL require `--via`, naming the episode in which the act was attested.

#### Scenario: Harness without via
- **WHEN** `act … --harness claude` is run without `--via`
- **THEN** the write is refused with exit 2, and the message says to record the attesting turn first

#### Scenario: Person's control without via
- **WHEN** `act …` is run with an author and no harness, and without `--via`
- **THEN** the write succeeds

### Requirement: Recognised by shape in validation
`validate` SHALL treat as a self-attested act every episode with `content.mode: none` that no episode names as its `container`, whatever its `kind`. It SHALL report an error when such an episode's `standpoint` differs from its `source.author`, or when its `via` link resolves to an episode with a different standpoint. An unresolved `via` SHALL be info only.

#### Scenario: Act with a mismatched standpoint
- **WHEN** a hand-written `mode: none` episode with no children has `standpoint` Jane and `source.author` Ada
- **THEN** `validate` reports an error

#### Scenario: Childless conversation container
- **WHEN** a `kind: conversation` episode with `mode: none` has no children
- **THEN** `validate` applies the act rules and also reports info `container_without_children`, naming `conversation turn` as the remedy

