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

| Verb | What it does |
|---|---|
| `init [dir]` | Create `episodes.yaml`, `episodes/` and an empty `index.yaml`. `--author`, `--harness` and `--zone` go to `defaults`. |
| `workspace` | Show the resolved workspace, how it was found (`flag`, `env`, `marker`, `pointer`), and its configuration. |
| `workspace pointer [dir] [--at D] [--force]` | Write a `.episodes` pointer naming the workspace. |
| `record inline` | Record content that has no other home: a note, or a transcript the harness made. Without `--occurred`, the time is now and the basis is `recorded`. |
| `record ref` | Record a trace of content that lives elsewhere (`--ref`, `--uid` or `--message-id`, `--version`, `--hash` or `--hash-file`). `--occurred` and `--basis` are required. A repeated `(standpoint, media_type, uid)` is refused unless you pass `--allow-duplicate`. |
| `conversation start` | Write a conversation container (`content: {mode: none}`) and its first turn, under one lock. |
| `conversation turn <container>` | Append a turn. It is given the next ordinal under the lock. `--standpoint` is required on every turn. |
| `act` | Record a self-attested act: its standpoint is `source.author` and its content is `{mode: attested}`, a mode that carries no other key. When a harness records it, it must say how the person attested it: `--via <episode>` (the turn where they asked) or `--control` (a dedicated logging control; the caller's statement, which the binary cannot check). |
| `observe <id>` | Write `source_status` on a reference episode, either from `--state` or by hashing `--content-file`, or by comparing `--version`. |
| `redact <id> --fields … --reason …` | Remove fields from the closed set and write the `redacted` block. Redacting `content.ref` also removes `content.uid`, `content.version`, `content.hash` and `links`, so the tombstone no longer says where the content is, and the result names what it added. It is irreversible and never cascades, and every result says that git history still holds the text. |
| `show <id> [--children]` | Print an episode, located by the path its id derives (the index is never read). |
| `list` | Filter by occurred interval (`--since`, `--until`), `--standpoint`, `--kind` and `--container`, ordered by when episodes occurred (`--by recorded` orders by id). With `--container`, the default order is the container's own: by `ordinal`, then by id, with children that have no `ordinal` last. |
| `thread <container>` | Print the children in ordinal order, each with its standpoint, time and text. A withdrawn turn keeps its place. |
| `validate` | Check every file, the index and every segment. Exits 4 on any error. |
| `index [--check]` | Rebuild the root index from the files and write missing segments, or compare without writing. |
| `version` | Print the version, the format (`episodes/0.1`) and the date of the link-relation snapshot. |

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
