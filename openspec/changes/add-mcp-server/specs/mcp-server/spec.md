## ADDED Requirements

### Requirement: Serve a workspace over MCP
`episodes serve --mcp [--workspace W] [--author A] [--harness H] [--model M] [--conversation-tools]` SHALL run a Model Context Protocol server over stdio, bound to one workspace resolved as every verb resolves it. `serve` without `--mcp` SHALL be a usage error. When no workspace is found, `serve` SHALL fail as a verb does, before speaking the protocol. Stdout SHALL carry only the protocol; diagnostics SHALL go to stderr. The server SHALL report the binary's version as its implementation version.

#### Scenario: Started without --mcp
- **WHEN** `episodes serve` is run
- **THEN** it exits with a usage error and writes nothing to stdout

#### Scenario: No workspace
- **WHEN** `episodes serve --mcp` is run outside any workspace with no `--workspace` or `EPISODES_WORKSPACE`
- **THEN** it exits with the error a verb gives for no workspace, before any protocol message

### Requirement: The reference tool set
The server SHALL list these tools, named as the format's Reference Tool Set names them: `episode_record_inline`, `episode_record_ref`, `act_attest`, `episode_observe`, `episode_redact`, `episode_show`, `episode_list`, `episode_query`, `episode_thread`, `validate` and `workspace_status`. With `--conversation-tools` it SHALL also list `conversation_start` and `conversation_turn`; without the flag, it SHALL NOT list them, and a call to either SHALL be refused as an unknown tool. `episode_show`, `episode_list`, `episode_query`, `episode_thread`, `validate` and `workspace_status` SHALL carry `readOnlyHint`. `episode_redact` SHALL carry `destructiveHint`. The other writing tools SHALL carry `destructiveHint: false`.

#### Scenario: Default tool list
- **WHEN** a client lists tools from `serve --mcp`
- **THEN** it sees the eleven tools, and neither conversation tool

#### Scenario: Conversation tools on request
- **WHEN** a client lists tools from `serve --mcp --conversation-tools`
- **THEN** it sees thirteen tools, including `conversation_start` and `conversation_turn`

### Requirement: A tool is its verb
Each tool SHALL perform the verb it is named after, as `--json` would: `episode_record_inline` is `record inline`, `episode_record_ref` is `record ref`, `conversation_start` and `conversation_turn` are `conversation start` and `conversation turn`, `act_attest` is `act`, `episode_observe` is `observe`, `episode_redact` is `redact`, `episode_show` is `show`, `episode_list` is `list`, `episode_query` is `query`, `episode_thread` is `thread`, `validate` is `validate`, and `workspace_status` is `workspace`. Parameters SHALL be the verb's flags and arguments in snake case, as the Reference Tool Set lists them. A repeatable flag SHALL be a list: `participants`, `links` as `{rel, href}` objects, `mementos` and `fields`. `text` SHALL be a value; there SHALL be no `text_file`. A parameter not in the tool's schema SHALL be refused. A successful call's result SHALL be the verb's `--json` output, as structured content and as its JSON text. A call the verb refuses or rejects SHALL be a tool error carrying the verb's error code and message. A value SHALL never be read as a flag, whatever it begins with. `validate` SHALL return its report as a result, not an error, whether or not the workspace is valid.

#### Scenario: Record an inline note
- **WHEN** `episode_record_inline` is called with `kind: note`, `standpoint: https://example.com/ada` and `text: "the invoice"`
- **THEN** a note is written, and the result is what `record inline … --json` would print, with `occurred.basis: recorded`

#### Scenario: A refusal is the verb's
- **WHEN** `episode_record_ref` is called with a `(standpoint, media_type, uid)` an existing reference episode has, and no `allow_duplicate`
- **THEN** the result is a tool error with code `duplicate`, naming the existing episode, and nothing is written

#### Scenario: Text that looks like a flag
- **WHEN** `episode_record_inline` is called with `text: "--kind=other"`
- **THEN** the episode's text is `--kind=other` and its kind is the one given

