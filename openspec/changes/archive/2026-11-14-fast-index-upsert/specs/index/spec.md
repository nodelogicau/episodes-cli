## ADDED Requirements

### Requirement: The upsert's root does not depend on how it is computed
When the root was written by this binary, a create's upsert SHALL write the bytes of the full upsert: the root as read, with entries for months the create seals moved to their segments, the new entry added, entries sorted by id, and the whole re-encoded. An implementation MAY compute these bytes without parsing or re-encoding the existing entries, but only when the create opens no new month, the new id sorts after the root's last entry, the root's first entry is in an open month, and the root's head and tail are in the form this binary writes, with no conflict markers. Otherwise the full upsert SHALL run. For a root not written by this binary that passes those checks, the result MAY differ from the full upsert's; it SHALL remain a root that `index --check` compares against the files and `index` rebuilds. When the create opens no new month, the upsert's cost SHALL NOT grow with the time it takes to parse and re-encode the root's entries.

#### Scenario: A turn in an open month
- **WHEN** a turn is created in a workspace whose root holds 20,000 entries from the current month, all with smaller ids
- **THEN** the root written is byte-identical to the one the full upsert would write, and the create completes without parsing the existing entries

#### Scenario: A create that seals a month
- **WHEN** the first id of a new month is created
- **THEN** the full upsert runs, writing the sealed months' segments and their counts, as *Every create updates the index* requires

#### Scenario: A root with conflict markers
- **WHEN** `index.yaml` holds git conflict markers and an episode is created
- **THEN** the full upsert runs, rebuilding the root from the files, as before this requirement

#### Scenario: A malformed middle is not detected
- **WHEN** a hand-edited root has a well-formed head and tail but an unparseable entry between them, and an episode is created in an open month
- **THEN** the new entry is appended without parsing, and `index --check` reports the root as unreadable until `index` rebuilds it

#### Scenario: Equivalence across a sequence
- **WHEN** a random sequence of creates, across several month boundaries and including out-of-order ids, is applied once with the fast path enabled and once with only the full path
- **THEN** after every create the two roots are byte-identical
