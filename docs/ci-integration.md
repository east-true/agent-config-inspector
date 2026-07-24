# CI integration

Agent Config Inspector can pin repository-owned instruction state, verify it in pull requests, and emit SARIF without placing instruction text in CI artifacts.

## Snapshot workflow

Create the default lockfile from the current repository state:

```bash
./bin/agent-config-inspector pin .
```

Review and commit `agent-config-inspector.lock.json` with the instruction configuration it represents. The file contains repository-relative metadata, adapter identity, and domain-separated digests, never instruction plaintext or user context.

Verify the committed state later:

```bash
./bin/agent-config-inspector verify .
```

Use a stricter finding threshold when repository instruction warnings should fail CI:

```bash
./bin/agent-config-inspector verify . --fail-on warning
```

When an intentional instruction or adapter change modifies the prediction, run `pin` again, review the lockfile diff, and commit the updated snapshot with the change. See the [snapshot format](snapshot-format.md) for canonicalization, comparison, and overwrite rules.

`pin` and `verify` refuse `--include-user-context`. A commit-ready snapshot cannot encode the existence, location, content, fingerprint, size, or token estimate of user-level instructions.

## SARIF

Generate a GitHub-compatible SARIF 2.1.0 report:

```bash
./bin/agent-config-inspector verify . \
  --format sarif \
  > agent-config-inspector.sarif
```

SARIF contains repository-relative locations only. User and external source locations are omitted. Workspace labels are rejected for SARIF rather than silently discarded.

## GitHub Action

Run the composite Action after checking out the repository:

```yaml
permissions:
  contents: read
  security-events: write

steps:
  - uses: actions/checkout@v7.0.1
  - uses: east-true/agent-config-inspector@v0.7.0
    with:
      command: verify
      snapshot: agent-config-inspector.lock.json
      fail-on: warning
      version: v0.7.0
      upload-sarif: "true"
```

The Action downloads the selected GitHub Release archive, requires a matching entry in `checksums.txt`, verifies SHA-256, and extracts only the expected binary and license members before execution.

For reproducible CI, pin both the Action reference and the `version` input to an exact release tag. `version: latest` is convenient for evaluation but allows the installed adapter behavior to change independently of the workflow commit.

## Action inputs

| Input | Default | Meaning |
|---|---|---|
| `command` | `verify` | Run `verify` or `scan` |
| `workspace` | `.` | Workspace path relative to the checkout |
| `snapshot` | `agent-config-inspector.lock.json` | Repository-relative snapshot used by `verify` |
| `fail-on` | `error` | Finding threshold: `error`, `warning`, or `never` |
| `version` | `latest` | Release tag to install, or `latest` |
| `upload-sarif` | `false` | Upload the generated SARIF report to GitHub code scanning |

The `sarif-file` output contains the generated SARIF path when one is available.

## Minimal informational scan

A repository can run the Action without a committed snapshot:

```yaml
steps:
  - uses: actions/checkout@v7.0.1
  - uses: east-true/agent-config-inspector@v0.7.0
    with:
      command: scan
      fail-on: never
      version: v0.7.0
```

This reports the current prediction but does not establish a drift baseline. Use `pin` and `verify` when reviewable change detection is the objective.

## CI safety notes

- Keep the workflow token at the smallest required permission set.
- Add `security-events: write` only when uploading SARIF.
- Review reports before publishing artifacts from a private repository.
- Do not enable user context in CI or attempt to encode it in a snapshot.
- Treat static findings as instruction-discovery evidence, not proof of agent compliance.
- Update pinned releases and lockfiles intentionally, with adapter evidence review.
