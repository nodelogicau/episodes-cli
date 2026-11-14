## 1. Implementation

- [x] 1.1 Add `--control` to `act` in `internal/cli/cmd_record.go`; refuse `--via` with `--control` (exit 2, usage) before minting
- [x] 1.2 Replace the harness check: refuse when a harness is resolved and neither `--via` nor `--control` is given, with a message naming both flags
- [x] 1.3 Add `attestation` (`via`, `control` or `author`) to the JSON result, and say which in the text result
- [x] 1.4 Update `act`'s `Use`, `Long` help and flag help, stating that `--control` is the caller's statement and cannot be checked

## 2. Tests

- [x] 2.1 Replace `TestHarnessWithoutVia` with tests for each scenario in *The attestation is named*: harness with neither flag, defaults harness with `--control`, connector harness with `--control`, both flags, and a person's own act
- [x] 2.2 Test *A control writes no marker*: a `--control` act's file matches a no-harness act's file apart from `source` and the id, and carries no `links`
- [x] 2.3 Assert `attestation: via` in the existing deep-work test, and run the upstream suite (its self-attested-acts scenarios must still pass)

## 3. Documentation

- [x] 3.1 README: the `act` row in the verb table and the quick-start comment
- [x] 3.2 `SPEC-FEEDBACK.md` item 7: what we now decide, with a link to nodelogicau/episodes#25 and its review comment
- [x] 3.3 `CHANGELOG.md`: an Unreleased entry referencing #4
