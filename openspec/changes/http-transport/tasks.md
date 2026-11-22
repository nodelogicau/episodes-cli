## 1. Serve over HTTP

- [ ] 1.1 Add `--http <addr>` and `--http-token-file <path>` to `serve`. `--http-token-file` without `--http` is a usage error.
- [ ] 1.2 Resolve the token: file contents trimmed, else `EPISODES_HTTP_TOKEN`; empty counts as none. Refuse a tokenless non-loopback address with a usage error before listening.
- [ ] 1.3 Mount `sdk.NewStreamableHTTPHandler` at `/mcp`, returning the one bound server for every request, behind a bearer middleware using `crypto/subtle.ConstantTimeCompare`.
- [ ] 1.4 Add `GET /healthz` returning `{"status":"ok","version":...}` without a token; 404 for every other path.
- [ ] 1.5 Write the startup line to stderr: version, workspace root, address, whether a token is required.
- [ ] 1.6 On the existing signal context, call `http.Server.Shutdown` with a 30 s deadline and exit 0.

## 2. Provenance

- [ ] 2.1 Default `source.harness` to `http` for an HTTP session with no `clientInfo.name`, after `--harness`, `EPISODES_HARNESS` and `defaults.source.harness`. Leave stdio unchanged.

- [ ] 2.2 Over HTTP, resolve `hash_file` and `content_file` against the root with `filepath.EvalSymlinks`, and refuse a usage error unless the result is inside the root. Leave stdio unchanged.

## 3. Tests

- [ ] 3.1 Over an `httptest` server, run one session through `initialize`, `tools/list` and `episode_list`, and compare with the stdio result for the same workspace.
- [ ] 3.2 Token: missing gives 401, wrong gives 401, correct is served. The file wins over the environment.
- [ ] 3.3 A tokenless `:port` refuses to start; a tokenless `127.0.0.1:port` serves.
- [ ] 3.4 `/healthz` gives 200 without a token, and its body has only `status` and `version`; `/` gives 404.
- [ ] 3.5 Two sessions with ten concurrent `episode_record_inline` calls each: twenty episodes, `episodes validate` passes.
- [ ] 3.6 Per-session harness from each handshake; a nameless session gets `http`.
- [ ] 3.7 Over HTTP: an absolute `content_file`, a `../` path and a symlink that leads outside are each refused, and nothing is written. A path inside the root is hashed as over stdio.
- [ ] 3.8 SIGTERM during a write: the write completes and the process exits 0.

## 4. Wrap-up

- [ ] 4.1 `serve --help` and the README: the HTTP section, the token rule, `/healthz`, and an example for a container.
- [ ] 4.2 A CHANGELOG entry under Unreleased.
- [ ] 4.3 Archive the change and sync `mcp-server`.
