# ADR 0005: Static prediction versus behavioral evidence

- Status: accepted
- Date: 2026-07-24

> Recorded retroactively. This decision was made at project inception in `docs/initial-design.md`; the ADR captures it in the numbered series planned in that document.

## Context

"What an agent actually sees" spans three layers: a static prediction from documented discovery and precedence rules, a behavioral confirmation from running an installed CLI against a marker fixture, and the model's actual compliance during a task. Conflating them would let the product overclaim — a passing static resolver does not prove a model received or followed an instruction, and confirming one discovery case does not confirm unmeasured precedence rules.

## Decision

Separate the layers explicitly and keep them in distinct fields and terms.

- Label default resolver output `predicted-effective`; it never asserts model compliance.
- Keep behavioral probes opt-in, isolated from the core scan path, and confirmed per measured case only; the scan must work fully without any probe dependency or installed provider CLI.
- Treat model compliance (`observed-compliance`) as out of scope for the MVP.
- Record adapter evidence with a kind (`official-document`, `behavioral-probe`, `source-code`, `inference`) and a checked date; rules without a behavioral fixture are `documented`, not `verified`.

## Consequences

Users can trust exactly what each result claims, and marketing cannot inflate "predicts" into "observes." Confidence (`verified`/`documented`/`best-effort`) is tracked as a separate axis from capability depth (`baseline`/`full`), so a `full + documented` adapter is representable. The cost is more fields and terminology than a single "the agent sees X" answer, which is the deliberate price of not overclaiming.
