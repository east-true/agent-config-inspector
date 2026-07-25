# ADR 0008: Glob compatibility strategy

- Status: accepted
- Date: 2026-07-24

> Recorded retroactively. This decision was recorded in the numbered series planned in `docs/initial-design.md` and reflects the implementation as built. It supersedes the exploratory "wrap a single glob library" option left open in the initial design's undecided list.

## Context

Path-specific instruction scope (for example Copilot's `applyTo`) depends on glob matching, and providers do not share identical glob dialects. The initial design left open whether a single third-party library (such as doublestar) could cover every provider dialect. ADR 0002 commits the project to avoiding third-party runtime dependencies, and glob patterns from a repository are untrusted input that must match in bounded, linear time without pathological backtracking.

## Decision

Implement glob matching in-tree with no third-party dependency.

- Compile each pattern to a regular expression via a dedicated in-tree matcher (`internal/parser`), normalizing separators to `/` and operating on workspace-relative logical paths.
- Keep matching deterministic and bounded; use only linear-time regular expressions on untrusted patterns.
- Handle provider-specific dialect differences within the relevant adapter and its fixtures rather than assuming one shared external library covers every provider.

## Consequences

Glob behavior stays under direct review, matches the zero-dependency posture, and is validated per provider with golden and hostile fixtures. Each new provider dialect must be modeled and fixtured explicitly instead of inherited from a general-purpose library, which is the main maintenance cost. Because matching is in-tree, its resource bounds and determinism are covered by the project's own fuzz and hostile-input tests.
