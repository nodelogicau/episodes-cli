# query Specification

## Purpose
`query`: recall by free text, following the format's model-free baseline. Introduced by change `recall-by-query`.
## Requirements
### Requirement: Query normalisation
`episodes query <text>` SHALL trim characters with the Unicode `White_Space` property from both edges of the query, and SHALL take its terms to be the runs of non-`White_Space` characters. It SHALL strip leading and trailing characters of Unicode general category P from each term and from the whole trimmed query before case folding. Characters inside a term SHALL be kept. Terms left empty SHALL be ignored. A query whose whole form and every term are empty SHALL resolve no episodes and SHALL NOT be an error.

#### Scenario: Punctuation at the edges
- **WHEN** `query '"invoice?"  (Ada)'` is run
- **THEN** the terms are `invoice` and `Ada`, and the whole query is `invoice?"  (Ada`

#### Scenario: Interior punctuation kept
- **WHEN** `query "e-mail"` is run
- **THEN** the one term is `e-mail`, and a text containing `email` but not `e-mail` does not match

#### Scenario: Empty query
- **WHEN** `query "  ?! "` is run
- **THEN** the result is an empty list and the exit status is 0

### Requirement: Lexical and uid matching
An episode SHALL match lexically when the folded whole query, or any folded term, occurs under Unicode case folding in its `label` or, for an inline episode, in its `content.text`. Terms SHALL be combined by OR, with no stemming and no stopwords. An episode SHALL match by uid when a term, before or after trimming, equals its `content.uid` exactly, with no folding. A field absent from the file, including one that has been redacted, SHALL NOT match.

#### Scenario: Case-folded substring
- **WHEN** an inline episode's text is `Paid the INVOICE today` and `query invoice` is run
- **THEN** the episode is returned with resolution `lexical`

#### Scenario: Any term matches
- **WHEN** `query "invoice receipt"` is run and one episode mentions only `receipt`
- **THEN** that episode is returned

#### Scenario: uid exact
- **WHEN** a reference episode has `content.uid: <abc@example.com>` and `query "<abc@example.com>"` is run
- **THEN** it is returned with resolution `uid`

#### Scenario: Redacted text does not match
- **WHEN** an episode's `label` matched before `label` was redacted, and the query is run again
- **THEN** the episode is not returned on account of `label`

### Requirement: Query results and order
Each result SHALL be one episode carrying its `container`, a `resolution` and the number of distinct folded terms it matched. `resolution` SHALL be `uid` when the uid rule matched, else `lexical`. A container SHALL be returned only when it matched itself. Results SHALL be ordered by resolution (`uid` before `lexical`), then by distinct folded terms matched, most first, then by `id` descending by code point. `--limit N` SHALL be applied after ordering.

#### Scenario: More terms rank higher
- **WHEN** `query "invoice ada"` is run, episode A matches both terms and episode B, with a later id, matches one
- **THEN** A is listed before B

#### Scenario: Ties by id descending
- **WHEN** two episodes each match one term
- **THEN** the one with the greater id is listed first

#### Scenario: Container not returned through a child
- **WHEN** a turn's text matches and its conversation container's label does not
- **THEN** the turn is returned with its `container`, and the container is not returned

### Requirement: Query filters and withdrawn episodes
`query` SHALL accept `--since`, `--until`, `--tz`, `--standpoint`, `--kind` and `--container` with the meanings they have for `list`, including *Date bounds in the reader's zone*, and `--participant P`, which keeps episodes whose `participants` include `P` as written. Filters SHALL be decided from the index and the episode files, never from a cache. A withdrawn episode SHALL never be returned.

#### Scenario: Withdrawn episode never returned
- **WHEN** a note whose `content.text` matched has had it redacted
- **THEN** `query` does not return the note, and no flag makes it do so

#### Scenario: Participant filter
- **WHEN** `query invoice --participant https://example.com/ada` is run
- **THEN** only matching episodes listing that participant exactly are returned

#### Scenario: Index lagging after a merge
- **WHEN** a merged branch added a matching episode without updating `index.yaml`
- **THEN** `query` returns it

#### Scenario: Since a local day
- **WHEN** a workspace has `defaults.zone: Australia/Melbourne`, a note reading "invoice" occurred at `2026-11-16T09:12:00+11:00`, and `query invoice --since 2026-11-16` is run
- **THEN** the note is returned, and the JSON result carries `zone: Australia/Melbourne`

