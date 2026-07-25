# ADR 0003: Provider adapter boundary

- Status: accepted
- Date: 2026-07-24

> Recorded retroactively. This decision was made at project inception in `docs/initial-design.md`; the ADR captures it in the numbered series planned in that document.

## Context

Each provider (Claude Code, Codex CLI, Gemini CLI, Kimi Code CLI, Copilot CLI) defines its own discovery locations, scope, import syntax, and precedence, and provider behavior changes quickly. New adapters must be addable one at a time without weakening existing adapters' accuracy or regression coverage, and adapter code must not be able to reach the filesystem or network in surprising ways.

## Decision

Confine each provider's behavior behind a narrow adapter contract, kept as declarative data plus pure callbacks, and run all filesystem traversal, parsing, comparison, and reporting in shared provider-neutral engines.

- Adapters supply identity (provider/surface/version), support level, root rules, source rules, import syntax, ordering/replacement semantics, and evidence records.
- Adapters do not perform filesystem traversal, network access, or report formatting; ordering callbacks must be pure functions.
- Each provider/surface/version has its own identity; a product family does not share one adapter across surfaces (for example Claude Code CLI is distinct from other Claude integrations).
- Adapters are compiled into the binary as reviewable, versioned source; semantics are never downloaded at runtime.

## Consequences

The shared engine is exercised across every provider, so path bugs and content bugs are tested independently of provider rules. Adding a provider is a bounded, reviewable contribution. Because adapters cannot touch the filesystem or network, the safety boundaries in ADRs 0004 and 0007 hold uniformly regardless of which adapter runs. Providers whose real behavior does not fit pure data-only rules require carefully constrained callbacks, which is the main cost of this boundary.
