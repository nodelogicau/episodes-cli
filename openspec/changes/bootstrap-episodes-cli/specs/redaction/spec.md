## ADDED Requirements

### Requirement: Redact fields
`episodes redact <id> --fields <path>[,<path>…] --reason R` SHALL remove each listed path from the file and write a `redacted` block with `timestamp`, `reason`, `source` and `fields`, last in the file. All four SHALL be required.

#### Scenario: Withdraw a turn
- **WHEN** `redact epi_T --fields content.text --reason "No longer wanted"` is run
- **THEN** the file has no `content.text` and ends with `redacted: {timestamp, reason, source, fields: [content.text]}`

### Requirement: Closed set of paths
Only `label`, `participants`, `place`, `links`, `content.text`, `content.ref`, `content.uid`, `content.version`, `content.hash` and `content.language` SHALL be redactable. Any other path SHALL be refused on write and SHALL be an error in `validate` when listed in a file. `links` SHALL be redacted whole. A path the episode does not carry, and that no existing block lists, SHALL be refused.

#### Scenario: Index field refused
- **WHEN** `redact epi_T --fields standpoint` is run
- **THEN** the command exits 2 naming the closed set

#### Scenario: Absent field refused
- **WHEN** `redact epi_T --fields place` is run on an episode with no `place`
- **THEN** the command exits 2

### Requirement: Union and irreversibility
A second redaction SHALL make `fields` the union of the old and new paths and SHALL replace `timestamp`, `reason` and `source`. No verb SHALL remove a `redacted` block or restore a redacted field.

#### Scenario: Redact again
- **WHEN** an episode redacted for `label` is redacted for `content.text`
- **THEN** it carries one block with `fields: [content.text, label]` and the new reason

### Requirement: No cascade and no implicit redaction
Redacting an episode SHALL change no other file. No verb other than `redact` SHALL write a `redacted` block.

#### Scenario: Container untouched
- **WHEN** a turn is redacted
- **THEN** its container and sibling turns are byte-identical to before

### Requirement: Git history notice
Every successful `redact` SHALL state, in text output and in the JSON field `notice`, that the removed text remains in version-control history and in existing clones.

#### Scenario: Notice in JSON
- **WHEN** `redact … --json` succeeds
- **THEN** the result carries a non-empty `notice` that mentions git history

### Requirement: Redacted required fields are valid only by listing
`validate` SHALL accept an inline episode without `text`, or a reference episode without `ref`, only when `redacted.fields` lists that path.

#### Scenario: Tombstone
- **WHEN** a reference episode lacks `ref` and its block lists `content.ref`
- **THEN** `validate` reports no error for it
