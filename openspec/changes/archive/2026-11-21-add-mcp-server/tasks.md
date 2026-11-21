## 1. In-process options

- [x] 1.1 Add `cli.ExecuteWith(Options{Getenv}, …)`; `Execute` calls it with `os.Getenv` and no clock, unchanged in behaviour (D3)
- [x] 1.2 Thread `getenv` through `app` to `resolveNow`, `readerZone` and `zoneFor` (attribution and workspace are passed as flags, D3)
- [x] 1.3 The existing suite passes unchanged; a test runs a verb through `ExecuteWith` with an `Env` that hides `EPISODES_NOW` and checks the clock is used

## 2. Server skeleton

- [x] 2.1 Add `github.com/modelcontextprotocol/go-sdk` v1.7.0 and `internal/mcp` with `Options` (workspace root, version, author, harness, model, conversation tools) and `New`/`Run`
- [x] 2.2 `internal/cli/cmd_serve.go`: `serve --mcp` with `--author`, `--harness`, `--model`, `--conversation-tools`; usage error without `--mcp`; workspace found before the protocol starts; startup line and the `EPISODES_NOW is ignored` note on stderr
- [x] 2.3 The call runner: one mutex (D7), `ExecuteWith` with the bound `--workspace`, `--json`, `--author`, `--harness`, `--model` and the hiding `Env`; success → structured and text content; error envelope → tool error with code and message; `validate`'s exit 4 → result (D1)
- [x] 2.4 Bound harness falls back to the client's name from `initialize` (D4)

## 3. Tools

- [x] 3.1 The argument builder: `--name=value`, lists repeated, `links` as `--link=rel=href`, `fields` comma-joined, positionals after `--`, false/empty optionals omitted (D2)
- [x] 3.2 Input structs and mapping tables for the record group: `episode_record_inline`, `episode_record_ref`, `act_attest` (required `via`, `occurred`, `basis`; no `control`, no `standpoint`), `episode_observe`, `episode_redact`; `hash_file`/`content_file` resolved against the workspace root (D5, D6)
- [x] 3.3 Recall and workspace group: `episode_show`, `episode_list`, `episode_query`, `episode_thread`, `validate`, `workspace_status`
- [x] 3.4 `conversation_start` and `conversation_turn`, registered only with `--conversation-tools` (D9)
- [x] 3.5 Annotations: `readOnlyHint` on recall and workspace tools, `destructiveHint` on `episode_redact`, `destructiveHint: false` on the other writers; schemas reject unknown properties (D4)

## 4. Resources

- [x] 4.1 Templates `episode://{id}` (via `show`) and `episode://{id}/thread` (via `thread`), `application/json`, not-found for an unknown id (D8)
- [x] 4.2 Subscribe/unsubscribe handlers and `ResourceUpdated` after a successful observe or redact, for the episode and its container's thread

## 5. Tests

- [x] 5.1 Every mapping table's flags exist on the verb's flag set
- [x] 5.2 Through an in-memory client: tool list with and without `--conversation-tools`; annotations; a note recorded and its result equal to the CLI's `--json` shape; `duplicate` refusal; text beginning with `--`; links as objects; invalid `validate` as a result
- [x] 5.3 Binding: a call with `author` refused; client name as harness; `EPISODES_NOW` ignored; `model` per call
- [x] 5.4 `act_attest`: missing `via` refused; `via` from another standpoint refused with `act_via_standpoint`
- [x] 5.5 `episode_observe` with a relative `content_file`; two concurrent `conversation_turn` calls get consecutive ordinals
- [x] 5.6 Resources: read an episode and a thread; a subscriber notified after redact
- [x] 5.7 `serve` without `--mcp`, and outside a workspace

## 6. Docs

- [x] 6.1 README: `serve` row and an "MCP server" section (tools, binding, conversation tools, resources, a client config example)
- [x] 6.2 `SPEC-FEEDBACK.md`: item 19 (`tz` and v0.2.0) and items for the choices in D4, D6 and D1's `validate` result, with status rows
- [x] 6.3 CHANGELOG under Unreleased: Added `serve --mcp`
- [x] 6.4 `go test ./...`, `golangci-lint run ./...` and `openspec validate --specs` pass
