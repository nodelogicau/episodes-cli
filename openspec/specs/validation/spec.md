# validation Specification

## Purpose
`validate` as the workspace invariant. Introduced by change `bootstrap-episodes-cli`.
## Requirements
### Requirement: Validate the workspace
`episodes validate` SHALL check every episode file, `episodes.yaml`, the root index and every segment. It SHALL report findings with `severity` (`error`, `warning` or `info`), `code`, `id` or `path`, and `message`, and SHALL exit 4 when any error is found. It SHALL write nothing.

#### Scenario: Clean workspace
- **WHEN** `validate --json` is run on a workspace written only by this binary
- **THEN** the result carries no errors and the command exits 0

### Requirement: Structural errors
`validate` SHALL report as errors: an unparseable file or duplicate key; a missing required field; an invalid `type`, `kind`, `basis`, `derivation`, `mode` or `source_status.state`; `text` on a reference episode; `uid` on an inline episode; `uid` without `media_type`; `language` on `mode: none`, malformed, or `und`; any key besides `mode` on `mode: attested`; `tz` beside a date-time; mixed `start` and `end` forms; a critical suffix; a link without `rel` and `href` or with a key besides `rel`, `href` and `anchor`, or whose rel is neither of registered form nor an absolute URI; an `anchor` that is not `content.ref` or is on an episode without one; a `memento` link without `anchor` or with an episode-id `href`; a redaction path outside the closed set or a missing redaction field; inline without `text` or reference without `ref` unless redacted; a misplaced file; a sealed count mismatch; an id from two or more months ahead; a self-attested act (`mode: attested`) whose standpoint differs from its author or from its resolved `via` target; and a `revised` entry.

#### Scenario: Several errors reported together
- **WHEN** a workspace has a reference episode with `text` and a misplaced file
- **THEN** both errors are reported in one run

### Requirement: Warnings
`validate` SHALL report as warnings: an offset that disagrees with its zone; `occurred` later than `timestamp`; an id or `timestamp` later than now; a repeated `(container, ordinal)`; an unresolved `container`; a `container` whose id sorts after the child's, when both ids are UUIDv7 (with no check when either is not); a `mode: none` episode with no children (`container_without_children`); a `content.language` subtag known to be missing from the registry (`language_unregistered`); a rel of registered form missing from the link-relations snapshot (`link_rel_unregistered`); and an `up` or `collection` link to the episode's own container.

#### Scenario: Two branches assigned the same ordinal
- **WHEN** two turns of `epi_C` both carry ordinal 5
- **THEN** a warning `ordinal_repeated` names both

#### Scenario: Container order not checked for non-UUIDv7 ids
- **WHEN** a child's id is `epi_import-1` and its container's is a UUIDv7
- **THEN** no `container_later` warning is reported

### Requirement: Info findings
`validate` SHALL report at info level: a non-UUIDv7 id; an unresolved link or `via`; a hash algorithm it does not implement; and reference episodes sharing `standpoint`, `media_type` and `uid` (`possible_duplicate`).

#### Scenario: Possible duplicate
- **WHEN** two reference episodes from Ada share `mid:a@b` and `message/rfc822`
- **THEN** an info finding `possible_duplicate` names both, and neither is invalid

### Requirement: Standalone
`validate` SHALL need no DKF or intentions code, data or workspace, and SHALL NOT report anything about objects of other formats.

#### Scenario: Reference to a DKF id
- **WHEN** a participant is written as `clm_01…`
- **THEN** `validate` treats it as an opaque reference and reports nothing about it

