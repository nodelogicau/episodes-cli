# links Specification

## Purpose
RFC 8288 typed links, the registered-relation check, write-once links, `href` resolution, and no restated containment. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: Link shape
`--link rel=href` SHALL be repeatable. Each link SHALL have exactly `rel` and `href`. `rel` SHALL be a lowercase name in the embedded IANA Link Relations snapshot, or an absolute URI. `href` SHALL be an episode id or an absolute URI. Links SHALL be written sorted by `rel` then `href`, with exact duplicates removed.

#### Scenario: Transcript via its recording
- **WHEN** `record inline --derivation transcribed --link via=epi_M …` is run and `epi_M` exists
- **THEN** the file carries `links: [{rel: via, href: epi_M}]`

#### Scenario: Unregistered rel refused
- **WHEN** `--link fulfils=int_X` is given
- **THEN** the write is refused with exit 2, naming the rel and suggesting an absolute URI

### Requirement: Episode targets resolve and are earlier
On write, an episode `href` SHALL name an episode that exists in the workspace and was minted before the new one. In `validate`, an episode `href` that does not resolve SHALL be reported at info level (`link_unresolved`) and SHALL NOT make the episode invalid.

#### Scenario: Link to a missing episode on write
- **WHEN** `--link predecessor-version=epi_nope` is given
- **THEN** the write is refused with exit 3

#### Scenario: Target dropped later
- **WHEN** the episode a link targets is removed from the workspace and `validate` is run
- **THEN** an info finding `link_unresolved` names both episodes and there is no error

### Requirement: Links are written once
No verb SHALL add, remove or change a link on an existing episode, except `redact` removing `links` whole.

#### Scenario: No link verb
- **WHEN** the verb list is inspected
- **THEN** no verb accepts an existing episode id together with `--link`

### Requirement: Containment is not restated
A writer SHALL refuse an `up` or `collection` link whose `href` is the episode's own `container`. `validate` SHALL warn on one (`link_restates_container`).

#### Scenario: Up to own container
- **WHEN** `conversation turn epi_C --link up=epi_C …` is run
- **THEN** the write is refused with exit 2

### Requirement: Readers never reject for a rel
`show`, `list` and `thread` SHALL display an episode whatever its rels. `validate` SHALL report an error (`link_rel_invalid`) for a rel that is neither in the snapshot nor an absolute URI.

#### Scenario: Unknown rel on read
- **WHEN** a hand-written file carries `rel: fulfils`
- **THEN** `show` prints it and `validate` reports `link_rel_invalid`

