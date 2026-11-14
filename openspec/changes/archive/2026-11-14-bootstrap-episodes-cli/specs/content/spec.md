## ADDED Requirements

### Requirement: Inline content
An inline episode SHALL carry `content.mode: inline`, `media_type` (default `text/markdown`), `derivation` (one of `verbatim`, `transcribed`, `ocr`, `rendered`; default `verbatim`) and `text`. `text` MAY be absent only when `redacted.fields` lists `content.text`. An inline episode SHALL NOT carry `uid`.

#### Scenario: Transcript derivation
- **WHEN** `record inline --kind transcript --derivation transcribed --text-file t.txt` is run
- **THEN** the file carries `derivation: transcribed` and the file's text

#### Scenario: Inline with uid refused
- **WHEN** `record inline … --uid mid:a@b` is run
- **THEN** the write is refused with exit 2

#### Scenario: Missing text without redaction
- **WHEN** a file has `mode: inline`, no `text` and no `redacted` block
- **THEN** `validate` reports an error

### Requirement: Reference content and the rule against ingestion
A reference episode SHALL carry `content.mode: ref` and `ref`, and optionally `uid`, `version`, `hash`, `media_type` and `language`. `ref` SHALL NOT match the episode id pattern. A reference episode SHALL NOT carry `text`. `ref` MAY be absent only when `redacted.fields` lists `content.ref`.

#### Scenario: Ref naming an episode refused
- **WHEN** `record ref --ref epi_01a1… …` is run
- **THEN** the write is refused with exit 2, and the message points to `--link`

#### Scenario: Text on a reference refused
- **WHEN** `record ref … --text "body"` is run
- **THEN** the write is refused with exit 2

### Requirement: Content identity
`uid` SHALL require `media_type`. `--message-id <value>` SHALL be converted to a `mid:` URI by removing the surrounding angle brackets and percent-encoding as RFC 2392 requires, and SHALL be refused together with `--uid`. A `uid` given with `--uid` SHALL be written verbatim.

#### Scenario: Message-ID conversion
- **WHEN** `record ref --ref gmail:msg_1 --message-id "<CAF3x9q@mail.example.com>" --media-type message/rfc822 …` is run
- **THEN** the file carries `uid: mid:CAF3x9q@mail.example.com`

#### Scenario: uid without media type
- **WHEN** `record ref --uid 7q2m4k9d0s1a@google.com` is run without `--media-type`
- **THEN** the write is refused with exit 2

### Requirement: Hash
`--hash-file <path|->` SHALL set `hash` to `sha256:` plus the lowercase hex SHA-256 of the bytes with every CRLF replaced by LF and nothing else changed. `--hash` SHALL be accepted on write only as `sha256:<64 hex>`. Readers SHALL accept any `<algorithm>:<digest>`, and `validate` SHALL report an algorithm it does not implement as unverified (info), never as invalid.

#### Scenario: CRLF normalised, whitespace kept
- **WHEN** two files differing only in CRLF versus LF line endings are hashed
- **THEN** the hashes are equal, and a file differing in a trailing space hashes differently

#### Scenario: Unknown algorithm on read
- **WHEN** a file carries `hash: blake3:ab12…`
- **THEN** `validate` reports info `hash_unverified` and no error

### Requirement: The none mode
`content: {mode: none}` SHALL be written only by `conversation start` (for the container) and `act`. It SHALL NOT carry `language` or any other content field.

#### Scenario: Language on none refused
- **WHEN** `act … --language en` is run
- **THEN** the write is refused with exit 2

### Requirement: Language
`language` SHALL be one BCP 47 tag that is well-formed under RFC 5646, written in the case RFC 5646 §2.1.1 recommends, and SHALL NOT be `und`. Tags SHALL be compared without regard to case. A tag that is not well-formed SHALL be refused on write and SHALL be an error in `validate`. No registry check SHALL be made.

#### Scenario: Case normalised
- **WHEN** `--language EN-au` is given
- **THEN** the file carries `language: en-AU`

#### Scenario: Undetermined refused
- **WHEN** `--language und` is given
- **THEN** the write is refused with exit 2, and the message says to leave out a language that is not known
