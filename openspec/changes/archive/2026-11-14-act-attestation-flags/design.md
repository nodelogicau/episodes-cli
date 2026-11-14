## Context

`act` (in `internal/cli/cmd_record.go`) refuses when `c.src.Harness != "" && via == ""`. The harness is resolved by `prov.Resolve` from the flag, then `EPISODES_HARNESS`, then `defaults.source.harness`. Issue #4 and the review on nodelogicau/episodes#25 show the condition tests the wrong thing: the draft distinguishes acts attested in an episode, which link `via` it, from acts attested through a dedicated control, which have no `via`. A harness field is present in both.

The episode file cannot record which case applied without a format change, and nodelogicau/episodes#21 (an explicit act marker) is still open.

## Goals / Non-Goals

**Goals:**
- Let a caller say how an act was attested, and refuse when a harness is involved and it says nothing.
- No change to the file, the index or `validate`, so no workspace changes meaning.

**Non-Goals:**
- Writing an attestation marker into the episode. That waits for nodelogicau/episodes#21.
- The narrower validator check the upstream review sketches (a harness that recorded a same-standpoint turn in the act's window should link `via`). Upstream calls that a separate question.
- Changing the act rules `validate` applies.

## Decisions

### D1. An explicit `--control` flag, not a relaxed default

The caller states the case: `--via <epi>` (attested in an episode) or `--control` (attested by a dedicated control). With a harness and neither, `act` is refused, so a harness that forgets the attesting turn still fails loudly.

**Alternative rejected:** ignore a harness that came from `EPISODES_HARNESS` or `defaults` for this check. That fixes the defaults case by guessing which values the person "really" set, does nothing for a connector passing `--harness`, and needs `prov` to report where each value came from.

**Alternative rejected:** drop the rule and let any act omit `via`. That loses the one mechanical guard against a harness recording an act from a chat turn without citing it.

### D2. `--control` is a boolean, named for the draft's case

The draft's words are "a control whose only purpose is to log the act". The flag takes no value. It is mutually exclusive with `--via` (both → exit 2, usage), and accepted without a harness, where it changes nothing but the result's `attestation`.

### D3. The result reports the attestation; the file does not

`attestation` (`via`, `control` or `author`) goes in the JSON result, so a harness can log what it claimed. Writing it to the file would add a field the format does not define. Readers would ignore it and `validate` could not use it, and adding it later when nodelogicau/episodes#21 decides is straightforward.

### D4. Checks run before minting, in a fixed order

Order: both flags → usage; no author → refused; standpoint mismatch → refused; harness with neither flag → refused; then the existing `via` resolution. A failed check therefore writes nothing, as today.

## Risks / Trade-offs

- [A harness passes `--control` to get past the refusal for an act it inferred] → It is a false statement the binary cannot detect, which is the same trust the CLI already gives `--harness` and `--author`. The help text says `--control` is the caller's statement, and the skill (a follow-on) will say when it applies. The result's `attestation: control` makes the claim visible in logs.
- [Upstream settles #25 differently] → The rule lives in one place in `actCmd`, and SPEC-FEEDBACK item 7 tracks it.

## Migration Plan

None. Every call that succeeded before still succeeds with the same file. Rollback is reverting the commit.

## Open Questions

None blocking. If nodelogicau/episodes#21 adopts an explicit marker, `--via` and `--control` are where it would be set.
