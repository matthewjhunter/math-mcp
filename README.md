# math-mcp

An MCP server that exposes accurate math, statistics, and financial calculations
to LLM clients. The intent is to give models a precise alternative to inferring
numeric answers from context — call a tool, get the right number.

In practice, that means catching the kind of mistakes LLMs make when asked to
estimate. In one real-world test, the server identified two errors in a hand-
maintained financial planning document: a mortgage-payment comparator off by
~$90/mo (which propagated to an ~$87/mo opportunity-cost claim) and a loan
payoff term off by ~3 months because the napkin math ignored continuing
interest accrual.

Three tool surfaces, one tool per function for discoverability:

| Surface | Backend | Examples |
| --- | --- | --- |
| `math_*` | Go `math` stdlib (float64) | `math_sqrt`, `math_pow`, `math_sin`, `math_log`, `math_gamma`, `math_constants` |
| `stats_*` | `gonum.org/v1/gonum/stat` (float64) | `stats_mean`, `stats_median`, `stats_stddev`, `stats_percentile`, `stats_correlation` |
| `financial_*` | `github.com/razorpay/go-financial` (`shopspring/decimal`) | `financial_pmt`, `financial_npv`, `financial_pv`, `financial_fv`, `financial_rate`, `financial_nper` |

All tools echo their inputs back in the response so the model can verify what
was actually sent (defends against silent argument-order mistakes like swapping
PV and FV). Domain errors (sqrt of a negative, log of zero, etc.) are returned
as structured tool errors rather than silent NaN, and the error message
includes the offending value so the model can self-correct.

## Install

```sh
go install github.com/matthewjhunter/math-mcp/cmd/math-mcp@latest
```

Register with Claude Code (user scope):

```sh
claude mcp add math-mcp -s user -- math-mcp
```

## Conventions

### Math surface — `math_*`

Single-input tools take `{ "x": <float> }`. Two-input tools take
`{ "x": <float>, "y": <float> }`. `math_atan2` follows the standard
mathematical convention: passing `x=3, y=4` returns the angle of the vector
`(3, 4)` (i.e. `atan2(y, x)`).

Trig functions take **radians**, not degrees — pipe through `math_deg_to_rad`
if you have degrees. `math_log` is the natural logarithm (use `math_log10` or
`math_log2` for other bases). `math_round` uses round-half-away-from-zero
(not Python's banker's rounding). `math_mod` returns a result with the sign of
the dividend (Go/C convention; Python's `%` follows the divisor).

### Stats surface — `stats_*`

Aggregate tools take `{ "values": [<floats>] }`. `stats_percentile` adds a
`percentile` field in `[0, 100]`. `stats_correlation` takes two equal-length
series `x` and `y`. Sample variants (`stats_variance`, `stats_stddev`) use
`n-1` in the denominator; population variants (`stats_pop_variance`,
`stats_pop_stddev`) use `n`. `stats_correlation` is Pearson (linear).

### Financial surface — `financial_*`

All money inputs and outputs are passed as **decimal strings** (e.g.
`"0.0033333"`, `"200000"`, `"-1199.10"`). Decimal precision is preserved
end-to-end via `shopspring/decimal`; results include both the precise string
form and a `result_float` convenience value. Sign convention follows numpy /
Excel:

- Money received (loan principal disbursed to you, account balance you own)
  is **positive**.
- Money paid out (regular payments, deposits) is **negative**.

The optional `when` field is `"end"` (default; ordinary annuity, the standard
mortgage convention) or `"begin"` (annuity-due). The input schema advertises
these as a JSON Schema enum, so strict MCP clients can reject invalid values
before the request reaches the server.

`financial_npv` follows the **numpy_financial** convention: `cash_flow[0]` is
the period-0 cash flow and is **not** discounted; `cash_flow[i]` is divided by
`(1+rate)^i`. Excel's `NPV()` treats the first value as period 1 — see the
tool description for porting guidance.

`financial_rate` solves for the periodic rate by Newton-Raphson and returns
an error if the solver fails to converge within `max_iter` (default 100).

## Correctness notes

A few specific conventions worth knowing:

- **`stats_median` averages the two middle values for even-length input** —
  the textbook definition. `median([1,2,3,4]) = 2.5`.
- **`stats_percentile` uses linear interpolation between order statistics**
  (R type 7) — matches `np.percentile(x, p)` and Excel's `PERCENTILE.INC`.
  Example: the 75th percentile of `[1,2,3,4]` is 3.25.
- **Domain errors echo the offending input value** — `sqrt(-42.5)` returns
  `"math_sqrt: input must be non-negative (got -42.5)"`, not a bare
  "invalid input".

## Status

`go-financial` does not currently expose IRR or MIRR; they are deliberately
omitted rather than approximated. If needed they can be added later via a
direct Newton-Raphson search over `financial_npv`.

## Development

```sh
task test    # go test -race ./...
task lint    # golangci-lint run
task vuln    # govulncheck ./...
task build   # build math-mcp binary
```

CI runs all three on every push.

## License

Apache 2.0 — see `LICENSE`.
