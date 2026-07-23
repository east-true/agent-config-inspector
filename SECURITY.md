# Security policy

## Supported versions

This repository is in developer preview. Security fixes are applied to the latest supported tagged release and the latest commit on the default branch.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability, leaked instruction, credential, or unredacted local path. Use the repository's private **Security → Report a vulnerability** flow when available. If that flow is unavailable, contact the maintainer privately through the GitHub profile before sharing reproduction data.

Include the smallest safe reproduction possible. Replace credentials, user names, home paths, hostnames, private repository names, and instruction text with synthetic values. Do not attach a real user-context report.

Useful details include:

- affected commit or release;
- operating system and filesystem behavior;
- command-line flags, with sensitive paths replaced;
- whether a workspace escape, symlink, import, redaction, or parser boundary is involved;
- a synthetic fixture that demonstrates the issue.

## Security boundaries

The default scanner is designed to:

- perform no network requests;
- execute no repository code, hooks, provider CLIs, MCP servers, or build commands;
- read only inside the selected workspace;
- reject symlinks unless explicitly enabled, and still reject targets outside the workspace;
- bound individual source size and provider-specific Claude/Gemini/Copilot import depth, and report Kimi's soft instruction-size guidance;
- omit instruction contents and absolute workspace paths from reports;
- exclude user-level instructions unless explicitly requested.

Repository snapshots add stricter boundaries: `pin` and `verify` refuse user context, accept only workspace-relative snapshot paths, reject symlink escapes, and refuse to overwrite an existing file unless it is already a valid snapshot. Snapshot data is limited to repository-relative metadata and domain-separated SHA-256 digests.

Repository configuration inventories do not read user, managed, plugin, or session definitions and never activate the resources they describe. MCP inventory additionally hides commands, arguments, URLs, environment/header names and values, authentication and OAuth details, tool names, and approval values. Its digest covers only the public structural projection, not raw configuration or hidden values. Server names and repository-relative paths are still metadata and should be reviewed before publishing a report from a private repository.

The composite GitHub Action downloads release assets only from this repository, requires a matching entry in `checksums.txt`, verifies SHA-256 before extraction, and accepts only the expected binary and license archive members. Pin the Action reference and its `version` input to an exact release tag when reproducible CI installation matters.

`--include-user-context` expands read authority to documented user instruction locations. Those sources remain opaque in output, but the report can still reveal that user context exists and affects a result. Treat such reports as local artifacts unless reviewed.

Static analysis cannot guarantee that an agent will obey an instruction. Instruction files are behavioral context, not an enforcement boundary.

The opt-in `probe` command is a separate boundary from the default scanner. Plan mode starts no provider process. Execution requires both `--execute` and `--acknowledge-quota`, runs only against a generated read-only fixture, isolates provider state in a temporary home, passes an allowlisted environment, and does not emit or retain the provider response. It still executes an installed third-party binary and permits provider-required network access. Verify the binary and avoid execution on machines where managed hooks, wrappers, or system policy are not understood.

Never report a probe issue with a real API key or raw provider response. Include only the sanitized probe result, detected version, fixture digest, operating system, and the smallest synthetic reproduction.
