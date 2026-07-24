# Agent Config Inspector documentation

This directory is the canonical source for Agent Config Inspector documentation. The files are versioned with the implementation so provider behavior, schemas, safety boundaries, and examples can change in the same commit as the code they describe.

Start with the repository [README](../README.md) for a short project overview.

## Get started

| Document | Purpose |
|---|---|
| [Getting started](getting-started.md) | Build the CLI, run a first scan, select targets, and interpret results |
| [CLI reference](cli-reference.md) | Commands, common options, output formats, and exit codes |
| [CI integration](ci-integration.md) | Pin and verify snapshots, emit SARIF, and configure the GitHub Action |

## Provider behavior

| Document | Purpose |
|---|---|
| [Support matrix](support-matrix.md) | Capability-level provider support, checked versions, limits, and evidence |
| [Gemini CLI adapter](gemini-cli.md) | Gemini context discovery and conservative adapter contract |
| [Kimi Code CLI adapter](kimi-code-cli.md) | Kimi instruction discovery and conservative adapter contract |
| [Copilot CLI adapter](copilot-cli.md) | Copilot custom-instruction discovery and surface boundaries |
| [Behavioral probes](behavioral-probes.md) | Opt-in generated-fixture measurement contract and evidence registry |

Claude Code and Codex CLI rules and primary sources are currently cataloged in the [support matrix](support-matrix.md). A surface is supported only where the matrix explicitly says so; compatibility is never inferred from a similar filename or adjacent product.

## Configuration inventories

| Document | Purpose |
|---|---|
| [Agent Skills inventory](skills-inventory.md) | Repository-owned Claude and Codex skill discovery |
| [Custom agents inventory](agents-inventory.md) | Repository-owned Claude and Codex custom-agent discovery |
| [MCP inventory](mcp-inventory.md) | Repository-owned Claude and Codex MCP declaration discovery |

Inventories describe discoverable configuration without activating skills, starting agents, or contacting MCP servers. Their content-redaction boundaries are part of each contract.

## Data and safety contracts

| Document | Purpose |
|---|---|
| [Privacy](privacy.md) | Read boundaries, user-context opt-in, redaction, and safe publication guidance |
| [Limitations](limitations.md) | Static-prediction and provider-runtime boundaries |
| [Snapshot format](snapshot-format.md) | Canonical repository lockfile and drift semantics |
| [Security policy](../SECURITY.md) | Vulnerability reporting and secure operating boundaries |
| [`schemas/`](../schemas) | Machine-readable report, snapshot, probe, and inventory schemas |

## Design and contribution

| Document | Purpose |
|---|---|
| [Initial design](initial-design.md) | Architecture decisions, delivery phases, and roadmap |
| [Snapshot canonicalization ADR](adr/0006-snapshot-canonicalization.md) | Snapshot hashing and deterministic serialization decision |
| [Contributing](../CONTRIBUTING.md) | Evidence, tests, fixtures, and pull-request expectations |

Documentation changes should accompany the implementation or contract change they describe. Avoid copying canonical details into another documentation system; link to the relevant file instead.
