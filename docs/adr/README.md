# Architecture Decision Records

This log records the foundational decisions behind Agent Config Inspector. Each record states the context, the decision, and its consequences. Records 0001–0005, 0007, and 0008 were recorded retroactively from `../initial-design.md`; 0006 was written with the snapshot implementation.

| ADR | Decision |
|---|---|
| [0001](0001-project-name-and-scope.md) | Project name and scope — a static provider-aware config inspector, not a generator |
| [0002](0002-go-as-implementation-language.md) | Go as the implementation language, single binary, zero third-party runtime dependencies |
| [0003](0003-provider-adapter-boundary.md) | Provider adapter boundary — declarative rules and pure callbacks over a shared engine |
| [0004](0004-safe-redaction-default.md) | Safe redaction by default across text, JSON, and SARIF |
| [0005](0005-static-prediction-vs-behavioral-evidence.md) | Separate static prediction, behavioral evidence, and model compliance |
| [0006](0006-snapshot-canonicalization.md) | Snapshot canonicalization and the commit-safe lockfile |
| [0007](0007-no-repository-command-execution.md) | No repository command execution in the core scan |
| [0008](0008-glob-compatibility-strategy.md) | In-tree glob matching with no third-party dependency |

New decisions continue the numbering. When an open item from the initial design's undecided list is resolved, record it here and strike it through in that document.
