# Security policy

## Supported versions

This repository is in developer preview. Security fixes are applied to the latest commit on the default branch until tagged releases begin.

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
- bound individual source size and Claude import depth;
- omit instruction contents and absolute workspace paths from reports;
- exclude user-level instructions unless explicitly requested.

`--include-user-context` expands read authority to documented user instruction locations. Those sources remain opaque in output, but the report can still reveal that user context exists and affects a result. Treat such reports as local artifacts unless reviewed.

Static analysis cannot guarantee that an agent will obey an instruction. Instruction files are behavioral context, not an enforcement boundary.
