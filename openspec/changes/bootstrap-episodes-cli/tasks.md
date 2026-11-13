## 1. Project Setup

- [x] 1.1 Pin Go in `mise.toml` (`go = "1.26.7"`, same as the siblings); `go mod init github.com/nodelogicau/episodes-cli`
- [x] 1.2 Add dependencies `spf13/cobra`, `gopkg.in/yaml.v3`, `github.com/google/uuid`, `golang.org/x/sys`; blank-import `time/tzdata` in `cmd/episodes`
- [x] 1.3 Create the package skeleton: `cmd/episodes`, `internal/{apperr,temporal,model,prov,store,query,cli}`
- [x] 1.4 Copy `Makefile`, `.golangci.yml`, `.gitattributes`, `.gitignore` and the GitHub Actions (test and lint on PR, release matrix on tag) from `intentions-cli`, renaming binary and module
- [x] 1.5 Copy `internal/apperr` from `intentions-cli` unchanged (exit codes 0–5; codes `usage`, `not_found`, `check_failed`, `no_workspace`, `refused`, `invalid`), and add `duplicate`
- [x] 1.6 Add an origin header (source repo, path, commit) to every copied file (D2)

## 2. Temporal (`internal/temporal`)

- [x] 2.1 Copy zone loading and offset-at-instant from `intentions-cli/internal/temporal`
- [x] 2.2 RFC 9557 parser: RFC 3339 date-time plus suffixes; one IANA zone accepted; critical flag detected; elective suffixes recorded so writers can refuse them and readers can ignore them
- [x] 2.3 Date-alone values with optional `tz`; same-form check for `start` and `end`
- [x] 2.4 Offset–zone agreement check at the local time, including the fall-back overlap and spring-forward gap
- [x] 2.5 Instant comparison and "not after recording" for date-times, for dates in `tz`, and for dates in UTC with one day's allowance
- [x] 2.6 Formatter: offset always, zone only when supplied; the recorded-now default from the process offset plus a configured zone
- [x] 2.7 Table tests for every `occurred-time` scenario here and upstream, including Melbourne 2026-04-05 02:30 at both offsets and the 2026-10-04 gap

## 3. Format Layer (`internal/model`)

- [x] 3.1 Types `Episode`, `Occurred`, `Content`, `Link`, `Source`, `SourceStatus`, `Redacted`, with canonical order documented on each
- [x] 3.2 Id minting `epi_<uuidv7>` (monotonic) and lenient parsing; UUIDv7 instant extraction for sharding
- [x] 3.3 Node-level decoder: any key order, duplicate-key and non-mapping errors, ordered extras retained
- [x] 3.4 Encoder: canonical order, extras placement, `redacted` last, quoted `occurred`, literal `text`, sorted `participants` and `links`, UTC `Z` instants
- [x] 3.5 Field rules shared by writes and `validate`: required fields, `kind` pattern, enums, mode-specific content rules, `uid` ⇒ `media_type`, `ref` ≠ episode id, `none` without `language`
- [x] 3.6 Hash: `sha256` over CRLF→LF bytes, `<alg>:<digest>` parsing, the unverified-algorithm result (copied from `particulars-cli`)
- [x] 3.7 `mid:` conversion from a `Message-ID` value (strip angle brackets, RFC 2392 percent-encoding)
- [x] 3.8 BCP 47 well-formedness from the RFC 5646 ABNF (hand-written), §2.1.1 case canonicalisation, `und` refusal, case-insensitive comparison
- [x] 3.9 Embedded IANA Link Relations snapshot (dated) and the rel check (registered or absolute URI)
- [x] 3.10 Golden-file tests: every README example and every upstream spec example round-trips byte-identically; reordered input re-emits canonically; extras (`occasion`) kept

## 4. Provenance (`internal/prov`)

- [x] 4.1 Copy from `particulars-cli/internal/prov`; rename the environment variables to `EPISODES_AUTHOR`, `EPISODES_HARNESS`, `EPISODES_MODEL`
- [x] 4.2 Resolution order flag → environment → `defaults.source`; require author or harness; keep the placeholder check

## 5. Store (`internal/store`)

