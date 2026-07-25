# ADR 0007: No repository command execution

- Status: accepted
- Date: 2026-07-24

> Recorded retroactively. This decision was made at project inception in `docs/initial-design.md`; the ADR captures it in the numbered series planned in that document.

## Context

The tool scans untrusted repositories. Instruction files, imports, and frontmatter are attacker-controlled and may try to induce command execution, run package-manager or Git hooks, or start MCP servers as a side effect of scanning. Users run the tool on developer machines and in CI, where any execution of repository-controlled content is a serious risk.

## Decision

The core scan never executes repository-controlled content.

- Use only Go filesystem APIs for observation; never run repository commands, package scripts, Git hooks or filters, build tools, or MCP servers.
- Observe filesystem markers instead of invoking Git subprocesses in the MVP; any future Git integration must run in a restricted mode that verifies hooks and filters are not executed.
- Do not evaluate command substitution or template expressions during parsing; inventories describe configuration without starting the servers or agents they describe.
- Confine the only third-party execution to the opt-in `probe` command, which is a separate boundary requiring explicit `--execute` and `--acknowledge-quota`, runs against a generated read-only fixture, and isolates provider state.

## Consequences

Scanning a hostile repository cannot run its code, which makes the tool safe to point at arbitrary and CI-checked-out repositories. The guarantee is testable and is asserted by no-network and read-only-workspace integration tests. The cost is that behaviors requiring real execution are unavailable in the default path and are only ever reachable through the explicitly gated probe boundary.
