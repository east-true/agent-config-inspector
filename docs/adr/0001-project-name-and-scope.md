# ADR 0001: Project name and scope

- Status: accepted
- Date: 2026-07-24

> Recorded retroactively. This decision was made at project inception in `docs/initial-design.md`; the ADR captures it in the numbered series planned in that document.

## Context

Repositories carry several agent configuration surfaces (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, rule directories, path-scoped instructions). Each coding agent discovers, scopes, imports, and orders them differently, so agents working on the same file can receive different guidance. The market already has tools that generate or lint these files; what is missing is a common way to see, for a specific target, which instruction sources a given provider is predicted to receive and why.

A name and a scope boundary were needed before implementation so the product would not drift into building yet another agent, prompt generator, or orchestrator.

## Decision

Name the product **Agent Config Inspector**, with repository and binary name `agent-config-inspector`.

Scope the product as a static, provider-aware inspector of existing agent configuration:

- Compute and compare the instruction sources each provider is predicted to receive for a target, with provenance.
- Read and explain origin, scope, and precedence; never generate, rewrite, or execute configuration.
- Keep the name valid as scope expands from instruction discovery to skills, agents, and MCP inventories.

Explicit non-goals: authoring `AGENTS.md`/`CLAUDE.md`, full semantic analysis of natural-language instructions, a single quality score, judging model compliance, and running repository code, hooks, or MCP servers.

## Consequences

Every feature is measured against "does this help a user see what an agent is predicted to receive, and why?" Configuration generation and quality scoring stay out of the core product. The `inspector` framing also communicates the safety boundary — the tool reads and explains, it does not produce or run configuration — which anchors the redaction, no-execution, and static-prediction decisions recorded in later ADRs.
