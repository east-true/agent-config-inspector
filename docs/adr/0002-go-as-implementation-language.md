# ADR 0002: Go as implementation language

- Status: accepted
- Date: 2026-07-24

> Recorded retroactively. This decision was made at project inception in `docs/initial-design.md`; the ADR captures it in the numbered series planned in that document.

## Context

The tool must ship as an easy-to-run CLI for local developer machines and CI, cross-compile to Linux and macOS (Windows later), traverse untrusted repository trees safely, and start quickly. It also needs a low barrier for external contributors who add provider adapters and fixtures. Prior exploratory work existed in Python but was not a hard constraint on the production implementation.

## Decision

Implement the tool in Go, distributed as a single static binary.

- Prefer the standard library and avoid third-party runtime dependencies; add one only for a clear need, with a compatible license and bounded security impact.
- Migrate prior Python experiments as provider-neutral fixtures and expected results rather than by porting code.

## Consequences

Single-binary distribution and cross-compilation are straightforward, CLI startup is fast, and safe path/process handling comes from the standard library. As implemented, the project carries **zero third-party runtime dependencies**: the CLI uses the standard `flag` package, and Markdown parsing and glob matching are custom in-tree implementations (see ADR 0008). This keeps the supply-chain surface minimal and output deterministic, at the cost of maintaining parsing and glob compatibility layers ourselves instead of reusing a CommonMark or doublestar library. The dependency posture is revisited only when a concrete need outweighs that cost.
