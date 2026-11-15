## ADDED Requirements

### Requirement: Duplicate lookup may use a cache outside the workspace
The duplicate lookup of *Duplicate references refused unless allowed* MAY read candidates from a derived cache. If it does, the cache SHALL NOT be written inside the workspace. It SHALL live under `$EPISODES_CACHE_DIR` when that is set, otherwise under the user cache directory, and SHALL be keyed by `workspace.id` together with the workspace's absolute path. Before each lookup the cache SHALL be reconciled with the directory listing: every listed file it has not seen is parsed, and every name no longer listed is dropped. Every candidate SHALL be checked against its file before it is reported. The lookup's result SHALL then equal a full parse's, except for an episode file revised in place to gain a matching `standpoint`, `media_type` or `uid`. A cache that is missing, unreadable, not decodable, or built for a different workspace SHALL be rebuilt from the files. A failure to read or write the cache SHALL NOT fail the verb. `validate` SHALL NOT use the cache.

#### Scenario: Warm cache
- **WHEN** the cache already covers 20,000 reference episodes and the same `mid:` as one of them is recorded again from the same standpoint
- **THEN** the write is refused with code `duplicate`, naming that episode, without parsing the other files

#### Scenario: A file merged in that the cache lacks
- **WHEN** a reference episode with a given `uid` arrives through a git merge after the cache was last written, and the same `uid` is then recorded from the same standpoint
- **THEN** the merged file is parsed during reconciliation and the write is refused, naming it

#### Scenario: A redacted uid
- **WHEN** the cache lists an episode under a `uid` that a later `redact` removed from its file
- **THEN** that episode is not reported as a duplicate

#### Scenario: Cold or corrupt cache
- **WHEN** the cache file is missing or does not decode
- **THEN** the lookup gives the same result as a full parse and writes a fresh cache

#### Scenario: Unwritable cache directory
- **WHEN** `EPISODES_CACHE_DIR` names a directory that cannot be created or written
- **THEN** `record ref` still succeeds or refuses exactly as a full parse would

#### Scenario: Nothing written in the workspace
- **WHEN** any `record ref` runs
- **THEN** no file is created in the workspace tree other than the episode and the index
