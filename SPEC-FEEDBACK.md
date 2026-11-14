# Feedback on the Episodes Format draft from the `episodes` reference implementation

The [Episodes Format](https://github.com/nodelogicau/episodes) README says that
no implementation exists. This file records the points where implementing the
draft forced a decision the text did not make, what this implementation
decided, and what we propose the text say. "We" below means this
implementation as it was when the item was raised.

Items 1 to 16 come from the design of the `bootstrap-episodes-cli` change
(v0.1.0). Items 17 and 18 were found while implementing it, when the
README's examples and the upstream acceptance scenarios were run as tests.
Items 1 to 16 were raised on 2026-11-13 as
[nodelogicau/episodes#19](https://github.com/nodelogicau/episodes/issues/19)
to [#34](https://github.com/nodelogicau/episodes/issues/34), one issue each,
and are open. Items 17 and 18 were raised on 2026-11-14 as
[#35](https://github.com/nodelogicau/episodes/issues/35) and
[#36](https://github.com/nodelogicau/episodes/issues/36).

| # | Topic | Status |
|---|---|---|
| 1 | `(container, ordinal)` uniqueness and assignment | open, [#19](https://github.com/nodelogicau/episodes/issues/19) |
| 2 | A `container` that does not resolve | open, [#20](https://github.com/nodelogicau/episodes/issues/20) |
| 3 | A childless container is read as a self-attested act | open, [#21](https://github.com/nodelogicau/episodes/issues/21) |
| 4 | A tolerance for "nothing is recorded before it happens" | open, [#22](https://github.com/nodelogicau/episodes/issues/22) |
| 5 | What a writer does on a second `(standpoint, media_type, uid)` | open, [#23](https://github.com/nodelogicau/episodes/issues/23) |
| 6 | Which bytes `hash` covers, per media type | open, [#24](https://github.com/nodelogicau/episodes/issues/24) |
| 7 | A harness recording a self-attested act should name the attesting turn | open, [#25](https://github.com/nodelogicau/episodes/issues/25) |
| 8 | The validator needs a registry snapshot | open, [#26](https://github.com/nodelogicau/episodes/issues/26) |
| 9 | Where a writer learns the zone | open, [#27](https://github.com/nodelogicau/episodes/issues/27) |
| 10 | Elective suffixes on input | open, [#28](https://github.com/nodelogicau/episodes/issues/28) |
| 11 | Redacting a field the episode never carried | open, [#29](https://github.com/nodelogicau/episodes/issues/29) |
| 12 | Rebuilding when a sealed segment's count is wrong | open, [#30](https://github.com/nodelogicau/episodes/issues/30) |
| 13 | A withdrawn turn in a conversation | open, [#31](https://github.com/nodelogicau/episodes/issues/31) |
| 14 | What immutability can be checked | open, [#32](https://github.com/nodelogicau/episodes/issues/32) |
| 15 | A sealed month with no files | open, [#33](https://github.com/nodelogicau/episodes/issues/33) |
| 16 | A container's standpoint | open, [#34](https://github.com/nodelogicau/episodes/issues/34) |
| 17 | The `EPISODE` example's `participants` are not sorted | open, [#35](https://github.com/nodelogicau/episodes/issues/35) |
| 18 | `english` is well-formed under the BCP 47 ABNF | open, [#36](https://github.com/nodelogicau/episodes/issues/36) |

---

## 1. `(container, ordinal)` uniqueness and assignment

**What the draft says.** `ordinal` is a position within `container`, from 1,
"required when `container` is present and order is meaningful". The Status
section asks whether `(container, ordinal)` should be unique, and names "a
replayed turn taking an `ordinal` already used" as a duplicate.

**What we decided.** A write that repeats a pair is refused with exit 2 and
error code `duplicate`, naming the episode that holds it. `conversation turn`
assigns one more than the highest existing ordinal, under the workspace lock.
`validate` reports a repeated pair as a warning (`ordinal_repeated`), not an
error, because two branches can each assign the same next ordinal and merge
without a conflict.

**Proposal.** A writer MUST NOT write a `(container, ordinal)` pair an episode
in the workspace already holds. A validator SHOULD warn on a repeated pair
and MUST NOT reject either episode.

## 2. A `container` that does not resolve

**What the draft says.** An unresolved *link* is reported and does not make
the episode invalid. Nothing is said of an unresolved `container`, or of a
container recorded after its child.

**What we decided.** On write, `--container` must name an existing episode
minted before the new one (exit 3 if missing, exit 2 if later). In
`validate`, a container that does not resolve is a warning
(`container_unresolved`), and one whose id is not earlier is a warning
(`container_later`).

**Proposal.** Treat an unresolved container as an unresolved link is
treated: reported, never invalid. Add that a container is recorded before
its children, so its id sorts first.

## 3. A childless container is read as a self-attested act

**What the draft says.** A validator treats as a self-attested act every
`mode: none` episode that no episode names as its container. Writers SHOULD
write a container together with its first child.

**What we decided.** `conversation start` writes the container and its first
turn under one lock, container first. A crash between the two renames, or a
partial copy, leaves a childless container, which the recognition rule reads
as an act and holds to the act rules. We follow the draft: `validate` applies
the act rules to it, and for `kind: conversation` or `thread` adds an info
finding, `container_without_children`, naming the remedy (`conversation turn
<id>` appends a child, which makes it a container again and changes no
existing file).

**Proposal.** Recognise a self-attested act by shape *and* by the absence of
a container-only `kind` (`conversation`, `thread`), or add a marker field to
containers. Either removes the ambiguity rather than leaving each
implementation to report it.

## 4. A tolerance for "nothing is recorded before it happens"

**What the draft says.** A writer MUST NOT record an episode whose
`occurred.start` or `occurred.end` is later than its `timestamp`. A validator
warns, rather than fails, because clocks disagree by minutes.

**What we decided.** No tolerance on write: the refusal (exit 2) names the
difference ("by 85 minutes"). A harness given a mail `Date` a few seconds
ahead of its own clock must wait and retry, or record the time as inferred.

**Proposal.** State a tolerance a writer may apply to a time a source
*states* (for example two minutes), so that writers agree on what is
refused, and keep the validator's warning for anything beyond it.

## 5. What a writer does on a second `(standpoint, media_type, uid)`

**What the draft says.** A validator MAY report reference episodes sharing
`standpoint`, `media_type` and `uid` as possible duplicates. What a writer
should do on finding one is listed as open.

**What we decided.** `record ref` with a `uid` looks for existing reference
episodes with the same `standpoint`, `media_type` and `uid` and refuses with
exit 2 code `duplicate`, listing them, unless given `--allow-duplicate` (for
a distinct occurrence that shares a `UID`, such as one instance of a
recurring event). Different standpoints never count.

**Proposal.** A writer SHOULD NOT record a second reference episode with the
same `standpoint`, `media_type` and `uid` without an explicit override, and
SHOULD cite the existing one instead.

## 6. Which bytes `hash` covers, per media type

**What the draft says.** `hash` is taken "over the content as fetched", CRLF
normalised to LF and nothing else changed.

**What we decided.** The harness chooses the bytes and passes them with
`--hash-file`; the binary normalises CRLF and hashes.

**Proposal.** Name the bytes per media type, at least for `message/rfc822`
(the raw message as the mailbox returns it) and `text/calendar` (the
VCALENDAR object containing the event). Otherwise two harnesses recording one
email write two different hashes, and `source_status` cannot be compared
across them.

## 7. A harness recording a self-attested act should name the attesting turn

**What the draft says.** A writer MUST record an act only on explicit
attestation, "a rule for writers" that no validator can check. When the
attestation is an episode in the workspace the act names it with `via`; a
dedicated control with no conversation has no `via`.

**What we decided.** v0.1.0 required `--via` whenever the resolved source
carried `harness`. That was wrong (episodes-cli#4): a harness can come from
`EPISODES_HARNESS` or `defaults.source.harness` when a person logs their own
act, and a dedicated control built as a connector has no turn to cite,
although the draft lets it omit `via`. We now follow the review on
[nodelogicau/episodes#25](https://github.com/nodelogicau/episodes/issues/25#issuecomment-6868142537):
the rule depends on how the act was attested. `act` takes `--via <episode>`
(the attestation is that episode) or `--control` (a dedicated control, so no
episode holds it). With a harness and neither, it is refused. With no harness,
neither is required. `--control` is the caller's statement and cannot be
checked, and nothing about it is written to the file.

**Proposal.** As in the #25 review: when the attestation was made in an
episode the writer recorded, which includes every attestation made in
conversation, a self-attested act MUST link `via` that episode. Decide it
together with #21 (an explicit act marker would let `--via` and `--control`
be recorded) and #34.

## 8. The validator needs a registry snapshot

**What the draft says.** A `rel` that is neither registered in the IANA Link
Relations registry nor an absolute URI is invalid, while a reader MUST NOT
reject an episode for a `rel` it does not know.

**What we decided.** We embed a dated snapshot of the registry (reported by
`version --json` as `link_relations`), refuse an unknown lowercase rel on
write, and report one as an error in `validate`. `show`, `list` and `thread`
display the episode whatever its rels.

**Proposal.** Every validator must otherwise carry the registry and agree on
its date, and a relation registered after a validator's snapshot is invalid
to it and valid to a newer one. Make an unregistered lowercase rel a
validator warning, not invalid.

## 9. Where a writer learns the zone

**What the draft says.** "The zone is written whenever it is known, and
never guessed."

**What we decided.** When a verb supplies `occurred` itself, it writes a
zone only when one is configured: `--zone`, then `EPISODES_ZONE`, then
`defaults.zone` in `episodes.yaml` (an implementation key the draft's
"unknown keys are ignored" admits). It never takes the host's zone. With a
zone configured the offset is the zone's at that instant; without one it is
the process's local offset and no suffix is written.

**Proposal.** Define `defaults.zone` in `episodes.yaml`, so a configured zone
is portable between implementations.

## 10. Elective suffixes on input

**What the draft says.** Writers MUST NOT write any suffix but the zone;
readers MUST ignore an elective suffix they do not implement.

**What we decided.** A writer given one (`--occurred
"…[u-ca=gregory]"`) refuses with exit 2 rather than strip it, since a
writer never silently changes what it was given.

**Proposal.** Say whether a writer given an elective suffix strips it or
refuses.

## 11. Redacting a field the episode never carried

**What the draft says.** "Every path listed in `fields` is absent from the
file."

**What we decided.** `redact` refuses a path the episode does not carry and
that no earlier block already lists, so `fields` never claims a removal that
did not happen. Listing an already-redacted path again is accepted.

**Proposal.** Say whether `fields` may list a path the episode never
carried.

## 12. Rebuilding when a sealed segment's count is wrong

**What the draft says.** "A segment never changes once written" and "the
index MUST be fully reconstructible" from the files. A late merge fails
validation, naming the files.

**What we decided.** `index` rebuilds the root and writes any missing
segment, but never rewrites an existing one. A sealed month whose directory
count differs from its segment is reported with the files and the remedy
(re-mint the branch's episodes before merging), the root keeps the segment's
own count so the mismatch stays visible, and the command exits 4.

**Proposal.** State that a rebuild fails on a late merge and names the
files, rather than rewriting the segment.

## 13. A withdrawn turn in a conversation

**What the draft says.** Redacting `content.text` withdraws an inline
episode: "it carries no record, so it is out of recall".

**What we decided.** `list` leaves a withdrawn episode out unless given
`--include-withdrawn`. `thread` keeps its position as a placeholder line with
no text, so the order of the remaining turns stays readable.

**Proposal.** Say whether a reader may show a withdrawn turn's place in its
container.

## 14. What immutability can be checked

**What the draft says.** Any change to an existing file other than
`source_status` and `redacted` is non-conformant, in an open month and a
sealed one alike.

**What we decided.** `validate` reports a file whose indexed fields disagree
with its index entry as `revised`. In a sealed month that is durable
evidence, because a segment never changes. In an open month, running `index`
rewrites the root and removes the evidence, and unindexed fields
(`content.text`, `label`, `links`) are protected only by version control
anywhere. The README says so.

**Proposal.** An informative note: only indexed fields in sealed months are
verifiable without the store's history.

## 15. A sealed month with no files

**What the draft says.** "Each sealed month … has a segment."

**What we decided.** No segment is written for a month nothing was minted
in: when the first January id seals October and November, segments are
written for those two and none for an empty December.

**Proposal.** Say so, so that two implementations list the same `segments`.

## 16. A container's standpoint

**What the draft says.** "Every episode is someone's." A `mode: none`
conversation container records nobody's act or utterance.

**What we decided.** A container's standpoint defaults to the resolved
`source.author`, overridable with `--container-standpoint`. The draft
already says a container is valid whatever its standpoint.

**Proposal.** Say whose a container is, or that its standpoint is the
person whose conversation it is.

## 17. The `EPISODE` example's `participants` are not sorted

**What the draft says.** `participants` is "sorted", and canonical order is
what makes two implementations' files byte-identical. The `EPISODE` example
lists `https://github.com/benfortuna` before `https://claude.ai`.

**What we decided.** We sort by code point, so our encoding of the example
lists `https://claude.ai` first and does not round-trip the README byte for
byte. Our golden test uses the sorted form.

**Proposal.** Sort the example, and say what "sorted" sorts by (we use
Unicode code point order of the reference string).

## 18. `english` is well-formed under the BCP 47 ABNF

**What the draft says.** A validator rejects a tag that is not well-formed,
and MUST NOT reject one for a subtag missing from the registry. The
upstream scenario "An ill-formed tag" expects `english` to be rejected.

**What we decided.** Under RFC 5646's ABNF, `english` is well-formed: a
primary language subtag may be five to eight letters (reserved for
registration). To satisfy the scenario without a registry, we also reject a
primary subtag of four or more letters, which no language has. That is the
one place the check goes beyond the grammar.

**Proposal.** Either use an example that is ill-formed under the ABNF (such
as `en_AU` or `en-`), or state that a primary subtag longer than three
letters is rejected.
