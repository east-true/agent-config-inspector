# ADR 0006: Snapshot canonicalization

- Status: accepted
- Date: 2026-07-24

## Context

The scan report contains transient findings, presentation fields, token estimates, and optionally redacted user context. Serializing that report as a repository lockfile could leak private-state existence and would make harmless presentation changes appear as repository drift.

The snapshot must remain commit-safe, deterministic, reviewable, and able to detect both repository instruction changes and adapter-semantics changes.

## Decision

Use a dedicated versioned lockfile model named `agent-config-inspector.lock.json`.

- Reconstruct entries from repository-origin sources only.
- Recompute prediction and effective graph digests after filtering, rather than copying report-level values.
- Retain included-source order because it encodes provider precedence.
- Sort request identities, entries, and excluded sources where order has no semantic meaning.
- Omit timestamps, absolute roots, content, user labels, findings, token estimates, and external state.
- Calculate a SHA-256 lock digest over compact canonical JSON with an explicit domain separator.
- Treat tool and adapter identity changes as verification drift.
- Reject user-context options for `pin` and `verify` even though the builder independently filters user sources.
- Write snapshots atomically and refuse absolute, escaping, or symlink output paths.

## Consequences

Lockfiles are reproducible and cannot encode private user-context existence through their public structure or effective digest. Tool upgrades may require an intentional repin even when repository files are unchanged. This is desirable because changed resolver semantics can change the prediction without a source edit.

The snapshot is not a semantic proof: differently worded but equivalent instructions still produce drift.
