## Context

`serve --mcp` binds one workspace and runs `srv.Run(ctx, &sdk.StdioTransport{})`. The trust model is the local process: whoever can start the binary can already read and write the workspace's files, so the server needs no authentication.

Over HTTP that no longer holds. The intended deployment puts one server per person's workspace in a container, reached by the executive (for example, a Cloudflare Agent, whose MCP client refuses private addresses) and possibly by the app back end. In that deployment the endpoint is reachable from the internet, and the platform terminates TLS.

The same change is proposed in `particulars-cli` and `intentions-cli`. Flags, endpoints and token handling are identical across the three, so one deployment recipe serves all of them.

## Goals / Non-Goals

**Goals:**

- The tool surface, results, errors and instructions stay identical over stdio and HTTP.
- An HTTP endpoint is never open without a token unless it is bound to loopback.
- The server is safe for several concurrent sessions on one workspace.
- It behaves well in a container: a readiness probe, clean shutdown on SIGTERM, and logs on stderr.

**Non-Goals:**

- TLS. It is terminated by the platform or a proxy in front of the server.
- OAuth, or per-client and per-role scoped credentials. See the open questions.
- More than one workspace per process.
- Keeping the workspace durable, for example committing and pushing to a git remote after each write. That belongs to the deployment around the server, and can be revisited as an after-write hook.
- The legacy HTTP+SSE transport.

## Decisions

**Streamable HTTP via the SDK's `StreamableHTTPHandler`, at `/mcp`.** The go-sdk v1.7.0 handler takes a function that returns the server for a request. Returning the one bound server for every request gives every session the same workspace, lock and options. Alternative considered: one server per session. Rejected, because the one-call-at-a-time lock must be shared across sessions to keep the index consistent.

**`--http <addr>` selects the transport, and `--mcp` stays required.** `serve --mcp` alone is stdio, as today, so existing client configurations do not change. `addr` is `host:port`. A bare `:port` binds all interfaces, as in Go's `net.Listen`, which makes the token rule below matter. Alternative: `--transport stdio|http`. Rejected as more to type for no gain while there are two transports.

**A bearer token, compared in constant time, from the environment or a file.**
- `EPISODES_HTTP_TOKEN`, or `--http-token-file <path>` whose trimmed contents are the token.
- Never a flag value, which would show in process listings.
- A request to `/mcp` without `Authorization: Bearer <token>` gets 401 and never reaches the server.
- With no token configured, the server listens only on a loopback address (`127.0.0.1`, `::1` or `localhost`). Otherwise it refuses to start with a usage error.

Alternative: allow tokenless listening on any address with a warning. Rejected, because the failure is silent exposure of a person's memory.

**`GET /healthz` without a token.** It returns 200 with `{"status":"ok","version":...}` once the workspace is loaded and the listener is up. It reveals no workspace content or path. The container spike needed exactly this probe.

**A nameless remote session is given the harness `http`.** Over stdio a missing `clientInfo.name` leaves the harness unset, which is honest: the local user started the binary. Over HTTP the caller is remote. Defaulting the harness keeps a remote write from being recorded as the person's own unattributed act. An explicit `--harness`, `EPISODES_HARNESS` or `defaults.source.harness` still wins. Tools take no harness, as today.

**Over HTTP, files the server reads stay inside the workspace.** `episode_record_ref`'s `hash_file` and `episode_observe`'s `content_file` make the server read and hash a file. Over stdio that is harmless: the caller already runs as the same user. Over HTTP it would let any token holder learn the hash of, or probe for, any file the server can read. Over HTTP the path is therefore resolved against the workspace root, with symlinks evaluated, and refused as a usage error unless the result lies inside the root. Alternative: apply the rule to stdio too. Rejected for now, because it would break local callers who hash files elsewhere on their own machine. It is worth revisiting as a separate change.

**Shutdown uses `http.Server.Shutdown` with a 30-second deadline.** It is triggered by the existing `signal.NotifyContext`. In-flight tool calls finish, because each call holds the lock to completion. Cloudflare Containers allow up to 15 minutes between SIGTERM and SIGKILL, so 30 s is well inside that.

## Risks / Trade-offs

- [A single shared token gives full read and write access] → It is acceptable for one person's workspace behind one executive. Scoped tokens are an open question.
- [Long-lived sessions across container restarts lose their session id] → Clients must re-initialise. The SDK client and the Agents SDK `MCPClientManager` reconnect after restarts.
- [Binding `:port` with a token exposes the server on every interface] → That is intended for containers, and the token is required for it. The startup line on stderr names the address and says whether a token is in force.
- [The one-call-at-a-time lock serialises writers across sessions] → It is already the case for one stdio client issuing parallel calls. Throughput is bounded by disk, not by the lock, for one person's workspace.

## Migration Plan

None needed. Stdio behaviour and every existing flag are unchanged. Rollback is removing the flag.

## Open Questions

- A read-only token that offers only tools annotated `readOnlyHint`. That would give the app back end's reads, and the executive's role-scoped access (its ADR 0007), a credential that cannot write. Add it now, or wait for a consumer?
- Should the token file be re-read on SIGHUP, so it can rotate without a restart, or is a restart acceptable?
- Is `http` the right default harness name, or should it name the transport and the remote address?
