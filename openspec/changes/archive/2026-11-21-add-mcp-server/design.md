## Context

The Episodes Format's Reference Tool Set (upstream, merged 2026-11-21) names thirteen tools, one per verb. It requires that "a tool's result is the same as the implementation's non-interactive output for the operation, and every refusal the operation makes, the tool makes". It also binds the author, harness and clock.

The verbs' logic lives in cobra `RunE` closures in `internal/cli`. `cli.Execute(args, stdin, stdout, stderr, isTerminal)` already runs the whole CLI in-process without calling `os.Exit`, and the tests drive it that way. With `--json`:
- success writes one JSON object to stdout;
- an error writes `{"error": {"code", "message"}}` to stderr and returns an exit code.

The verbs read `EPISODES_NOW` (`helpers.go`, `resolveNow`), `EPISODES_AUTHOR`, `EPISODES_HARNESS` and `EPISODES_MODEL` (`prov.Resolve`), `EPISODES_WORKSPACE` (`store.Discover`) and `EPISODES_ZONE` from the process environment.

intentions-cli and particulars-cli each have an `internal/mcp` package on `github.com/modelcontextprotocol/go-sdk` v1.7.0 that reimplements every tool against their store. Neither offers resources.

## Goals / Non-Goals

**Goals:**
- Tools whose output and refusals are the verbs', by construction, not by keeping two implementations in step.
- The author, harness and clock bound as the format requires, with no way around it through a parameter or the environment.
- A small server: a table of tools, a mapping from arguments to flags, and resources.

**Non-Goals:**
- A `.mcpb` bundle, a skill or an MCP prompt. These go in a follow-up change.
- Any transport other than stdio.
- `import`, `index` and `init` as tools. They aren't in the set.
- Moving the verbs out of cobra.

## Decisions

**D1. A tool runs its verb through `cli.Execute`.** Each tool builds the verb's arguments from its input, adds `--workspace <bound root>`, `--json` and the bound attribution, and calls `Execute` with an empty stdin and buffers for stdout and stderr. The result is then:
- exit 0: the parsed stdout object;
- a non-zero exit with an `error` envelope: a tool error carrying that code and message;
- `validate` exiting 4 (`check_failed`, report on stdout, no envelope): the report as an ordinary result.

The alternative was moving each verb's body into a package that both the CLI and the server call. That's cleaner in the long run, but it means a large refactor of `cmd_record.go`, `cmd_mutate.go` and `cmd_recall.go` before the first tool works, and two call paths to keep identical. Running the verb itself makes "the same output, every refusal" true without a test per refusal. The cost is the work of building arguments, and building the arguments is pure, so it can be tested without a server.

**D2. Values are passed so they can't become flags.**
- Every flag is passed as `--name=value`, so a value beginning with `-` is never parsed as a flag.
- Positional arguments (`id`, `container`, the query text) come after `--`.
- A list becomes one `--name=value` per item.
- `links` become `--link=rel=href`.
- `fields` on redact becomes one comma-joined `--fields=` value, as the verb takes it.
- A false or empty optional is omitted.

The mapping is one table per tool: parameter, flag, and positional or not. A test runs each table against the verb's real flag set, so a renamed flag fails a test.

**D3. The binding goes through an options form of `Execute`, not the environment.** `cli.ExecuteWith(Options{Getenv}, …)` carries `Getenv`, the lookup the verbs use in place of `os.Getenv` for what they read directly: `EPISODES_NOW` in `resolveNow`, and `EPISODES_ZONE` in `readerZone` and `zoneFor`. The server passes a `Getenv` that hides `EPISODES_NOW`. No tool passes `--now`, so the verbs then use the real clock and mint from it, and no separate clock option is needed. `Execute` calls `ExecuteWith` with `os.Getenv`, unchanged.

Attribution and the workspace aren't routed through `Getenv`. The server passes `--author`, `--harness`, `--model` and `--workspace` explicitly, and these come before the environment in `prov.Resolve` and `store.Discover`. When a bound value is empty, the verb's own fallback reads the same process environment the server resolved from, so the result is the same.

