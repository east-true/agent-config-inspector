# Repository snapshot format

The default commit-ready snapshot is `agent-config-inspector.lock.json`. Its schema is [schemas/snapshot.schema.json](../schemas/snapshot.schema.json).

## Purpose

`pin` records the predicted repository-owned instruction graph for selected targets and providers. `verify` validates the lock digest, repeats the same scan, and emits `ACI063` when the current repository or pinned analysis metadata differs.

The lockfile is deterministic: the same repository bytes, request, tool version, and adapter registry produce byte-identical output. It has no creation timestamp or machine identity.

## Included data

- schema, tool, and adapter-registry versions;
- sorted provider and target request identities;
- provider adapter and documented version identity;
- repository-relative included and excluded source identities;
- raw and normalized repository digests;
- path scope and precedence order;
- repository-only effective graph digest;
- domain-separated lock digest.

## Excluded data

- workspace absolute path, user name, hostname, or home directory;
- user instruction existence, label, content, path, digest, size, or token estimate;
- external import content or path;
- findings, snippets, credentials, provider login state, sessions, or command output;
- timestamps and other machine-dependent values.

`pin` and `verify` reject `--include-user-context`. The lockfile type also cannot serialize scan-report source content.

## Canonicalization

Arrays that do not encode precedence are sorted. Included sources retain effective precedence order. Excluded sources sort by logical path and identity. JSON object order follows the versioned lockfile structure, strings are UTF-8 JSON strings without HTML escaping, and canonical bytes have no trailing newline.

The stored `lock_digest` is calculated after replacing that field with an empty digest:

```text
sha256("agent-config-inspector/repository-lock/v1\0" + canonical_json)
```

Repository source and effective graph digests retain their own domain separation. Any unknown field, unsupported schema version, non-canonical path, duplicate or unsorted entry, trailing JSON value, or lock-digest mismatch invalidates the snapshot before a repository scan begins.

Snapshot reads and writes are capped at 4 MiB. Writes are atomic, stay inside the selected workspace, reject symlink paths, and replace an existing file only when that file is already a valid snapshot.

## Drift behavior

`verify` rescans the providers and targets recorded in the lockfile. It does not accept provider or target overrides. Drift includes:

- an entry added or removed;
- source inclusion, exclusion, scope, ordering, or digest changes;
- effective repository graph changes;
- tool, adapter-registry, adapter identity, or documented provider-version changes.

On drift, the command emits `ACI063` and follows the configured `--fail-on` threshold. Run `pin` again only after reviewing and intentionally accepting the change.
