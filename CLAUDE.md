# math-mcp — Project Notes

MCP server that exposes accurate math, statistics, and financial calculations to
LLM clients. Goal: a precise alternative to inferring numeric answers.

## Layout

```
cmd/math-mcp/main.go           # entrypoint (stdio transport, signal handling)
internal/server/                # registers all surfaces on a single mcp.Server
internal/mathtools/             # math_* surface — Go stdlib math wrappers
internal/stats/                 # stats_* surface — gonum/stat aggregates
internal/financial/             # financial_* surface — razorpay/go-financial
```

## Conventions

- **One tool per function**, not a generic expression evaluator. Discoverability
  beats compactness — every available primitive is in the tool list.
- **Echo inputs back** in every tool response. This is the second-line defense
  against silent argument-order mistakes (e.g. PV/FV swap on `financial_pv`).
- **Domain errors are structured tool errors**, not silent NaN. If a math
  function rejects an input (`sqrt(-1)`, `log(0)`, `asin(2)`), the tool returns
  an MCP error result with the offending field named.
- **Decimal in / decimal out on the financial surface.** Money values are
  decimal strings end-to-end via `shopspring/decimal`. The `result_float` field
  is a convenience only; the canonical answer is `result` (string).
- **TDD.** Every surface package has table-driven tests against textbook /
  Wolfram reference values plus domain-error cases. Use the in-memory transport
  (`mcp.NewInMemoryTransports`) for end-to-end testing — it exercises the same
  registration / dispatch path as production.

## Sign convention (financial)

Money received is positive, money paid out is negative. This matches numpy and
Excel conventions and is documented in every financial tool description.

## NPV convention

`financial_npv` follows numpy_financial: `cash_flow[0]` is period 0 and is NOT
discounted. Excel's `NPV()` treats the first value as period 1 — porting from
Excel requires either including the initial outlay as `cash_flow[0]` (this
tool's convention) or computing as `initial_outlay + NPV(rate, future_flows)`.

## Not yet implemented

- `IRR` and `MIRR` — `razorpay/go-financial` does not expose these. Punt to
  later; if added, implement via Newton-Raphson over `financial_npv`.
- Fixed-precision arithmetic on the math surface — float64 only. The decimal
  path is reserved for the financial surface where pennies matter.

## Adding a new tool

1. Pick the right surface package (`mathtools`, `stats`, `financial`).
2. Add Input/Output structs with `jsonschema:"…"` field tags.
3. Add the registration in that package's `Register(s *mcp.Server)`.
4. Write a table-driven test in the same package against a known-correct
   reference value. For domain-error paths, add a case to the existing
   validation test.
5. Update the README tool table.

Tool descriptions should start with the standard offload nudge ("Use this tool
to compute … precisely instead of estimating") so the model is encouraged to
delegate rather than guess.
