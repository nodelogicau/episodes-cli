# redaction Specification

## Purpose
The `redact` verb, the closed field set, union semantics, and the git-history notice. Introduced by change `bootstrap-episodes-cli`.
## Requirements
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

### Requirement: Redacting content.ref is complete
A `redacted` block that lists `content.ref` SHALL also list each of `content.uid`, `content.version`, `content.hash` and `links` that the episode carried, and each SHALL be absent from the file. When `content.ref` is in a redaction's union, whether named now or by an earlier block, `redact` SHALL add whichever of the four the episode still carries, write them in `fields`, and name them in its result (`added` in JSON). Each of the four SHALL stay redactable on its own, without `content.ref`. `validate` SHALL report an error (`redaction_incomplete`) for each of the four still present in a file whose block lists `content.ref`, naming the field.

#### Scenario: The tombstone no longer says where
- **WHEN** a reference episode with `uid`, `version`, `hash` and `links` is redacted with `--fields content.ref`
- **THEN** all five are removed and listed, and the result's `added` names the four

#### Scenario: Nothing else carried
- **WHEN** a reference episode carrying none of the four is redacted with `--fields content.ref`
- **THEN** only `content.ref` is listed, and the result has no `added`

#### Scenario: Repairing an earlier redaction
- **WHEN** a file whose block lists `content.ref` still carries `content.uid` and `content.hash`
- **THEN** `validate` reports `redaction_incomplete` for each, and `redact --fields content.uid` removes both and leaves it valid

### Requirement: Derived values are deleted with the field
A value the binary derives from a field and keeps, such as the uid cache, SHALL be deleted or recomputed when that field is redacted, and SHALL NOT be used in the meantime. Whether it holds SHALL be decided from the episode file, not from the cache. `redact` SHALL delete the cached `uid` of an episode whose `content.uid` it removes. A lookup that finds a cached `uid` the file no longer carries, as after a redaction merged in from another clone, SHALL delete it.

#### Scenario: Redacted uid leaves the cache
- **WHEN** an episode whose `uid` is cached is redacted with `--fields content.ref`
- **THEN** the cache file no longer contains that `uid`

#### Scenario: Redaction merged in
- **WHEN** a file whose `uid` is cached arrives with `content.uid` redacted, and the same `uid` is then recorded
- **THEN** the write succeeds, and the cache no longer holds the redacted episode's `uid`

