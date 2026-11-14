# accumulation Specification

## Purpose
The append-only store: the single mutation choke point, atomic writes, and the workspace lock. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: No revision
No verb SHALL edit, rewrite or delete an episode file, except as `source-status` and `redaction` define. There SHALL be no `edit`, `delete` or `retire` verb.

#### Scenario: Better transcript is a new episode
- **WHEN** a corrected transcript of `epi_A` is recorded with `--link predecessor-version=epi_A`
- **THEN** a new file is written and `epi_A` is byte-identical to before

### Requirement: Single mutation path
Every rewrite of an existing episode file SHALL pass through one store function. That function SHALL refuse the write if any field differs from the file as read, other than `source_status`, `redacted`, and the fields `redacted.fields` removes.

#### Scenario: Guard catches a stray change
- **WHEN** an amendment function also changes `label` while setting `source_status`
- **THEN** the store refuses the write and the file is unchanged

### Requirement: Create without overwrite
Creating an episode SHALL write to its derived path with exclusive creation, and SHALL fail rather than overwrite an existing file.

#### Scenario: Path collision
- **WHEN** a file already exists at the path a new id derives
- **THEN** the write fails with exit 1 and the existing file is unchanged

### Requirement: Atomic writes
Every file write SHALL be atomic: written to a temporary file in the same directory and renamed into place. A failed write SHALL leave no partial file.

#### Scenario: Interrupted write
- **WHEN** a write is interrupted before the rename
- **THEN** no file exists at the target path, and no temporary file remains after the next write

### Requirement: Workspace write lock
Every writing verb SHALL hold an exclusive advisory lock on `episodes.yaml` from minting until its index upsert completes. Readers SHALL NOT take the lock. The lock SHALL be released when the process exits.

#### Scenario: Killed writer
- **WHEN** a writing process is killed while holding the lock
- **THEN** the next writer acquires the lock without manual cleanup

