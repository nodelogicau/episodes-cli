## 1. Model

- [x] 1.1 Add `ModeAttested` to `Modes`; add the `attested` content rule, reporting `attested_with_content` once per extra field (D1)

## 2. Validate

- [x] 2.1 Apply the act rules to `mode: attested` only; warn `container_without_children` for a `mode: none` episode with no children, whatever its kind (D2)
- [x] 2.2 Check `container_later` only when both ids are UUIDv7 (D3)

## 3. act

- [x] 3.1 Write `content: {mode: attested}`; refuse `--language` as `language_on_attested` (D4)
- [x] 3.2 Check that `show`, `list` and `thread` render `attested` without special cases

## 4. Tests and docs

- [x] 4.1 Update tests that expected `mode: none` acts or act rules by shape; add the scenarios in this change's specs, including an act as a container and a non-UUIDv7 `container_later`
- [x] 4.2 Upstream table: follow the upstream scenarios for acts and childless containers
- [x] 4.3 `README.md`, `CHANGELOG.md` (breaking: the act file output), and `SPEC-FEEDBACK.md` items 2, 3, 7 and 16
