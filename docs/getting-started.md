# Getting started

This guide builds Agent Config Inspector from source, runs a first repository scan, and explains the main fields in the result. For every available command and option, see the [CLI reference](cli-reference.md).

## Prerequisites

- Go 1.25 or newer when building from source.
- A local repository to inspect.
- No provider CLI, API credential, model request, or network connection for static scans.

The core currently has no third-party Go module dependencies. Prebuilt Linux and macOS archives and their SHA-256 checksums are published on the [Releases page](https://github.com/east-true/agent-config-inspector/releases/latest).

## Build from source

```bash
git clone https://github.com/east-true/agent-config-inspector.git
cd agent-config-inspector
go build -o bin/agent-config-inspector ./cmd/agent-config-inspector
```

Confirm the binary is available:

```bash
./bin/agent-config-inspector version
```

## Run a first scan

Scan all supported providers at the repository root:

```bash
./bin/agent-config-inspector scan /path/to/repository
```

The workspace argument defaults to the current directory, so this is equivalent when run inside the repository:

```bash
./bin/agent-config-inspector scan .
```

Select providers and a workspace-relative target for a focused result:

```bash
./bin/agent-config-inspector scan . \
  --providers claude,codex \
  --target backend/src/users.go
```

`--provider` and `--target` are repeatable. Their plural forms accept comma-separated values. Provider aliases and canonical IDs are both accepted.

## Read the result

Text output uses human-readable labels while JSON retains stable field names and enum values:

| Text label | JSON field | Meaning |
|---|---|---|
| `Result` | `prediction` | Whether applicable instructions were found |
| `Analysis state` | `state` | Shown in text only when resolution was not complete |
| `Instructions` | `included_sources` | Sources predicted to contribute, in resolution order |
| `Not applied` | `excluded_sources` | Discovered sources that did not contribute, with a reason |
| `Fingerprint` | `effective_digest` | A domain-separated SHA-256 fingerprint without instruction plaintext |
| `Estimated tokens` | `token_estimate` | A deterministic byte-based estimate, not a provider tokenizer result |
| `Findings` | `findings` | Drift, ambiguity, limits, runtime assumptions, and safety conditions |

Text groups all selected agents under each target, translates predictions into natural language, and places the reason directly below each source. Canonical provider IDs remain visible for precise issue reports. Use JSON when exact enum values or complete structured data are required.

```bash
./bin/agent-config-inspector scan . --format json
```

JSON preserves the complete report structure, including adapter evidence and full digests. The same safe-output contract still applies: instruction plaintext, absolute workspace paths, and sensitive user context are not exposed.

Comparisons describe normalized instruction-unit equality without sending content to an LLM. Different wording is reported conservatively; the scanner does not guess semantic equivalence.

### Empty results

`No applicable instructions` (`predicted-empty` in JSON) means that no supported instruction source applied to the selected provider and target. It does not mean the workspace was unreadable or that the repository contains no useful context.

For an empty result, text output lists the provider-specific instruction families it checked and provides short next steps. General README content, source files, and application-specific runtime or state directories are outside the instruction scan unless a documented adapter contract explicitly includes them.

### Workspace labels

Reports never infer or print the workspace directory name. Add an explicit label when several text or JSON reports need to be distinguishable:

```bash
./bin/agent-config-inspector scan . --workspace-label checkout-service
```

Labels are trimmed, limited to 80 UTF-8 bytes, and reject path separators and Unicode control or format characters. They are excluded from snapshots so a label cannot change lockfile identity. SARIF rejects workspace labels because that format has no equivalent safe field.

Without a label, text reports say that the workspace is hidden and do not print the internal placeholder. JSON uses the stable `<workspace>` placeholder required by the report schema. Each Provider block also states whether user context was included and reminds the reader that runtime behavior and model compliance were not observed.

## Explain one provider

Use `explain` to focus on one provider and target:

```bash
./bin/agent-config-inspector explain . \
  --provider codex \
  --target backend/src/users.go
```

The output uses the same static resolver as `scan` but omits unrelated providers.

## Compare providers

Use `diff` to focus on the relationship between selected providers:

```bash
./bin/agent-config-inspector diff . \
  --providers claude,codex \
  --target backend/src/users.go
```

Select JSON when another tool will consume the report:

```bash
./bin/agent-config-inspector diff . \
  --providers claude,codex \
  --target backend/src/users.go \
  --format json
```

Report JSON conforms to [`schemas/report.schema.json`](../schemas/report.schema.json).

## Choose a failure threshold

Warnings do not fail a command by default. Use a stricter threshold for automation:

```bash
./bin/agent-config-inspector scan . --fail-on warning
```

Use `--fail-on never` for an informational run that should complete successfully regardless of findings. Invalid usage, incomplete results, unsupported requests, and safety refusals retain their dedicated nonzero exit codes.

## Next steps

- Add pull-request drift detection with [CI integration](ci-integration.md).
- Check exact adapter coverage in the [support matrix](support-matrix.md).
- Inventory repository configuration with the [skills](skills-inventory.md), [custom agents](agents-inventory.md), and [MCP](mcp-inventory.md) contracts.
- Review [Privacy](privacy.md) before opting into user-level context.
- Review [Behavioral probes](behavioral-probes.md) before starting any provider process.