The alternative was `os.Unsetenv("EPISODES_NOW")` when `serve` starts. That's simpler, but it changes the process for anything else in it, including tests, and it hides the binding in a side effect.

**D4. Attribution is resolved once, at startup, except the client's name and `model`.** `serve` resolves author and harness with `prov.Resolve` from its flags, the environment and `defaults.source`. If no harness is found, the client's name from `initialize` is used, read per session. Each writing call passes `--author=…` and `--harness=…` explicitly, so the verb's own resolution order can't pick anything else. `model` comes from the call when given, else from the bound model.

Tool input structs have no `author`, `harness` or `now` field. go-sdk validates input against the inferred schema, and the schema is set to `additionalProperties: false` (explicitly, if inference doesn't already do it), so a call passing any of them is refused before it reaches the verb.

**D5. `act_attest` maps to `act --via=…`.** The tool's input has `via`, `occurred` and `basis` as required fields, and has no `control` or `standpoint`. The verb already refuses:
- a `via` that doesn't exist or isn't earlier;
- a `via` whose standpoint isn't the author's (`act_via_standpoint`);
- an act with no author.

No new checks are needed.

**D6. Paths are made absolute against the workspace root** before they're passed as `--hash-file` and `--content-file`. A server's working directory is whatever the client launched it in, which a model can't see. The workspace root is something the model can know from `workspace_status`. An absolute path is passed unchanged.

**D7. One mutex for every call.** Writes must be serialised, because ordinals, the index and the duplicate check assume one writer. Reads are serialised too. That's simpler, and a read sees no half-finished write; a 20k-episode `query` is about 1 s. The alternative was a read/write lock, which wouldn't be worth it at this size.

**D8. Resources are read through `show` and `thread`.**
- `episode://{id}` runs `show <id> --json`.
- `episode://{id}/thread` runs `thread <id> --json`.
- The server sets `SubscribeHandler` and `UnsubscribeHandler` and keeps a set of subscribed URIs.
- After a successful `episode_observe` or `episode_redact`, it calls `ResourceUpdated` for `episode://{id}`, and for `episode://{container}/thread` when the episode's JSON has a `container`.
- A read of an unknown id is the resource-not-found error.

**D9. Conversation tools are registered only with `--conversation-tools`.** An unregistered tool is unknown to the client, which matches the spec's "MAY leave out of the tools it lists".

**D10. SPEC-FEEDBACK items.** Item 19: the set is said to be drawn from v0.2.0, but `episode_list` and `episode_query` take `tz`, which first exists in v0.3.0, and the set doesn't say what `tz` means there. More items cover:
- which "author passed" behaviour was chosen (refused);
- relative paths resolved against the workspace root;
- `validate`'s invalid report being a result, not an error;
- anything else found while implementing.

## Risks / Trade-offs

- **[Building arguments drifts from the verbs]** A flag renamed or added in a verb isn't picked up by its tool. → The D2 test checks every mapped flag exists on the verb. A test per tool checks its result equals the CLI's `--json` for the same input.
- **[The JSON is parsed twice]** The verb encodes JSON, and the server decodes it to return structured content. → Small cost. The text content is the verb's bytes unchanged, so nothing is lost.
- **[Threading `Env` touches many call sites]** → It's mechanical. `Execute`'s behaviour is pinned by the existing suite, which must pass unchanged.
- **[Serialising reads]** A slow query blocks a write. → Acceptable at the sizes measured (`docs/benchmarks.md`). It can be revisited with a read/write lock.
- **[Resources are new in the family]** Neither sibling has them, so there's no shared pattern. → Keep it to two templates, with no listing of every episode.

## Migration Plan

This adds a verb and doesn't change any existing one, so there's nothing to migrate. It ships in v0.3.0. After the release, upstream's Status section can name the version that exposes the set, once it's been checked against the binary.

## Open Questions

None blocking. Whether to add an MCP prompt (the siblings each have a "discipline" prompt) is left to the skill follow-up.
