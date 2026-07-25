# ADR 0004: Safe redaction by default

- Status: accepted
- Date: 2026-07-24

> Recorded retroactively. This decision was made at project inception in `docs/initial-design.md`; the ADR captures it in the numbered series planned in that document.

## Context

Reports and snapshots are frequently pasted into issues, pull requests, and CI logs. Instruction sources can contain private repository content, and user-global configuration, credentials, absolute home paths, and machine identity must never leak through a report, a debug log, or a SARIF file. Users should get useful output without having to opt into safety.

## Decision

Make `safe` the default redaction level and apply the same policy to text, JSON, and SARIF.

- Represent the workspace root as the stable non-identifying token `<workspace>` in JSON and hide it in text; label external sources with opaque identifiers instead of real paths.
- Do not scan user-level context unless explicitly opted in, and even then never emit its raw content or real path.
- Strip credential-like strings from snippets; emit only the minimal lines a finding needs.
- Separate `RawSource` from `ReportableSource` at the type level so a formatter cannot bypass redaction.
- Require an explicit, interactively confirmed `--redaction none` (and a separate acknowledgement in non-interactive environments) before any sensitive output.

## Consequences

Default output is safe to share, and the same guarantee covers machine formats and debug logs, not just the human view. Type-level separation means new report formatters inherit redaction instead of re-implementing it. Some findings show less surrounding context than an unredacted view would, which is an accepted trade-off; users who genuinely need raw output must take an explicit, gated step.
