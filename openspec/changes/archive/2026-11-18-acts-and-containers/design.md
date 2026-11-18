## Context

Bootstrap D8 and D11 followed the earlier draft: an act is `mode: none` with no children, and `validate` recognised one by that shape. Upstream has since split the two meanings:
- `attested` marks an act;
- `none` marks a container with nothing of its own.

`model.Content.Mode` is a plain string, so readers that `switch` on it fall through to a default for any new mode. `observe` already refuses every mode except `ref`.

## Goals / Non-Goals

**Goals:** follow the upstream rules exactly; existing files keep validating without errors; no file is rewritten.

**Non-Goals:** migrating old acts to `mode: attested`. That would revise an episode, which accumulation forbids.

## Decisions

### D1. `ModeAttested` joins `Modes`

`model.ModeAttested = "attested"` is added to `Modes`, so `invalid_mode` accepts it and the error message lists it. The content rule for `attested` collects every set field (`ref`, `uid`, `version`, `hash`, `media_type`, `derivation`, `language`, `text`, and any undefined key). For each one it reports `attested_with_content` with `field` set to `content.<name>`, so each problem names its field, as #9 asks.

### D2. Act rules key on the mode

In `query/validate.go`, `v.act(ep, …)` runs when `ep.Content.Mode == ModeAttested`, and the child count is no longer consulted. A `mode: none` episode with no children gets one warning, `container_without_children`, for any `kind`. Its message names `conversation turn <id>` as the remedy for conversations and threads, and otherwise says that a child naming it as `container` makes it a container again. The old info-level finding with the same code goes, because the code is reused at warning level.

### D3. `container_later` between UUIDv7 ids only

The check uses `model.IsCanonicalID` on both ids. A non-UUIDv7 id carries no time, so comparing it by code point says nothing.

### D4. `act`

`act` sets `content: {mode: attested}`. `--language` is refused with `language_on_attested` before anything else happens. `conversation start` still writes `mode: none` for its container.

## Risks / Trade-offs

- **[Old binaries reading new acts]** → An older build reports `invalid_mode` on `mode: attested`. That is expected for a format change, and the CHANGELOG marks it as breaking.
- **[A real act written as `mode: none` is no longer checked]** → Its standpoint rule is no longer enforced. Upstream accepts this, and the warning still points at it.
