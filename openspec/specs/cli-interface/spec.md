# cli-interface Specification

## Purpose
The global CLI contract: non-interactive operation, `--json`, exit codes, workspace selection, attribution flags and `EPISODES_*` environment variables, `--now`, and `version`. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: Non-interactive operation
The CLI SHALL never prompt for input or read from a terminal. Text SHALL be accepted via `--text` or `--text-file <path>`, where `-` means piped stdin. A command lacking required input SHALL fail with a usage error rather than prompting.

#### Scenario: Turn text from stdin
- **WHEN** `episodes conversation turn epi_C --standpoint https://example.com/ada --text-file -` is run with text piped to stdin
- **THEN** the piped bytes become the turn's `content.text` and the command completes without prompting

#### Scenario: Missing standpoint
- **WHEN** `episodes record inline --kind note --text hi` is run without `--standpoint`
- **THEN** the command exits 2 and names `standpoint` on stderr

### Requirement: JSON output mode
Every verb SHALL accept `--json`. On success it SHALL write exactly one JSON object to stdout. On failure it SHALL write `{"error": {"code": <string>, "message": <string>}}` to stderr and nothing to stdout.

#### Scenario: Successful JSON output
- **WHEN** any verb is run with `--json` and succeeds
- **THEN** stdout holds one parseable JSON object and stderr is empty

#### Scenario: Failure in JSON mode
- **WHEN** any verb is run with `--json` and fails
- **THEN** stdout is empty and stderr holds one JSON object with `error.code` and `error.message`

### Requirement: Exit codes
The CLI SHALL use these exit codes: `0` success; `1` runtime error; `2` usage error, including every write a format rule refuses and every `duplicate` refusal; `3` not found; `4` check failed (`validate` with errors, `index` or `index --check` with drift or a late merge); `5` no workspace found.

#### Scenario: Unknown id
- **WHEN** `episodes show epi_does-not-exist` is run in a workspace
- **THEN** the command exits 3

#### Scenario: Refused write leaves nothing behind
- **WHEN** a write is refused by a format rule
- **THEN** the command exits 2, names the rule, and no file in the workspace is created or modified

#### Scenario: No workspace
- **WHEN** a workspace verb is run outside any workspace with no `--workspace` and no `EPISODES_WORKSPACE`
- **THEN** the command exits 5

### Requirement: Workspace selection
Every workspace verb SHALL accept `--workspace <dir>`. Precedence SHALL be `--workspace`, then `EPISODES_WORKSPACE`, then discovery as defined by the `workspace` capability.

#### Scenario: Flag wins over environment
- **WHEN** both `--workspace A` and `EPISODES_WORKSPACE=B` are set
- **THEN** workspace A is used

### Requirement: Source attribution on every writing verb
Every verb that writes SHALL accept `--author`, `--harness` and `--model`, falling back to `EPISODES_AUTHOR`, `EPISODES_HARNESS` and `EPISODES_MODEL`, then to `defaults.source` in `episodes.yaml`. The resolved source SHALL contain at least one of `author` or `harness`, SHALL be written to the `source` block of what the verb writes, and SHALL appear in the JSON result.

#### Scenario: Harness on the default author's behalf
- **WHEN** `episodes record inline … --harness claude --model claude-opus-5-5` is run in a workspace whose `defaults.source.author` is `https://example.com/ada`
- **THEN** the file carries `source: {author: https://example.com/ada, harness: claude, model: claude-opus-5-5}`

#### Scenario: No source at all
- **WHEN** a writing verb resolves neither an author nor a harness
- **THEN** the write is refused with exit 2 naming `source`

### Requirement: Deterministic time for tests
Writing verbs SHALL accept `--now <RFC 3339>` and honour `EPISODES_NOW`. These SHALL set the recording `timestamp` and any `occurred` the verb defaults to now. Without either, the process clock SHALL be used. The flag SHALL be documented as test-facing.

#### Scenario: Timestamp from --now
- **WHEN** a turn is recorded with `--now 2026-11-07T03:24:40Z`
- **THEN** the file carries `timestamp: 2026-11-07T03:24:40Z`

### Requirement: No git operations
No verb SHALL invoke git or modify `.git`.

#### Scenario: Recording in a git workspace
- **WHEN** an episode is recorded in a workspace that is a git repository
- **THEN** the episode file and index are written and no commit, stage or other git operation is performed

### Requirement: Version verb
`episodes version` SHALL print the binary version, the format it implements (`episodes/0.1`), and the date of its embedded link-relation snapshot. In JSON mode these SHALL be the fields `version`, `format` and `link_relations`.

#### Scenario: Version in JSON
- **WHEN** `episodes version --json` is run
- **THEN** stdout is an object with `version`, `format: "episodes/0.1"` and `link_relations`