- [x] 5.1 Workspace discovery (`--workspace`, strict `EPISODES_WORKSPACE`, `episodes.yaml` or `.episodes` pointer), `episodes.yaml` load with the format refusal, `defaults.zone` (copied from `particulars-cli`)
- [x] 5.2 Path derivation from id (month from the UUIDv7 instant in UTC; `legacy/`), adapted from `particulars-cli/internal/store/sharded.go`
- [x] 5.3 Sealing computation (newest month plus grace), refusal to mint into a sealed month, and a count check against segments
- [x] 5.4 Advisory lock on `episodes.yaml`: `flock` on unix, `LockFileEx` on windows; held from mint through index upsert
- [x] 5.5 `Create`: exclusive create, atomic temp-file-and-rename, index upsert, segment roll-over on a new month (no segment for empty months)
- [x] 5.6 `Amend`: the single mutation path, with the guard comparing every field except `source_status`, `redacted` and the redacted fields; atomic rewrite; no index change
- [x] 5.7 Index root and segment codec, entry shape (seven fields), rebuild that never rewrites an existing segment, and `--check` diff
- [x] 5.8 Directory enumeration plus index merge for recall (D12): parse ids the index lacks, drop entries whose file is gone
- [x] 5.9 Tests: concurrent writers (distinct ordinals, no torn index), killed writer releases the lock, the guard refuses a stray change, sealing across a year boundary, a late-merge count mismatch

## 6. Recording Verbs (`internal/cli`)

- [x] 6.1 Copy the cobra root, global flags (`--workspace`, `--json`, `--now`, attribution) and the text and JSON renderers from `intentions-cli`
- [x] 6.2 `init` (`--author`, `--harness`, `--zone`), `workspace`, `workspace pointer`
- [x] 6.3 `record inline`: text from flag, file or stdin; recorded-now default; `--occurred` requires `--basis`; containment; links
- [x] 6.4 `record ref`: required `--occurred` and `--basis`; `--uid` or `--message-id`; `--hash` or `--hash-file`; duplicate lookup and `--allow-duplicate` (D9)
- [x] 6.5 `conversation start`: container then first turn under one lock; `--container-standpoint`; both ids returned
- [x] 6.6 `conversation turn`: ordinal assignment under the lock; `(container, ordinal)` uniqueness; `--standpoint` required
- [x] 6.7 `act`: standpoint from author; `--via` same-standpoint check; `--via` required with a harness; `basis` limited to `stated` or `inferred`
- [x] 6.8 Write-time refusals with a named rule for every MUST and MUST NOT: future `occurred`, offset–zone mismatch, suffixes, `planned`, `up` or `collection` to own container, unresolved or later container and link targets
- [x] 6.9 CLI tests for every `recording`, `self-attested-acts`, `content`, `links` and `object-format` scenario

## 7. Mutation Verbs

- [x] 7.1 `observe`: stated state, or computed from `--content-file` / `--version`; `verified` and `detected` semantics; refused on inline and `none`
- [x] 7.2 `redact`: closed set, absent-path refusal, union, block last, git-history `notice`
- [x] 7.3 Tests for every `source-status`, `redaction` and `accumulation` scenario, including byte-identical neighbours after a redaction

## 8. Recall (`internal/query`, `internal/cli`)

- [x] 8.1 `show <id> [--children]`, locating the file by path derivation without reading the index
- [x] 8.2 `list` filters, occurred-instant ordering with date-only handling, `--by recorded`, `--include-withdrawn`, tombstone marking
- [x] 8.3 `thread <container>` text rendering with standpoint labels and withdrawn placeholders
- [x] 8.4 Tests for every `recall` scenario, including recall over a lagging index after a simulated merge

## 9. Validation

- [x] 9.1 `validate` as pure functions over a loaded workspace: every error, warning and info finding in `specs/validation`
- [x] 9.2 Self-attested-act recognition by shape; `container_without_children` with its remedy
- [x] 9.3 `revised` detection against root and segment entries
- [x] 9.4 Upstream acceptance suite: one test per scenario in `nodelogicau/episodes` `openspec/specs/*` (40 requirements, 78 scenarios), each named after its scenario
- [x] 9.5 Standalone check: the module imports nothing from `particulars-cli` or `intentions-cli`, enforced by a `go list -deps` test

## 10. Performance

- [x] 10.1 Benchmark `Create` with 20,000 open-month entries (target under 100 ms on a laptop, D14); record the result in `docs/benchmarks.md`
- [x] 10.2 Benchmark `record ref` duplicate lookup at 20,000 reference episodes; record it, and open a follow-on if it exceeds 500 ms

## 11. Documentation and Feedback

- [x] 11.1 README: install from source, quick start (init → conversation start → turn → act → record ref → observe → redact → thread → validate), the verb table, exit codes, discovery, and what immutability can and cannot be checked (D12)
- [x] 11.2 `SPEC-FEEDBACK.md` from the design's sixteen items, in the siblings' format (topic, what the draft says, what we decided, the proposal)
- [x] 11.3 `CHANGELOG.md` entry for v0.1.0
- [x] 11.4 Raise each feedback item as an issue on `nodelogicau/episodes`, after review by the person
