## Why

`act` refuses to record a self-attested act without `--via` whenever the resolved source carries a `harness` (design D11 of `bootstrap-episodes-cli`). Whether a harness field is present is not the same as how the act was attested, and the rule refuses legitimate acts in two cases ([#4](https://github.com/nodelogicau/episodes-cli/issues/4)):

- **Through defaults.** A harness can come from `EPISODES_HARNESS` or from `defaults.source.harness`, which `init --harness` writes. In such a workspace, a person running `episodes act` themselves, with no turn to cite, is refused.
- **Through connectors.** nodelogicau/episodes#17 lets `source.harness` name a non-AI connector. A "log my run" control built as a connector would have to cite a turn that doesn't exist, although the draft explicitly allows such a control to omit `via`.

Upstream, nodelogicau/episodes#25 made the same proposal as D11. Its review comment says the rule should depend on how the act was attested. This change brings the CLI in line with that before workspaces depend on the old behaviour.

## What Changes

- `act` gains `--control`. It is the caller's statement that the attestation was a dedicated control whose only purpose is to log the act, so that no episode holds the attestation. It is accepted with or without a harness, and refused together with `--via`.
- `--via <epi>` is unchanged, including the checks that the target exists, is earlier and has the act's standpoint.
- With a harness resolved and neither `--via` nor `--control`, `act` is still refused (exit 2). The message now names both flags.
- With no harness and neither flag, `act` is accepted as today: the person's own act.
- The JSON result gains `attestation`: `via`, `control` or `author`. Nothing new is written to the episode file, so files, the index and `validate` are unchanged.
- The help text, the README's `act` row and quick-start comment, and `SPEC-FEEDBACK.md` item 7 are updated. Item 7 now points at nodelogicau/episodes#25 and its review comment.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `self-attested-acts`: *A harness names the attestation* is replaced by a rule on how the act was attested (`--via` or `--control`, refused when a harness is resolved and neither is given). *Record a self-attested act* gains `--control` in its synopsis.

## Impact

- `internal/cli/cmd_record.go` (`actCmd`), with tests in `internal/cli/record_test.go` and the upstream suite.
- `README.md`, `SPEC-FEEDBACK.md` and `CHANGELOG.md`.
- No change to `internal/model`, `internal/store`, `internal/query`, the file format or the index.
- Compatibility: harness callers that pass `--via` are unaffected. Harness callers that were refused can now pass `--control` when that statement is true. Nothing that previously succeeded is refused.
