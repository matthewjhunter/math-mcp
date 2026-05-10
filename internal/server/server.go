// Package server constructs the math-mcp MCP server with all tool surfaces
// registered. It is exposed as a separate package so tests and alternative
// transports can share the same registration logic.
package server

import (
	"github.com/matthewjhunter/math-mcp/internal/financial"
	"github.com/matthewjhunter/math-mcp/internal/mathtools"
	"github.com/matthewjhunter/math-mcp/internal/stats"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New returns a fully configured MCP server with the math, stats, and financial
// tool surfaces registered.
func New(name, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    name,
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: `math-mcp exposes accurate math and financial calculations as discrete MCP tools.

Use these tools whenever you would otherwise estimate a numeric answer from context. Each tool returns the exact computed value and echoes its inputs so you can verify what was sent.

Surfaces:
  - math_*       Standard library math (trig, log/exp, pow, gamma, etc.) over float64.
  - stats_*      Aggregates over arrays (mean, median, stddev, percentile, correlation).
  - financial_*  Time-value-of-money calculations using shopspring/decimal for full precision.

Math surface conventions (common LLM traps):
  - Trig functions take RADIANS, not degrees. Pipe through math_deg_to_rad first if you have degrees.
  - math_log is the natural logarithm (ln, base e). Use math_log10 or math_log2 for other bases.
  - math_round uses round-half-away-from-zero, NOT banker's rounding (Python's default).
  - math_mod returns a result with the sign of x (the dividend), matching Go/C; Python's % follows the divisor.

Stats surface conventions:
  - stats_variance / stats_stddev are SAMPLE estimators (n-1 denominator). Use them when data is a sample of a larger population — the typical case. Use stats_pop_variance / stats_pop_stddev only when you literally have the entire population.
  - stats_median averages the two middle values for even-length input (textbook definition).
  - stats_percentile uses linear interpolation, matching np.percentile and Excel PERCENTILE.INC. percentile is in [0, 100].
  - stats_correlation is PEARSON (linear). It will report ~0 for non-linear relationships even when y is fully determined by x.

Financial surface conventions:
  - Sign convention: money received is POSITIVE, money paid out is NEGATIVE.
  - financial_npv treats cash_flow[0] as period 0 (NOT discounted); see the tool's description for Excel porting notes.`,
	})

	mathtools.Register(s)
	stats.Register(s)
	financial.Register(s)

	return s
}
