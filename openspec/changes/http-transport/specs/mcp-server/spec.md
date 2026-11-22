## MODIFIED Requirements

### Requirement: Serve a workspace over MCP
`episodes serve --mcp [--workspace W] [--author A] [--harness H] [--model M] [--conversation-tools] [--http ADDR] [--http-token-file F]` SHALL run a Model Context Protocol server bound to one workspace, resolved as every verb resolves it. Without `--http` the server SHALL speak stdio, and stdout SHALL carry only the protocol. With `--http` it SHALL speak Streamable HTTP as *HTTP transport* requires, and SHALL write nothing to stdout. Diagnostics SHALL go to stderr. `serve` without `--mcp` SHALL be a usage error, and so SHALL `--http-token-file` without `--http`. When no workspace is found, `serve` SHALL fail as a verb does, before speaking either transport. The server SHALL report the binary's version as its implementation version.

#### Scenario: Started without --mcp
- **WHEN** `episodes serve` is run
- **THEN** it exits with a usage error and writes nothing to stdout

#### Scenario: No workspace
- **WHEN** `episodes serve --mcp` is run outside any workspace with no `--workspace` or `EPISODES_WORKSPACE`
- **THEN** it exits with the error a verb gives for no workspace, before any protocol message

#### Scenario: No workspace over HTTP
- **WHEN** `episodes serve --mcp --http 127.0.0.1:8080` is run outside any workspace with no `--workspace` or `EPISODES_WORKSPACE`
- **THEN** it exits with the error a verb gives for no workspace, without listening

### Requirement: Files the server reads
`hash_file` on `episode_record_ref` and `content_file` on `episode_observe` SHALL be paths the server reads. A relative path SHALL be resolved against the workspace root. The bytes SHALL be read exactly as the verb reads them. Over HTTP, the path SHALL be resolved against the workspace root with symlinks evaluated. A path whose result does not lie inside the root SHALL be refused as a usage error, before anything is read or written. That includes an absolute path outside the root, a path that climbs out with `..`, and a symlink that leads outside.

#### Scenario: A relative content file
- **WHEN** `episode_observe` is called with `content_file: inbox/4471.eml` and that file is under the workspace root
- **THEN** the server hashes that file, as `observe --content-file <root>/inbox/4471.eml` would

#### Scenario: An outside path over HTTP
- **WHEN** an HTTP session calls `episode_record_ref` with `hash_file: /etc/passwd`, or with `hash_file: ../secrets.txt`
- **THEN** the call is refused as a usage error, and nothing is read or written

#### Scenario: A symlink out of the workspace over HTTP
- **WHEN** an HTTP session calls `episode_observe` with `content_file: inbox/link`, and `inbox/link` is a symlink to a file outside the root
- **THEN** the call is refused as a usage error, and nothing is written

## ADDED Requirements

### Requirement: HTTP transport
With `--http ADDR`, where `ADDR` is `host:port` and a missing host means every interface, the server SHALL serve MCP Streamable HTTP at the path `/mcp`. Every session SHALL be served by the one server bound to the workspace. Tools, results, errors, instructions and resources SHALL be identical to those over stdio, apart from the path rule in *Files the server reads*, and `--conversation-tools` SHALL apply as it does over stdio. Other paths SHALL return 404, except `/healthz`. On startup the server SHALL write to stderr the version, the workspace root, the address it listens on, and whether a token is required.

#### Scenario: The same result over both transports
- **WHEN** one client calls `episode_list` over stdio and another calls it over HTTP, on the same workspace
- **THEN** both results equal `episodes list --json`

#### Scenario: Unknown path
- **WHEN** a request is sent to `/` on a server started with `--http`
- **THEN** the response is 404

### Requirement: HTTP requires a token off loopback
The token SHALL be read from `EPISODES_HTTP_TOKEN`, or from the file named by `--http-token-file`, whose contents with surrounding whitespace trimmed are the token. The file SHALL win when both are set. An empty token SHALL count as none. When a token is configured, every request to `/mcp` SHALL carry `Authorization: Bearer <token>`. The comparison SHALL be in constant time. A request without the token, or with a different one, SHALL get 401 and SHALL not reach the server. When no token is configured, the server SHALL listen only if `ADDR`'s host is `127.0.0.1`, `::1` or `localhost`. Otherwise it SHALL exit with a usage error before listening.

#### Scenario: Missing token
- **WHEN** a server started with `EPISODES_HTTP_TOKEN=s3cret --http :8080` receives an `initialize` request with no `Authorization` header
- **THEN** the response is 401 and no session is created

#### Scenario: Correct token
- **WHEN** the same server receives `initialize` with `Authorization: Bearer s3cret`
- **THEN** a session is created and the tools are listed as over stdio

#### Scenario: Tokenless on all interfaces
- **WHEN** `episodes serve --mcp --http :8080` is started with no token configured
- **THEN** it exits with a usage error saying a token is required off loopback, and does not listen

#### Scenario: Tokenless on loopback
- **WHEN** `episodes serve --mcp --http 127.0.0.1:8080` is started with no token configured
- **THEN** it listens, and serves requests without an `Authorization` header

### Requirement: Health endpoint
`GET /healthz` SHALL return 200 with a JSON body `{"status": "ok", "version": <version>}` once the workspace is loaded and the listener is up. It SHALL not require the token, and SHALL reveal no workspace content, path or configuration.

#### Scenario: Readiness probe
- **WHEN** a container platform sends `GET /healthz` without a token to a server started with a token
- **THEN** the response is 200 with `status: ok` and the binary's version, and nothing else

### Requirement: HTTP sessions share one workspace, one call at a time
Concurrent HTTP sessions SHALL be served one call at a time across all sessions, as *Calls are applied one at a time* requires, so that ordinals, the index and the duplicate check see every earlier write from any session. Each session SHALL take the client name for `source.harness` from its own `initialize`, as *Author, harness and clock are bound* requires.

#### Scenario: Parallel records from two sessions
- **WHEN** two HTTP sessions each issue ten `episode_record_inline` calls concurrently
- **THEN** twenty episodes exist and `episodes validate` passes

#### Scenario: Turns from two sessions
- **WHEN** two HTTP sessions each send a `conversation_turn` for the same container without waiting, on a server started with `--conversation-tools`
- **THEN** the turns are given consecutive ordinals

#### Scenario: Each session's own harness
- **WHEN** a session whose client is named `executive` and a session whose client is named `connector` each record a note, with no harness bound by flag, environment or workspace default
- **THEN** the notes carry `source.harness: executive` and `source.harness: connector` respectively

### Requirement: A remote session is never the person
Over HTTP, when no harness is bound by `--harness`, `EPISODES_HARNESS` or `defaults.source.harness`, and the session's `initialize` carries no `clientInfo.name`, `source.harness` SHALL be bound to `http`. Over stdio the existing binding SHALL be unchanged.

#### Scenario: Nameless remote client
- **WHEN** an HTTP session whose `initialize` has an empty `clientInfo.name` records a note, with no harness otherwise bound
- **THEN** the note's `source.harness` is `http`

### Requirement: Clean shutdown over HTTP
On SIGTERM or SIGINT, a server speaking HTTP SHALL stop accepting new connections. It SHALL let requests in flight finish for up to 30 seconds, then exit 0. A writing call that has started SHALL either complete, or leave the workspace as it was before the call.

#### Scenario: Terminated during a write
- **WHEN** SIGTERM arrives while an `episode_record_inline` call is in flight
- **THEN** the call completes, its episode file and index entry are written, and the process exits 0
