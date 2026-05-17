# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities privately through GitHub's
[private vulnerability reporting][gh-pvr] for this repository. Do not
open a public issue or pull request for security problems.

A maintainer will acknowledge your report and coordinate a fix and
disclosure timeline with you.

[gh-pvr]: https://github.com/matthewjhunter/math-mcp/security/advisories/new

## Supported Versions

The latest tagged release receives security fixes. Older releases may
receive backports at the maintainer's discretion.

## Scope

In scope: the `math-mcp` server (`cmd/math-mcp`) and the internal
packages (`internal/server`, `internal/mathtools`, `internal/stats`,
`internal/financial`).

Out of scope: vulnerabilities in upstream dependencies should be reported
to those projects (Go SDK, gonum, go-financial, shopspring/decimal). If
a dependency vulnerability materially affects this project, please still
let us know so we can pin or patch.
