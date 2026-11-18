## Why

The draft no longer recognises a self-attested act by its shape (upstream change `acts-and-containers`, nodelogicau/episodes#20, #21, #25 and #34). An act's content is now `{mode: attested}`, a fourth mode alongside `inline`, `ref` and `none`, and `mode: none` means only a container with nothing of its own.

Today `act` writes `mode: none`, and `validate` applies the act rules to every `mode: none` episode that has no children. So a conversation whose first turn was never written fails rules that were never meant for it ([#8](https://github.com/nodelogicau/episodes-cli/issues/8), [#9](https://github.com/nodelogicau/episodes-cli/issues/9)).

## What Changes

- **BREAKING (file output):** `act` writes `content: {mode: attested}` with no other key, where it used to write `{mode: none}`. Existing files are not rewritten.
- `attested` is accepted as a mode everywhere `content.mode` is read: `show`, `list`, `thread`, `index` and `validate`.
- `content` with `mode: attested` and any other key, `language` included, is rejected, naming the field. `act --language` is refused, now naming mode attested.
- `validate` applies the act rules (`standpoint` equals `source.author`; a `via` target that resolves has the same `standpoint`) to every `mode: attested` episode, whatever its `kind` and whether or not it has children, and to no other episode.
- An episode with `mode: none` that no episode names as `container` is valid whatever its `standpoint`. `validate` warns `container_without_children`, naming it, where it used to give info plus the act rules. Acts written under the earlier draft read this way.
- `container_later` is checked only when both ids are UUIDv7.
- `--container <act id>` is accepted, since nothing refuses an act as a container.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `self-attested-acts`: `act` writes `mode: attested`; acts are recognised by that mode, not by shape; a childless `mode: none` episode warns.
- `content`: adds the `attested` mode and its no-other-key rule.
- `validation`: `container_later` only between UUIDv7 ids.

## Impact

- `internal/model` (`ModeAttested`, the content rule), `internal/query/validate.go` (the act and childless-container checks, the `container_later` guard), `internal/cli/cmd_record.go` (`act`), `render.go` (showing the mode in `thread`).
- Tests that assert `mode: none` for acts or the old act-by-shape rule are updated, and the upstream scenario table follows upstream.
- `README.md`, `CHANGELOG.md` and `SPEC-FEEDBACK.md` (items 2, 3, 7 and 16).
