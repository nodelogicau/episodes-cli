# occurred-time Specification

## Purpose
RFC 9557 parsing and writing, offset/zone agreement, date-only values and `tz`, `basis`, and the rule that nothing is recorded before it happens. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: RFC 9557 values
`occurred.start` and `occurred.end` SHALL each be an RFC 3339 date-time with an offset (or `Z`), optionally followed by one bracketed IANA zone suffix, or else a date alone. Both SHALL be the same form. Writers SHALL write them as double-quoted YAML strings in episode files and in the index.

#### Scenario: Zoned value is quoted
- **WHEN** a turn is recorded with occurred `2026-11-07T14:24:00+11:00[Australia/Melbourne]`
- **THEN** the file contains `start: "2026-11-07T14:24:00+11:00[Australia/Melbourne]"`

#### Scenario: Mixed forms refused
- **WHEN** `--occurred 2026-11-03 --end 2026-11-03T10:00:00Z` is given
- **THEN** the write is refused with exit 2

### Requirement: Offset governs, zone only when known
The instant SHALL be the one the offset fixes. A writer SHALL NOT write a zone that contradicts the offset: when a supplied value's zone, under its rules at that local time, gives a different offset, the writer SHALL write the offset alone and say that it left the zone out. A zone the writer has no rules for SHALL be left out in the same way. `validate` SHALL warn on a file whose offset disagrees with its zone (`zone_offset_mismatch`) and still take the instant from the offset. A writer SHALL write a zone only when one is supplied (`--occurred` with a suffix, `--zone`, `EPISODES_ZONE`, or `defaults.zone`) and SHALL NOT infer one from the host. `--zone`, `EPISODES_ZONE` and `defaults.zone` SHALL apply only to a time the verb takes from the clock: never to a supplied value, to a `stated` or `inferred` time, to a value in `Z`, or to a date alone.

#### Scenario: Fall-back hour kept apart
- **WHEN** two turns are recorded at `2026-04-05T02:30:00+11:00[Australia/Melbourne]` and `2026-04-05T02:30:00+10:00[Australia/Melbourne]`
- **THEN** both are accepted and `list` orders them an hour apart

#### Scenario: Wrong offset for the zone
- **WHEN** `--occurred "2026-11-07T14:24:00+10:00[Australia/Melbourne]"` is given
- **THEN** `start` is written `"2026-11-07T14:24:00+10:00"`, and the result warns that the zone, whose offset is `+11:00`, was left out

#### Scenario: Default zone against a supplied offset
- **WHEN** `--occurred 2026-11-07T12:24:00+09:00 --basis recorded` is given in a workspace with `defaults.zone: Australia/Melbourne`
- **THEN** `start` is written `"2026-11-07T12:24:00+09:00"` with no zone

#### Scenario: Unknown zone
- **WHEN** `--occurred "2026-11-07T14:24:00+11:00[Mars/Olympus_Mons]"` is given
- **THEN** `start` is written `"2026-11-07T14:24:00+11:00"`, with a warning

#### Scenario: Default time with no zone configured
- **WHEN** a turn is recorded with no `--occurred` and no zone configured
- **THEN** `occurred.start` carries the local offset and no bracketed suffix

### Requirement: Suffixes and dates
A writer given an `occurred` bound by any verb SHALL drop an elective suffix other than the zone, record the rest, and say what it dropped. It SHALL refuse a critical suffix other than the zone, naming the suffix, and write nothing. For a critical zone, it SHALL check the offset against the zone's rules at that local time. When they agree, it SHALL write the value without the flag. When they disagree, or the zone is not one it has rules for, it SHALL refuse, naming the disagreement. A writer SHALL never write a critical flag or a suffix other than one zone. A reader SHALL ignore an elective suffix it does not implement and SHALL treat as invalid a critical suffix it cannot honour. A date alone MAY carry `tz`. `tz` beside a date-time SHALL be refused on write and SHALL be an error in `validate`.

#### Scenario: Calendar suffix dropped on write
- **WHEN** `--occurred "2026-11-07T14:24:00+11:00[Australia/Melbourne][u-ca=gregory]"` is given
- **THEN** `start` is written `"2026-11-07T14:24:00+11:00[Australia/Melbourne]"`, and the result says `[u-ca=gregory]` was dropped

#### Scenario: Critical calendar refused
- **WHEN** `--occurred "2026-11-07T14:24:00+11:00[!u-ca=hebrew]"` is given
- **THEN** the write is refused with exit 2, naming `[!u-ca=hebrew]`

#### Scenario: Critical zone that agrees
- **WHEN** `--occurred "2026-11-07T14:24:00+11:00[!Australia/Melbourne]"` is given
- **THEN** `start` is written `"2026-11-07T14:24:00+11:00[Australia/Melbourne]"`

#### Scenario: Critical zone that disagrees
- **WHEN** `--occurred "2026-01-15T10:00:00+10:00[!Australia/Melbourne]"` is given
- **THEN** the write is refused with exit 2, naming the expected offset `+11:00`

#### Scenario: Critical suffix invalid on read
- **WHEN** a file carries `start: "2026-11-07T14:24:00+11:00[!Australia/Melbourne]"`
- **THEN** `validate` reports an error

### Requirement: Basis
`occurred.basis` SHALL be one of `recorded`, `stated` or `inferred`. A verb that supplies `occurred` from the clock SHALL write `recorded`. A verb given `--occurred` SHALL require `--basis` and SHALL NOT default it. Any other basis, including `planned`, SHALL be refused.

#### Scenario: Supplied time without basis
- **WHEN** `record ref … --occurred "2026-11-07T16:02:00+11:00"` is run without `--basis`
- **THEN** the write is refused with exit 2 naming `basis`

#### Scenario: Planned refused
- **WHEN** `--basis planned` is given
- **THEN** the write is refused with exit 2, and the message says that `occurred` comes from the happening, never from a plan

### Requirement: Nothing is recorded before it happens
A writer SHALL refuse an episode whose `occurred.start` or `occurred.end` is later than its `timestamp`, comparing a date-time as an instant, and a date alone with the recording instant's date reckoned in `tz` when present and otherwise in UTC allowing one day. Where `occurred.basis` is `stated`, a date-time MAY be up to five minutes later than `timestamp`, and SHALL be recorded as the source stated it, not clamped. This allowance SHALL NOT apply to `recorded` or `inferred` times, and SHALL NOT add to the one day allowed for a date alone. `validate` SHALL warn (`occurred_after_recording`) only beyond the allowance that applies.

#### Scenario: Ninety minutes logged at the start
- **WHEN** at 09:05 an act is recorded with start 09:00 and end 10:30 the same day
- **THEN** the write is refused with exit 2 and the message gives the 85-minute difference

#### Scenario: Open-ended recording
- **WHEN** at 09:05 an act is recorded with start 09:00 and no end
- **THEN** the write succeeds

#### Scenario: Mail dated thirty seconds ahead
- **WHEN** at 03:23:30Z `record ref` is given a mail whose `Date` header is 03:24:00Z, with `--basis stated`
- **THEN** the write succeeds, `start` is written as stated, and `validate` reports no `occurred_after_recording`

#### Scenario: Beyond the allowance
- **WHEN** a stated date-time is five minutes and one second later than `timestamp`
- **THEN** the write is refused with exit 2, and `validate` warns on such a file

#### Scenario: No allowance for a recorded time
- **WHEN** a `recorded` date-time is thirty seconds later than `timestamp`
- **THEN** the write is refused with exit 2
