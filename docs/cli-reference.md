# CLI reference

Agent Config Inspector uses a command-oriented interface. Static inspection commands default to the current directory when the workspace argument is omitted.

```text
agent-config-inspector scan [workspace] [options]
agent-config-inspector explain [workspace] --provider <id> --target <path>
agent-config-inspector diff [workspace] --providers <a,b> --target <path>
agent-config-inspector pin [workspace] --output <file>
agent-config-inspector verify [workspace] --snapshot <file>
agent-config-inspector probe <provider> [--case <id>] [--execute --acknowledge-quota]
agent-config-inspector inventory skills [workspace] [options]
agent-config-inspector inventory agents [workspace] [options]
agent-config-inspector inventory mcp [workspace] [options]
agent-config-inspector providers list
agent-config-inspector providers show <id>
agent-config-inspector version
```

Supported aliases are `claude`, `codex`, `copilot`, `gemini`, and `kimi`. Use `providers list` for canonical IDs and exact adapter metadata.

## Commands

| Command | Purpose |
|---|---|
| `scan` | Resolve and compare repository instructions for selected providers and targets |
| `explain` | Resolve one provider for one target with inclusion and exclusion evidence |
| `diff` | Compare selected providers for one target |
| `pin` | Write a canonical repository-only instruction snapshot |
| `verify` | Compare current repository instruction state with a snapshot |
| `inventory skills` | Inventory repository-owned Claude and Codex Agent Skills |
| `inventory agents` | Inventory repository-owned Claude and Codex custom agents |
| `inventory mcp` | Inventory repository-owned Claude and Codex MCP declarations |
| `providers list` | List registered provider identities and aliases |
| `providers show` | Show one adapter's capability and evidence records |
| `probe` | Print a safe plan or explicitly execute a generated-fixture behavioral probe |
| `version` | Print the binary version |

## Static inspection options

`scan`, `explain`, `diff`, `pin`, and `verify` share the selection and safety options below. A command may impose a narrower provider or target count.

| Option | Meaning |
|---|---|
| `--provider <id>` | Select one provider; repeatable |
| `--providers <a,b>` | Select comma-separated providers |
| `--target <path>` | Select one workspace-relative target; repeatable |
| `--targets <a,b>` | Select comma-separated workspace-relative targets |
| `--format text\|json\|sarif` | Select report format; default `text` |
| `--fail-on error\|warning\|never` | Select the finding threshold; default `error` |
| `--max-import-depth <n>` | Bound context import hops; provider-specific caps still apply |
| `--max-source-bytes <n>` | Bound bytes read from one source; default 1 MiB |
| `--follow-workspace-symlinks` | Follow symlinks only when their resolved target remains in the workspace |
| `--include-user-context` | Opt into documented user instruction locations with opaque output |

`scan`, `explain`, and `diff` additionally accept `--workspace-label <label>` for text and JSON reports. Labels are explicit, excluded from snapshots, and rejected for SARIF. See [Workspace labels](getting-started.md#workspace-labels).

`pin` accepts `--output <path>` and defaults to `agent-config-inspector.lock.json`. `verify` accepts `--snapshot <path>` with the same default. Both commands refuse `--include-user-context`; a repository lockfile cannot represent user context.

## Inventory options

All three inventory commands accept:

| Option | Meaning |
|---|---|
| `--provider <id>` | Select Claude Code or Codex CLI; repeatable |
| `--providers <a,b>` | Select comma-separated Claude and Codex providers |
| `--target <path>` | Select a workspace-relative launch path; repeatable |
| `--targets <a,b>` | Select comma-separated launch paths |
| `--format text\|json` | Select inventory format; default `text` |
| `--fail-on error\|warning\|never` | Select the finding threshold |
| `--max-source-bytes <n>` | Bound bytes read from one inventory source; default 1 MiB |
| `--follow-workspace-symlinks` | Follow source symlinks only inside the workspace |

These commands do not accept user context and never activate a discovered resource. Their independent JSON schemas and redaction rules are documented in [Skills inventory](skills-inventory.md), [Custom agents inventory](agents-inventory.md), and [MCP inventory](mcp-inventory.md).

## Behavioral probe options

```text
agent-config-inspector probe <provider> [options]

--case root-instruction-discovery
--format text|json
--timeout 2m
--execute
--acknowledge-quota
```

The default is side-effect-free plan mode. Actual execution requires both `--execute` and `--acknowledge-quota`, may consume provider quota, and is governed by the [Behavioral probe contract](behavioral-probes.md).

## Output formats

| Format | Intended use | Important boundary |
|---|---|---|
| `text` | Human review in a terminal or CI log | Instruction contents and absolute workspace paths remain hidden |
| `json` | Automation and archival | Validate against the command's schema before consuming |
| `sarif` | GitHub code scanning and compatible consumers | Repository-relative locations only; no user or external locations |

The report, snapshot, probe, skills, agents, and MCP JSON schemas are stored in [`schemas/`](../schemas).

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Completed below the configured finding threshold |
| 1 | A finding reached `--fail-on` |
| 2 | Invalid CLI usage or configuration |
| 3 | Internal error or incomplete result |
| 4 | Unsupported provider, surface, version, or probe case |
| 5 | Safety policy refused the request |

Finding thresholds control exit code 1 only. They do not suppress usage, incomplete-result, unsupported, or safety failures.
