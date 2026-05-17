# Contributing

Open an issue before sending a PR for substantial changes — easier to
agree on the approach up front than to rework it in review.

## Local development

```sh
task            # list tasks
task test       # go test -race -count=1 ./...
task check      # test + vet + fmt + lint + vuln
task build      # build math-mcp binary
```

`task check` must pass before a PR will be merged. Requires Go 1.25.10
or newer.

## Conventions

- Run `task fmt:fix` before committing.
- **One tool per function.** Discoverability beats compactness; do not
  reintroduce an expression evaluator or a multi-op wrapper.
- **Echo inputs back in every response.** Defends against silent
  argument-order mistakes (PV/FV swap and similar).
- **Domain errors return structured MCP errors, not silent NaN.** Include
  the offending input value in the error message (`"sqrt: input must be
  non-negative (got -42.5)"`) so the model can self-correct.
- **Decimal strings end-to-end on the financial surface.** `result_float`
  is a convenience field, not authoritative.
- **Sign convention follows numpy / Excel**: money received is positive,
  money paid out is negative.
- **NPV follows numpy_financial**: `cash_flow[0]` is period 0 and is not
  discounted. Excel's NPV is the outlier; document this in any new
  cash-flow tools.

## Testing

- Table-driven tests against textbook reference values.
- Use `mcp.NewInMemoryTransports()` to exercise the full register/dispatch
  path rather than calling handlers directly — catches schema/dispatch
  bugs the unit test won't.
- New tools must include both a happy-path case and at least one
  domain-error case.

## Adding a tool

1. Implement the tool in the appropriate package (`internal/mathtools`,
   `internal/stats`, or `internal/financial`).
2. Register it in that package's `Register(s *mcp.Server)` function.
3. Write a clear `Description` from the LLM's perspective — describe
   when to use it and what assumptions it makes. Mention any
   conventions that bite (radians vs degrees, sample vs population,
   sign conventions).
4. Add tests covering happy path + domain errors.
5. Mention any new conventions in `README.md` if a user-visible behavior
   isn't obvious from the tool description.

## Reporting issues

Bugs and feature requests: open a [GitHub issue][issues].

Security vulnerabilities: see [SECURITY.md](SECURITY.md). Don't open
public issues for security problems.

[issues]: https://github.com/matthewjhunter/math-mcp/issues
