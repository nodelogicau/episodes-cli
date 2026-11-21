## Why

The Episodes Format now names a [Reference Tool Set](https://github.com/nodelogicau/episodes#reference-tool-set) (upstream change `add-reference-tool-set`), and its Status section says no implementation exposes it yet. particulars-cli and intentions-cli each expose their format's set through `serve --mcp`. Until episodes-cli does the same, a harness can only record episodes by shelling out, and the set's rules for a tool call can't be checked against a binary (#32). Those rules are a bound author, harness and clock, `via` on every act, and a duplicate refused.

## What Changes

- New verb `episodes serve --mcp`, which speaks MCP over stdio using `github.com/modelcontextprotocol/go-sdk` (the version the sibling CLIs use).
  - It binds the workspace, `--author`, `--harness` and `--model` when it starts.
  - It exposes the reference set: `episode_record_inline`, `episode_record_ref`, `act_attest`, `episode_observe`, `episode_redact`, `episode_show`, `episode_list`, `episode_query`, `episode_thread`, `validate` and `workspace_status`.
  - With `--conversation-tools`, it also exposes `conversation_start` and `conversation_turn`. They are hidden by default, because the spec says turns come from the harness's transcript and not from a model re-typing them.
- **Each tool runs its verb.** The parameters are the verb's flags in snake case, with repeatable flags as lists and `links` as `{rel, href}`. The result is the verb's `--json` output, and a refusal is the verb's error code and message as a tool error.
- **The author, harness and clock are bound.** No tool takes `author`, `harness` or `now`, and a call that passes one is refused. `EPISODES_NOW` is ignored by the server. `source.timestamp`, minting and any default "now" use the real clock. Writing tools take `model`.
- **`act_attest` requires `via`, `occurred` and `basis`.** It has no `control` and no `standpoint`; the act's standpoint is the bound author.
- `hash_file` and `content_file` are paths the server reads, resolved against the workspace root when they are relative.
- **Resources:** `episode://{id}` and `episode://{id}/thread`. Subscribers are notified after `observe` and `redact`.
- **Tool annotations:** `readOnlyHint` on the recall and workspace tools, and `destructiveHint` on `episode_redact`.
- Every way this binary departs from, or makes concrete, what the set leaves open goes in `SPEC-FEEDBACK.md`. The first is that `episode_list`/`episode_query` take `tz`, which v0.2.0 doesn't have.
- **Not in this change:** a `.mcpb` bundle, a skill, and their release wiring. A follow-up change adds them.

## Capabilities

### New Capabilities

- `mcp-server`: `serve --mcp`, the tools and how each maps to its verb, binding, refusals, resources and notifications, and annotations.

### Modified Capabilities

None. The verbs' behaviour is unchanged. The server reuses them in-process.

## Impact

- New `internal/mcp` package (server, tool table, argument-to-flags mapping, resources) and `internal/cli/cmd_serve.go`.
- `internal/cli`: a way to run a verb in-process with the server's bindings applied. `Execute` gains an options form, and the CLI behaves exactly as before.
- New dependency `github.com/modelcontextprotocol/go-sdk`, and its own dependencies.
- README (`serve` row and an "MCP server" section), CHANGELOG under Unreleased, and `SPEC-FEEDBACK.md` items 19 onwards.
- The server ships in the next release (v0.3.0), since `tz` on `list`/`query` is unreleased.
