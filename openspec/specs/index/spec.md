# index Specification

## Purpose
`index.yaml` and its segments, entry shape, upsert on write, rebuild, check, and the rule that the file wins. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: Root and segments
`index.yaml` SHALL carry `format: episodes/0.1`, `segments` (each with `path` and `count`) and `entries` for the open months. Each sealed month, and `legacy` when it exists, SHALL have a segment at `index/<YYYY-MM>.yaml` or `index/legacy.yaml` carrying `format` and `entries`. Entries SHALL be sorted by id.

#### Scenario: Sealed segment listed
- **WHEN** September is sealed and holds 412 episodes
- **THEN** the root lists `{path: index/2026-09.yaml, count: 412}` and none of September's entries

### Requirement: Entry shape
Each entry SHALL mirror exactly `id`, `kind`, `occurred`, `container`, `ordinal`, `standpoint` and `timestamp` from its file, with `occurred` values double-quoted. Nothing else SHALL appear.

#### Scenario: Turn entry
- **WHEN** a turn is indexed
- **THEN** its entry has those seven fields (or fewer, when `container` and `ordinal` are absent), and no `label`, `content` or `links`

### Requirement: Every create updates the index
Creating an episode SHALL upsert its entry into the root. A create that opens a new month SHALL, in the same locked write, move every newly sealed month's entries into a new segment and record that segment's `count`. `observe` and `redact` SHALL NOT change the index.

#### Scenario: First episode of a new month
- **WHEN** the newest id is from November, the root holds October and November entries, and the first January id is minted
- **THEN** `index/2026-10.yaml` and `index/2026-11.yaml` are written and listed with their counts, no segment is written for December (which has no files), and the root's entries are the January episode alone

### Requirement: Segments never change
No verb SHALL rewrite an existing segment file.

#### Scenario: Rebuild leaves segments alone
- **WHEN** `index` is run and every sealed count matches
- **THEN** every segment file is byte-identical to before

### Requirement: Rebuild and check
`episodes index` SHALL rebuild the root from the files, replacing whatever was there, including git conflict markers, and SHALL write any missing segment. If a sealed month's count disagrees with its segment, it SHALL exit 4, naming the files, and leave that segment as it is. `episodes index --check` SHALL compare without writing and SHALL exit 4 on any missing, extra or changed entry or count, listing each.

#### Scenario: Conflict markers
- **WHEN** `index.yaml` holds conflict markers and `index` is run
- **THEN** a valid root is written from the files

#### Scenario: Drift detected
- **WHEN** a file exists with no root entry and `index --check` is run
- **THEN** the command exits 4 and names the id as missing

### Requirement: The file wins
No verb SHALL use an index entry where it disagrees with the file. `validate` SHALL report a disagreement between a file's indexed fields and its entry as an error (`revised`).

#### Scenario: Edited occurred
- **WHEN** a file's `occurred.start` is changed by hand and `validate` is run
- **THEN** an error `revised` names the episode and the field

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

