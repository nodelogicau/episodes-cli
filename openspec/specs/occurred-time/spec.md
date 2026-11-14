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
The instant SHALL be the one the offset fixes. A writer SHALL refuse a value whose offset disagrees with its zone's rules at that local time, and `validate` SHALL warn on one (`zone_offset_mismatch`) and still take the instant from the offset. A writer SHALL write a zone only when one is supplied (`--occurred` with a suffix, `--zone`, `EPISODES_ZONE`, or `defaults.zone`) and SHALL NOT infer one from the host.

#### Scenario: Fall-back hour kept apart
- **WHEN** two turns are recorded at `2026-04-05T02:30:00+11:00[Australia/Melbourne]` and `2026-04-05T02:30:00+10:00[Australia/Melbourne]`
- **THEN** both are accepted and `list` orders them an hour apart

#### Scenario: Wrong offset for the zone
- **WHEN** `--occurred "2026-11-07T14:24:00+10:00[Australia/Melbourne]"` is given
- **THEN** the write is refused with exit 2 naming the expected offset `+11:00`

#### Scenario: Default time with no zone configured
- **WHEN** a turn is recorded with no `--occurred` and no zone configured
- **THEN** `occurred.start` carries the local offset and no bracketed suffix

### Requirement: Suffixes and dates
A writer SHALL refuse a critical (`!`) suffix and any suffix other than one zone. A reader SHALL ignore an elective suffix it does not implement and SHALL treat as invalid a critical suffix it cannot honour. A date alone MAY carry `tz`. `tz` beside a date-time SHALL be refused on write and SHALL be an error in `validate`.

#### Scenario: Calendar suffix refused on write
- **WHEN** `--occurred "2026-11-07T14:24:00+11:00[u-ca=gregory]"` is given
- **THEN** the write is refused with exit 2

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
A writer SHALL refuse an episode whose `occurred.start` or `occurred.end` is later than its `timestamp`, comparing a date-time as an instant, and a date alone with the recording instant's date reckoned in `tz` when present and otherwise in UTC allowing one day. `validate` SHALL warn (`occurred_after_recording`) on any episode that breaks this rule.

#### Scenario: Ninety minutes logged at the start
- **WHEN** at 09:05 an act is recorded with start 09:00 and end 10:30 the same day
- **THEN** the write is refused with exit 2 and the message gives the 85-minute difference

#### Scenario: Open-ended recording
- **WHEN** at 09:05 an act is recorded with start 09:00 and no end
- **THEN** the write succeeds

