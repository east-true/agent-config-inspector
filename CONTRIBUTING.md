# Contributing

Thank you for helping make agent configuration easier to inspect.

## Before opening a change

- Use an issue for a new provider, surface, schema change, or behavior that needs design agreement.
- Do not add a provider from memory or by assuming compatibility with another product surface.
- Link official documentation or a reproducible, sanitized behavioral probe for every discovery rule.
- Keep fixtures synthetic. Never commit real user instructions, credentials, hostnames, absolute home paths, or private repository names.

## Local workflow

```bash
go test ./...
go vet ./...
go build ./cmd/agent-config-inspector
go run ./cmd/agent-config-inspector verify .
git diff --check
```

The project currently avoids third-party runtime dependencies. A new dependency should have a clear need, compatible license, and bounded security impact.

## Adapter expectations

An adapter change should include tests for applicable cases:

- root and nested discovery;
- precedence or replacement;
- imports, cycles, and depth limits;
- path-specific scope;
- empty, malformed, oversized, or unreadable sources;
- symlink and workspace escape behavior;
- deterministic output and safe redaction;
- explicit unsupported behavior for adjacent surfaces.

Provider support is capability-specific. Do not label an adapter “full” when it handles only repository instruction discovery.

Snapshot or CI-distribution changes must also preserve deterministic canonical JSON, repository-only redaction, safe workspace-relative file handling, strict archive membership, and checksum verification. Run `./scripts/build-release.sh v0.0.0-test <temporary-directory>` when changing release packaging.

Behavioral probe changes must keep plan mode side-effect free, use synthetic generated fixtures, avoid cached-login copying, pass credentials only through documented process-scoped channels, bound and discard raw output, and confirm only the exact measured case. Unit tests must use a fake provider executor; automated test suites must never make real model requests.

## Pull requests

Keep pull requests focused. Describe the observed or documented behavior, the conservative fallback, and the tests that establish it. Generated golden files must only be updated intentionally and should be reviewable in the diff.

By contributing, you agree that your contribution is licensed under Apache License 2.0.