#### Scenario: Links as objects
- **WHEN** `episode_record_inline` is called with `links: [{rel: via, href: epi_T}]`
- **THEN** the episode carries that link, as `--link via=epi_T` would write it

#### Scenario: An invalid workspace still reports
- **WHEN** `validate` is called on a workspace with an error
- **THEN** the result is the report, with `valid: false`, and is not a tool error

### Requirement: Author, harness and clock are bound
No tool SHALL accept `author`, `harness` or `now`, and a call that passes one SHALL be refused as an unknown parameter. The server SHALL bind `source.author` from `--author`, then `EPISODES_AUTHOR`, then `defaults.source.author`, and `source.harness` from `--harness`, then `EPISODES_HARNESS`, then `defaults.source.harness`, then the connected client's name. Every writing tool SHALL accept `model`, used for that call. Without it, the model SHALL be bound from `--model`, then `EPISODES_MODEL`, then `defaults.source.model`. `source.timestamp`, the minting instant and any time defaulted to now SHALL come from the server's clock. `EPISODES_NOW` SHALL be ignored, and the server SHALL say so on stderr when it is set.

#### Scenario: An author in a call
- **WHEN** `episode_record_inline` is called with `author: https://example.com/eve`
- **THEN** the call is refused and nothing is written

#### Scenario: The client's name as harness
- **WHEN** no `--harness`, `EPISODES_HARNESS` or `defaults.source.harness` is set and a client named `claude-code` records a note
- **THEN** the note's `source.harness` is `claude-code`

#### Scenario: EPISODES_NOW ignored
- **WHEN** `EPISODES_NOW` is `2020-01-01T00:00:00Z` as the server starts, and a note is recorded
- **THEN** the note's `source.timestamp` and id come from the clock, and stderr said that `EPISODES_NOW` is ignored

### Requirement: Acts are attested through an episode
`act_attest` SHALL require `via`, `occurred` and `basis`, and SHALL have no `control` and no `standpoint` parameter. The act's standpoint SHALL be the bound author. `via` SHALL name an episode in the workspace when the call is made, whose standpoint is the bound author; any other SHALL be refused as `act` refuses it. When the server has no bound author, `act_attest` SHALL be refused.

#### Scenario: Missing via
- **WHEN** `act_attest` is called with `occurred` and `basis` but no `via`
- **THEN** the call is refused

#### Scenario: Via from another standpoint
- **WHEN** `act_attest` is called with `via` naming Claude's turn, and the bound author is Ada
- **THEN** the call is refused with `act`'s code for a `via` from another standpoint

### Requirement: Files the server reads
`hash_file` on `episode_record_ref` and `content_file` on `episode_observe` SHALL be paths the server reads. A relative path SHALL be resolved against the workspace root. The bytes SHALL be read exactly as the verb reads them.

#### Scenario: A relative content file
- **WHEN** `episode_observe` is called with `content_file: inbox/4471.eml` and that file is under the workspace root
- **THEN** the server hashes that file, as `observe --content-file <root>/inbox/4471.eml` would

### Requirement: Calls are applied one at a time
The server SHALL run one call at a time, so that ordinals, the index and the duplicate check see every earlier write.

#### Scenario: Two turns at once
- **WHEN** a client sends two `conversation_turn` calls for one container without waiting
- **THEN** they are given consecutive ordinals

### Requirement: Episodes as resources
The server SHALL offer resource templates `episode://{id}`, read as `show <id> --json`, and `episode://{id}/thread`, read as `thread <id> --json`, both as `application/json`. A client MAY subscribe to either. After a successful `episode_observe` or `episode_redact` of an episode, the server SHALL notify subscribers of `episode://{id}`, and of `episode://{container}/thread` when the episode has a container. The scheme SHALL never be written into an episode.

#### Scenario: Read an episode
- **WHEN** a client reads `episode://epi_E…`
- **THEN** it receives what `show epi_E… --json` prints

#### Scenario: Notified after redact
- **WHEN** a client subscribed to `episode://epi_C…/thread` and a turn of that conversation is redacted
- **THEN** the client is notified that `episode://epi_C…/thread` changed
