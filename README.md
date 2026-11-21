# episodes

A command-line tool for the [Episodes Format](https://github.com/nodelogicau/episodes):
episodic memory that keeps its standpoint. It records what happened, when and
where, from whose standpoint, and where the record of it lives.

`episodes` is the reference implementation of the draft `episodes/0.1`. It is
a sibling of [`particulars`](https://github.com/nodelogicau/particulars-cli)
(what is believed) and [`intentions`](https://github.com/nodelogicau/intentions-cli)
(what a person means to do), and works the same way. It is built to be
driven by an LLM harness from a shell and reviewed by people through git. It
never prompts, every verb supports `--json`, and it performs no git
operations.

Episodes differ from their siblings in one way: **they accumulate and are
never revised.** After an episode is written, only two changes to its file
are allowed. `observe` records whether its source can still be reached, and
`redact` removes fields. No verb edits or deletes an episode, and the store
refuses every other rewrite.

> The format is an early draft and so is this tool. Where the draft left a
> behaviour undecided, this implementation decided and recorded it in
> [SPEC-FEEDBACK.md](SPEC-FEEDBACK.md).

## Install with Homebrew

On macOS or Linux, from the same tap as `particulars` and `intentions`:

```sh
brew install nodelogicau/tap/episodes
brew upgrade nodelogicau/tap/episodes   # later, for a new release
```

The cask installs the release archive for your platform, checked against
its SHA-256 sum, and clears macOS's quarantine flag from the binary, since
it is not signed or notarised.

## Install from a release

Each [release](https://github.com/nodelogicau/episodes-cli/releases) has an
archive for darwin, linux and windows on amd64 and arm64, and a
`checksums.txt` of their SHA-256 sums. On macOS or Linux:

```sh
VERSION=0.2.0
OS=darwin           # or linux
ARCH=arm64          # or amd64
ARCHIVE=episodes_${VERSION}_${OS}_${ARCH}.tar.gz
BASE=https://github.com/nodelogicau/episodes-cli/releases/download/v${VERSION}

curl -fL -O "$BASE/$ARCHIVE" -O "$BASE/checksums.txt"
shasum -a 256 --check --ignore-missing checksums.txt   # sha256sum on Linux
tar -xzf "$ARCHIVE" episodes
sudo install -m 0755 episodes /usr/local/bin/      # or anywhere on your PATH
episodes --version
```

The binaries are not signed or notarised. If macOS refuses to run one,
clear the quarantine flag with `xattr -d com.apple.quarantine episodes`.

On Windows, download `episodes_<version>_windows_<arch>.zip`, check it with
`Get-FileHash -Algorithm SHA256` against its line in `checksums.txt`, and
put `episodes.exe` on your `PATH`.

## Install from source

Requires Go 1.26.7 (pinned in `mise.toml`).

```sh
git clone https://github.com/nodelogicau/episodes-cli
cd episodes-cli
make build          # dist/episodes, static, CGO_ENABLED=0
make test           # the whole suite, including the upstream acceptance scenarios
make cross          # darwin, linux and windows on amd64 and arm64
```

The binary has no runtime dependencies and carries the IANA time-zone
database.

## Quick start

```sh
# A workspace for Ada. --zone is written whenever the tool records the time itself.
episodes init ~/memory --author https://example.com/ada --zone Australia/Melbourne
cd ~/memory
export EPISODES_HARNESS=claude

# A conversation: the container and its first turn are written together.
episodes conversation start --standpoint https://example.com/ada \
  --text "log ninety minutes of deep work from nine" --json
#   → container epi_C…, turn epi_T… (ordinal 1)

# The assistant's reply. Every turn states its own standpoint.
echo "Logged." | episodes conversation turn epi_C… --standpoint https://claude.ai --text-file -

# The act the person asked for. A harness says how it was attested: --via the turn,
# or --control when the person used a dedicated logging control.
episodes act --occurred "2026-11-08T09:00:00+11:00[Australia/Melbourne]" \
  --end "2026-11-08T10:30:00+11:00[Australia/Melbourne]" --basis stated \
  --label "Deep work on the budget draft" --via epi_T…

# A trace of an email Jane sent. Its Message-ID becomes uid: mid:…, and its text is never copied.
episodes record ref --kind email --standpoint https://example.com/jane \
  --ref gmail:msg_18f2c3a9 --message-id "<CAF3x9q@mail.example.com>" \
  --media-type message/rfc822 --occurred "2026-11-07T14:24:00+11:00" --basis stated \
  --hash-file message.eml

# Later: the email was deleted at the source.
episodes observe epi_E… --state gone

# The person withdraws a turn. The file keeps its place; the text leaves the file, not git history.
episodes redact epi_T… --fields content.text --reason "No longer wanted"

episodes thread epi_C…
episodes list --standpoint https://example.com/ada --since 2026-11-02 --until 2026-11-09
episodes validate
```

## Verbs

A `--since` or `--until` given as a date alone, on `list` or `query`, is a
day where you are: in `--tz`, then `EPISODES_ZONE`, then the workspace's
`defaults.zone`, then UTC, never the host's zone. So in a workspace made with
`--zone Australia/Melbourne`, `--since 2026-11-16` starts at midnight in
Melbourne. `--json` reports the zone used as `zone`. A date-time bound is its
own instant. Stored dates are not affected: an `occurred` date with no `tz`
is still a UTC day, as the format says.

| Verb | What it does |
|---|---|
| `init [dir]` | Create `episodes.yaml`, `episodes/` and an empty `index.yaml`. `--author`, `--harness` and `--zone` go to `defaults`. |
| `workspace` | Show the resolved workspace, how it was found (`flag`, `env`, `marker`, `pointer`), and its configuration. |
| `workspace pointer [dir] [--at D] [--force]` | Write a `.episodes` pointer naming the workspace. |
| `record inline` | Record content that has no other home: a note, or a transcript the harness made. Without `--occurred`, the time is now and the basis is `recorded`. |
| `record ref` | Record a trace of content that lives elsewhere (`--ref`, `--uid` or `--message-id`, `--version`, `--hash` or `--hash-file`). `--memento <uri>` (repeatable) records an archived snapshot you already made as a `memento` link anchored to `--ref`; recording never submits anything to an archive. `--occurred` and `--basis` are required. A repeated `(standpoint, media_type, uid)` is refused unless you pass `--allow-duplicate`. |
| `conversation start` | Write a conversation container (`content: {mode: none}`) and its first turn, under one lock. |
| `conversation turn <container>` | Append a turn. It is given the next ordinal under the lock. `--standpoint` is required on every turn. |
| `import [<file>\|-]` | Create many episodes from NDJSON, one line per episode, checked together and written in chunks of 1,000 (see [Bulk import](#bulk-import)). |
| `act` | Record a self-attested act: its standpoint is `source.author` and its content is `{mode: attested}`, a mode that carries no other key. When a harness records it, it must say how the person attested it: `--via <episode>` (the turn where they asked) or `--control` (a dedicated logging control; the caller's statement, which the binary cannot check). |
| `observe <id>` | Write `source_status` on a reference episode, either from `--state` or by hashing `--content-file`, or by comparing `--version`. |
| `redact <id> --fields … --reason …` | Remove fields from the closed set and write the `redacted` block. Redacting `content.ref` also removes `content.uid`, `content.version`, `content.hash` and `links`, so the tombstone no longer says where the content is, and the result names what it added. It is irreversible and never cascades, and every result says that git history still holds the text. |
| `show <id> [--children]` | Print an episode, located by the path its id derives (the index is never read). |
| `list` | Filter by occurred interval (`--since`, `--until`; a date alone is a day in your zone, see above), `--standpoint`, `--kind` and `--container`, ordered by when episodes occurred (`--by recorded` orders by id). With `--container`, the default order is the container's own: by `ordinal`, then by id, with children that have no `ordinal` last. `--ndjson` streams one episode per line instead (see [Streaming](#streaming-list---ndjson)). |
| `query <text>` | Find episodes whose `label` or inline text contains any of the query's terms, under Unicode case folding, or whose `content.uid` equals a term exactly. There is no model, cache or index (see [Recall by query](#recall-by-query)). Takes `list`'s filters, plus `--participant` and `--limit`. |
| `thread <container>` | Print the children in ordinal order, each with its standpoint, time and text. A withdrawn turn keeps its place. |
| `validate` | Check every file, the index and every segment. Exits 4 on any error. |
| `index [--check]` | Rebuild the root index from the files and write missing segments, or compare without writing. |
| `version` | Print the version, the format (`episodes/0.1`) and the date of the link-relation snapshot. `--version` does the same. |
| `serve --mcp` | Serve the workspace to an MCP client over stdio, exposing the format's reference tool set (see [MCP server](#mcp-server)). |

Every writing verb takes `--author`, `--harness` and `--model`. These fall
back to `EPISODES_AUTHOR`, `EPISODES_HARNESS` and `EPISODES_MODEL`, then to
`defaults.source` in `episodes.yaml`, and at least one of author or harness
is required. `--now` (or `EPISODES_NOW`) is for tests only. It fixes the
recording timestamp, the minting instant and any `occurred` the verb defaults
to now.

`--occurred` and `--end` take an RFC 9557 value, which is put in the form a
writer writes, with a warning naming anything dropped:

- an elective suffix other than the zone (`[u-ca=gregory]`) is dropped;
- a zone whose offset contradicts the one given, or a zone this build has no
  rules for, is left out, and the offset is written alone;
- a critical flag (`[!…]`) is honoured: on the zone, the value is written
  without the flag when the offset agrees, and refused when it does not or
  the zone is unknown; on any other suffix, the value is refused.

`--zone`, `EPISODES_ZONE` and `defaults.zone` apply only to a time the verb
takes from the clock, never to a value you supply.

`EPISODES_CACHE_DIR` overrides where derived caches are kept (see below).

## MCP server

`episodes serve --mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io)
server over stdio, bound to one workspace. It exposes the Episodes Format's
[Reference Tool Set](https://github.com/nodelogicau/episodes#reference-tool-set):

| Group | Tools |
|---|---|
| Record | `episode_record_inline`, `episode_record_ref`, `act_attest`, `episode_observe`, `episode_redact` (and `conversation_start`, `conversation_turn` with `--conversation-tools`) |
| Recall | `episode_show`, `episode_list`, `episode_query`, `episode_thread` |
| Workspace | `validate`, `workspace_status` |

Each tool runs its verb in-process. Its parameters are the verb's flags in
snake case, with repeatable flags as lists and `links` as `{rel, href}`, and
its result is the verb's `--json` output. A refusal is the verb's error code
and message.

- **The author, harness and clock are bound.** No tool takes `author`,
  `harness` or `now`, and a call that passes one is refused. The author
  comes from `--author`, then `EPISODES_AUTHOR`, then
  `defaults.source.author`. The harness comes from `--harness`, then
  `EPISODES_HARNESS`, then `defaults.source.harness`, then the client's
  name. `EPISODES_NOW` is ignored. A writing tool may pass `model`.
- **`act_attest` requires `via`**, naming the bound author's episode in
  which they asked, and has no `control` or `standpoint`.
- **Conversation tools are off by default.** A harness should record turns
  from its own transcript, through a hook that runs `episodes conversation`,
  not by a model re-typing them.
- **`hash_file` and `content_file`** are read by the server, relative to the
  workspace root.
- **Resources** `episode://{id}` and `episode://{id}/thread` read as `show`
  and `thread`. Subscribers are notified after `observe` or `redact` changes
  one.

Calls run one at a time. A client configuration looks like this:

```json
{
  "mcpServers": {
    "episodes": {
      "command": "episodes",
      "args": ["serve", "--mcp", "--workspace", "/path/to/workspace", "--author", "https://example.com/ada"]
    }
  }
}
```

### Claude Desktop

Each release also has `episodes-<version>.mcpb`, a Claude Desktop extension
that bundles the server for macOS (a universal binary) and Windows. Open
the file with Claude Desktop, choose the workspace folder, and optionally
give your author URI. Without one, `defaults.source.author` in
`episodes.yaml` is used, if set. The harness is recorded as Claude
Desktop's client name. The extension lists the eleven default tools; the
conversation tools are not enabled. Build one locally with `make bundle`.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | runtime error, including a workspace whose `format` this release does not implement |
| 2 | usage error, or a write a format rule refuses (the message names the rule), including `duplicate` |
| 3 | not found: an id, container, `via` or link target |
| 4 | check failed: `validate` found errors, or `index` / `index --check` found drift or a late merge |
| 5 | no workspace found |

With `--json`, a success writes exactly one object to stdout, and a failure
writes `{"error": {"code", "message"}}` to stderr and nothing to stdout.

## Bulk import

`episodes import [<file>|-]` creates episodes from NDJSON, one JSON object
per line, for deterministic connectors that write mail, calendar and file
traces at volume. A line holds the episode's own fields, with the same names
and shapes as the YAML file: `kind`, `standpoint`, `occurred`,
`participants`, `place`, `label`, `container`, `ordinal`, `links` and
`content`.

```json
{"@key": "m1", "kind": "email", "standpoint": "https://example.com/jane", "occurred": {"start": "2026-11-07T14:24:00+11:00", "basis": "stated"}, "label": "Invoice", "content": {"mode": "ref", "ref": "gmail:18c2", "uid": "mid:18c2@example.com", "media_type": "message/rfc822"}}
{"kind": "file", "standpoint": "https://example.com/jane", "container": "@m1", "ordinal": "auto", "occurred": {"start": "2026-11-07T14:24:00+11:00", "basis": "stated"}, "content": {"mode": "ref", "ref": "gmail:18c2/att/1", "media_type": "application/pdf"}}
```

- The binary mints each `id`, in line order, and supplies `type`, `source`
  (from `--author`, `--harness` and `--model`, their environment variables
  or `defaults.source`, once for the batch) and one `timestamp` for the
  batch. A line that carries `id`, `type`, `source`, `timestamp`,
  `source_status`, `redacted`, or any key the format doesn't define, is
  refused. Import is not a copy tool.
- Every rule `record` applies to one episode applies to each line, and
  what `record` defaults, import defaults too: an inline line's `media_type`
  and `derivation`, and its time. An inline line with no `occurred`, or with
  only `{"basis": "recorded"}`, happened now, written in `--zone`,
  `EPISODES_ZONE` or `defaults.zone`. A reference line always gives
  `occurred` with its `basis`.
- `"@key": "<name>"` names a line within the batch. A later line can give
  `"@<name>"` as its `container` or as a link `href`. A key defined twice,
  never defined, or defined only on a later line fails the line that uses
  it.
- `"ordinal": "auto"` is one more than the highest ordinal the container
  holds, counting the workspace and earlier lines of the batch. It is
  worked out under the lock.
- A self-attested act (`content.mode: attested`) is refused: record each
  one with `episodes act`. A container (`content.mode: none`) must be named
  as `container` by a later line in the batch.

Every line is checked before any line is written:

| Flag | Values |
|---|---|
| `--on-error` | `stop` (default): if any line fails, nothing is written, and every failing line is reported. `skip`: the lines that pass are written, and a line naming a failed line fails with `dependency_failed`. |
| `--on-duplicate` | For a repeated `(standpoint, media_type, uid)`, in the workspace or earlier in the batch. `refuse` (default) fails the line with `duplicate`. `skip` reports it `skipped` with the existing id, which makes re-running a connector safe. A later line naming its `@key` gets the existing episode. `allow` writes it. |

Lines are written in chunks of 1,000. Each chunk holds the workspace lock
once, re-checks its lines against a fresh listing (another writer may have
taken an ordinal or recorded a uid since the check), and updates the index
once, so a capture hook waits for at most one chunk. A chunk that has been
written stays written if a later one fails. Re-running with
`--on-duplicate skip` resumes the batch, for lines that have a `uid`.

The result names every line's outcome, in input order: `written` with its
`id` and `path`, `skipped` with the existing `id`, or `failed` with a `code`
and `message`. It also gives the counts. `--json` writes one object,
`{"written", "skipped", "failed", "lines": [...]}`. `--ndjson` writes one
object per line and then a summary. The exit status is 0 when no line
failed, 2 when any line did, and 1 on an I/O failure, after the result.

Every decoded line is held in memory until the check is done, which suits
batches of about 10,000 lines. Split far larger ones.

## Recall by query

`episodes query <text>` follows the format's model-free baseline, so two
implementations give the same results in the same order:

- Whitespace (Unicode `White_Space`) is trimmed from the query's edges, and
  its terms are its runs of non-whitespace. Leading and trailing punctuation
  (Unicode category P) is stripped from each term and from the whole query.
  Punctuation inside a term stays, so `timeout?` becomes `timeout` while
  `e-mail`, `o'brien` and `auth/session` are kept whole. A query left empty,
  such as `?!`, finds nothing and is not an error.
- An episode matches when the whole query, or any one term, occurs in its
  `label` or, for an inline episode, its `content.text`, under full Unicode
  case folding (`straße` matches `STRASSE`). The content of a reference
  episode is not searched, and nor are `participants`. There is no stemming
  and there are no stopwords, so `dog's` does not find "the dog barked".
- A term that equals an episode's `content.uid`, before or after trimming,
  exactly and with no folding, also matches it, so `mid:abc@example.com`
  finds its mail.

Each result carries `resolution` (`uid` when a term equalled the uid, else
`lexical`) and `matched` (how many distinct terms matched, compared under
case folding). Results are ordered by resolution, then by `matched`, most
first, then by id, newest first. `--limit` cuts the ordered list. A container
is a result only when it matched itself, not through its children. Each
result carries its `container`, so results can be grouped by conversation.

```sh
episodes query "invoice ada" --since 2026-11-01 --limit 5
episodes query "mid:abc@example.com" --json
episodes query invoice --participant mailto:alex@example.org
```

`--since`, `--until`, `--tz`, `--standpoint`, `--kind` and `--container`
mean what they mean for `list`. `--participant` keeps episodes whose `participants`
include the reference exactly as written: `mailto:Alex@example.org` does not
match `mailto:alex@example.org`. A withdrawn episode is never returned, and a
redacted field never matches. Nothing derived from content is kept anywhere,
so there is nothing for `redact` to clear. Every file the filters leave is
read, so a query over 20,000 episodes takes about a second. Narrowing it by
standpoint, kind or time cuts that in proportion.

## Streaming (`list --ndjson`)

`list --ndjson` writes one JSON object per episode per line, for selectors
and consolidators reading a large workspace. It can't be combined with
`--json`. Its default order is `--by recorded` (id order), which streams.
`--by occurred` and `--by ordinal` hold the matching index entries, though
not the episodes, until they are sorted.

- `--fields full` (default) writes whole episodes. Each file is parsed and
  written as it is reached, and withdrawn episodes are left out unless
  `--include-withdrawn` is given.
- `--fields index` writes only the seven indexed fields (`id`, `kind`,
  `occurred`, `container`, `ordinal`, `standpoint`, `timestamp`) and `path`.
  It opens no episode file the index already covers, and parses only the
  files the index lacks, so it stays correct when the index lags. **It lists
  withdrawn episodes too.** The index can't tell them apart, since it holds
  neither `content.mode` nor `redacted`. Use `--fields full` when withdrawn
  episodes must be left out.

## Workspace discovery

The workspace is the first of these that applies:

1. `--workspace <dir>`;
2. `EPISODES_WORKSPACE`, which is strict: if it names no workspace, the verb exits 5 rather than searching further;
3. the nearest ancestor of the current directory holding `episodes.yaml`, or
   a `.episodes` pointer file whose first line names the workspace directory.
   This lets a project repository point at a memory workspace kept elsewhere.

## What immutability can and cannot be checked

The store enforces accumulation. `observe` and `redact` both go through one
function, and it refuses any rewrite that changes anything other than
`source_status`, `redacted`, and the fields the redaction removes. That
protects against this tool. A person or another tool editing files by hand
is a different matter:

- **Indexed fields in a sealed month are verifiable.** These are `id`,
  `kind`, `occurred`, `container`, `ordinal`, `standpoint` and `timestamp`. A
  sealed segment never changes, so a file that disagrees with its entry has
  been revised, and `validate` reports `revised`.
- **Indexed fields in an open month are verifiable only until the next
  `index`.** `validate` reports the disagreement, but rebuilding the root
  rewrites the entry from the file and the evidence is gone.
- **Unindexed fields are protected only by review.** That covers
  `content.text`, `label`, `participants`, `place` and `links`. In git
  terms: review the pull request.

Redaction removes text from the current file and from what this tool reads
and recalls. It does not remove it from git history or from existing clones.

## Which bytes `hash` covers

`episodes` hashes whatever bytes you give it, so which bytes to give is up
to you. The format fixes them by `media_type`, so that two writers recording
the same content write the same hash:

| `media_type` | Bytes to give `--hash-file` (or to hash for `--hash`) |
|---|---|
| `message/rfc822` | The complete raw message, headers and body, as the mailbox stores it. |
| `text/calendar` | The iCalendar object that holds the event, as the calendar store returns it. |
| Any other | The bytes of the content in that representation. |
| None | The bytes `ref` returns. Give `--media-type` whenever you give a hash; `record ref` warns when you don't. |

If you hold only a provider's own rendering, such as an API's JSON for a
message, give no bytes and no hash, and give `--version` where the source has
one. A hash of other bytes makes every later `observe --content-file` report
the source as `changed`.

A hash identifies one stored copy. Two recipients' copies of one email hash
differently, and `uid` is the identity across them. The algorithm (`sha256`)
and the line-ending rule (CRLF becomes LF before hashing) are unchanged.

## The uid cache

`record ref --uid` (or `--message-id`) refuses a second reference episode
with the same standpoint, media type and uid. `uid` is not in the index, so
the lookup keeps a cache of it outside the workspace:
`<user cache dir>/episodes/uid/<workspace.id>-<hash>.json` (on macOS
`~/Library/Caches`, on Linux `$XDG_CACHE_HOME` or `~/.cache`), or under
`$EPISODES_CACHE_DIR` when that is set. The hash is of the workspace's
absolute path, so two clones keep separate caches.

The cache is never trusted over the files. Each lookup parses the files it
hasn't seen and drops the names that are gone, and every match is checked
against its file before a write is refused. Nothing is written in the
workspace. Deleting the cache is always safe: the next lookup rebuilds it
from the files. The one thing it can miss is a file edited by hand to gain a
matching uid, which breaks accumulation anyway, and `validate`, which never
uses the cache, reports the resulting `possible_duplicate`.

A redacted uid is deleted from the cache, not just ignored. `redact` removes
it when it removes `content.uid`. A lookup that finds a cached uid its file
no longer carries, after a redaction merged in from another clone, removes
it then. Until that lookup runs, the uid stays in this machine's cache
file. `episodes` doesn't keep any other value derived from episode content.

## Further reading

- [SPEC-FEEDBACK.md](SPEC-FEEDBACK.md): the decisions the draft forced, to be raised upstream.
- [docs/benchmarks.md](docs/benchmarks.md): write and lookup costs at 20,000 episodes.
- [CHANGELOG.md](CHANGELOG.md)

## License

MIT. The format specification is CC0.
