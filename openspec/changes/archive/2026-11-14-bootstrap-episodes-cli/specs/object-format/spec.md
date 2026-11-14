## ADDED Requirements

### Requirement: Identifiers
Writers SHALL mint ids as `epi_` followed by a lowercase canonical UUIDv7, with a monotonic counter, so that ids minted in the same millisecond sort in creation order. Readers SHALL accept any id matching `^epi_[A-Za-z0-9-]+$`.

#### Scenario: Burst minting
- **WHEN** 1,000 episodes are recorded in a tight loop
- **THEN** sorting their ids gives their creation order

#### Scenario: Legacy id is readable
- **WHEN** a workspace contains `episodes/legacy/epi_import-42.yaml`
- **THEN** `show epi_import-42` prints it

### Requirement: Canonical field order and serialisation
Writers SHALL emit fields in the order `id, type, kind, occurred, standpoint, participants, place, label, container, ordinal, links, content, source, timestamp, source_status, redacted`, with fields this format does not define after the specified fields and before `source_status` and `redacted`, and `redacted` last. Output SHALL use 2-space indentation, literal block scalars for multi-line `text`, sorted and de-duplicated `participants`, `links` sorted by `rel` then `href`, recording instants as UTC `Z` at seconds precision, absent optional fields omitted, and no document markers. Two writes of the same episode SHALL be byte-identical.

#### Scenario: The spec's example round-trips
- **WHEN** the README's `message` example is read and re-encoded
- **THEN** the output is byte-identical to the input

#### Scenario: Reordered input
- **WHEN** a file with its fields in reverse order is read and re-encoded by `observe`
- **THEN** the output is in canonical order

### Requirement: Order-tolerant, unknown-tolerant reading
Readers SHALL accept fields in any order and SHALL NOT reject a file for a field this format does not define. Such a field SHALL be given no meaning and SHALL be preserved byte-for-byte by every rewrite. Duplicate keys and non-mapping documents SHALL be invalid.

#### Scenario: Leftover occasion field
- **WHEN** an episode carrying `occasion: int_X` is validated and then observed
- **THEN** validation does not reject it, and `occasion: int_X` is still present after the rewrite

### Requirement: Required fields and kind
An episode SHALL carry `id`, `type: episode`, `kind`, `occurred`, `standpoint`, `content`, `source` and `timestamp`. `kind` SHALL match `^[a-z][a-z0-9-]*$`, and any such word SHALL be accepted. `kind` SHALL NOT change which fields are valid.

#### Scenario: Unknown kind
- **WHEN** an episode with `kind: voicememo` is validated
- **THEN** no finding is reported for its kind

#### Scenario: Malformed kind refused on write
- **WHEN** `record inline --kind "Phone Call"` is run
- **THEN** the write is refused with exit 2

### Requirement: References
`standpoint`, each of `participants`, `place` and `source.author` SHALL be non-empty strings. A value that is not a URI, or that nothing resolves, SHALL be accepted.

#### Scenario: Non-URI participant
- **WHEN** an episode is recorded with `--participant "Jane at accounts"`
- **THEN** the write succeeds and the participant is written as given

### Requirement: Source and timestamp
`source` SHALL contain at least one of `author` or `harness`, and optionally `model`. `timestamp` SHALL be the recording instant in UTC. A writer SHALL keep `source` (who recorded it) distinct from `standpoint` (whose it is) and SHALL NOT default one from the other, except as `self-attested-acts` requires.

#### Scenario: Recording someone else's email
- **WHEN** a harness acting for Ada records an email Jane sent with `--standpoint https://example.com/jane`
- **THEN** the file carries `standpoint: https://example.com/jane` and `source.author: https://example.com/ada`

### Requirement: Label is captured, not composed
`label` SHALL be accepted only via `--label` and SHALL be written as given. The binary SHALL NOT generate a label.

#### Scenario: No label supplied
- **WHEN** a turn is recorded without `--label`
- **THEN** the file carries no `label`
