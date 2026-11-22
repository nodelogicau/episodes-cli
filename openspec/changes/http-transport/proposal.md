## Why

`serve --mcp` speaks only stdio, so a workspace can be reached only by a client that starts the binary itself on the same machine. The executive needs to reach memory from a hosted runtime: a Cloudflare Agent can act as an MCP client only over Streamable HTTP or SSE, and other hosts have the same limitation. A spike on 2026-11-22 (`artifacts-spike`) confirmed that a container can keep a workspace durable in a git remote, which leaves the transport as the missing piece.

## What Changes

- `serve --mcp --http <addr>` serves the same server over MCP Streamable HTTP at `/mcp`, bound to one workspace as today. Without `--http`, `serve --mcp` speaks stdio, unchanged.
- HTTP requests need a bearer token, read from `EPISODES_HTTP_TOKEN` or `--http-token-file`, never from a flag value. Without a token, the server refuses to listen on anything but a loopback address.
- `GET /healthz` reports that the workspace is loaded and the server is ready, without needing the token, for container and orchestrator readiness checks.
- Every HTTP session shares the one workspace, and calls are applied one at a time across all sessions. Each session's client name is the harness for its writes, unless one is bound by flag, environment or workspace default.
- A remote session is never the person: a session whose handshake gives no client name defaults `source.harness` to `http`.
- Over HTTP, `hash_file` and `content_file` must name a file inside the workspace root. An absolute path, a path that climbs out with `..`, or a symlink that leads outside is refused. Over stdio the caller is local and may already read those files, but over HTTP these parameters would let any token holder read any file the server can read.
- On SIGTERM or SIGINT the server stops accepting requests, lets calls in flight finish, and exits 0.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `mcp-server`: *Serve a workspace over MCP* becomes a transport-neutral requirement with stdio as the default. New requirements cover the HTTP transport, the token, the health endpoint, sessions sharing the one-call-at-a-time lock, the harness for nameless remote sessions, and shutdown. *Files the server reads* gains the rule confining paths to the workspace over HTTP.

## Impact

- `internal/cli/cmd_serve.go`: the `--http` and `--http-token-file` flags, the HTTP server, the health handler, and signal-driven shutdown.
- `internal/mcp`: the server already applies calls one at a time and reads the client name per session. HTTP sessions use it as it is, apart from the nameless-session harness default and the path check on `hash_file` and `content_file`.
- `github.com/modelcontextprotocol/go-sdk` v1.7.0 already provides `StreamableHTTPHandler`. No new dependency.
- README and `serve --help`. The MCP bundle and plugin are unaffected: they start the binary over stdio.
- The same change is proposed for `particulars-cli` and `intentions-cli`, so the three servers share flags, endpoints and token handling.
