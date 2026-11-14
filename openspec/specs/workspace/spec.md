# workspace Specification

## Purpose
`init`, the `episodes.yaml` marker and configuration, refusing an unimplemented `format`, discovery, and the `.episodes` pointer. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: Initialise a workspace
`episodes init [dir]` SHALL create `episodes.yaml` with `format: episodes/0.1` and `workspace.id` (a bare UUIDv7), the `episodes/` directory, and an empty `index.yaml`. `--author`, `--harness` and `--zone` SHALL be written to `defaults`. `init` SHALL refuse a directory that already holds `episodes.yaml`.

#### Scenario: Fresh init
- **WHEN** `episodes init ./memory --author https://example.com/ada --zone Australia/Melbourne` is run
- **THEN** `./memory/episodes.yaml` holds `format`, a `workspace.id`, `defaults.source.author` and `defaults.zone`, and `./memory/index.yaml` holds `format: episodes/0.1` and no entries

#### Scenario: Init over an existing workspace
- **WHEN** `episodes init` is run where `episodes.yaml` exists
- **THEN** the command exits 2 and nothing is changed

### Requirement: Refuse an unimplemented format
Every workspace verb SHALL refuse a workspace whose `format` is not `episodes/0.1`, naming the version, and SHALL NOT read anything else from it. Unknown keys in `episodes.yaml` SHALL be ignored.

#### Scenario: Newer format
- **WHEN** any verb is run in a workspace with `format: episodes/0.2`
- **THEN** the command exits 1 naming `episodes/0.2`

### Requirement: Discovery
Without `--workspace` or `EPISODES_WORKSPACE`, the workspace SHALL be the nearest ancestor of the current directory that holds either `episodes.yaml` or a `.episodes` pointer file naming a workspace directory. `EPISODES_WORKSPACE` SHALL be strict: if it does not name a workspace, the verb SHALL exit 5 rather than fall back to discovery.

#### Scenario: Pointer in a project repository
- **WHEN** a verb is run in `~/code/app/src` and `~/code/app/.episodes` names `~/memory`
- **THEN** the workspace is `~/memory`

#### Scenario: Strict environment variable
- **WHEN** `EPISODES_WORKSPACE` names a directory with no `episodes.yaml`
- **THEN** the command exits 5

### Requirement: Workspace inspection and pointer
`episodes workspace` SHALL print the resolved root, how it was found (`flag`, `env`, `marker`, `pointer`), and its configuration. `episodes workspace pointer [dir] [--at D] [--force]` SHALL write a `.episodes` pointer in `D` (default: the current directory) naming the workspace, and SHALL refuse to overwrite an existing pointer without `--force`.

#### Scenario: Show how the workspace was found
- **WHEN** `episodes workspace --json` is run under a pointer
- **THEN** the result carries `root` and `found_by: "pointer"`

