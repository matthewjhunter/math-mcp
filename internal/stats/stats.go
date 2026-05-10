// Package stats registers MCP tools that compute aggregates over arrays of
// numbers. Implementations come from gonum/stat and gonum/floats.
package stats

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat"
)

// ArrayIn is the input shape for tools that take a single array of values.
type ArrayIn struct {
	Values []float64 `json:"values" jsonschema:"the data points to aggregate (must be non-empty)"`
}

// PercentileIn is the input for percentile queries.
type PercentileIn struct {
	Values     []float64 `json:"values" jsonschema:"the data points to aggregate (must be non-empty)"`
	Percentile float64   `json:"percentile" jsonschema:"the percentile to compute, in [0, 100]"`
}

// Result is the output shape for stats tools.
type Result struct {
	Function string    `json:"function" jsonschema:"name of the aggregate that was computed"`
	N        int       `json:"n" jsonschema:"number of input values"`
	Inputs   []float64 `json:"inputs" jsonschema:"the input array (echoed back)"`
	Result   float64   `json:"result" jsonschema:"computed aggregate value"`
}

var (
	errEmpty   = errors.New("values must be non-empty")
	errPercent = errors.New("percentile must be between 0 and 100")
)

func arrayTool(s *mcp.Server, name, desc string, fn func([]float64) float64) {
	h := func(_ context.Context, _ *mcp.CallToolRequest, in ArrayIn) (*mcp.CallToolResult, Result, error) {
		if len(in.Values) == 0 {
			return nil, Result{}, fmt.Errorf("%s: %w", name, errEmpty)
		}
		return nil, Result{
			Function: name,
			N:        len(in.Values),
			Inputs:   in.Values,
			Result:   fn(in.Values),
		}, nil
	}
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, h)
}

// Register adds all stats tools to the server.
func Register(s *mcp.Server) {
	const offload = "Use this tool to compute the aggregate accurately instead of estimating from values in context. "

	arrayTool(s, "stats_sum", offload+"Returns the sum of the values.",
		func(v []float64) float64 { return floats.Sum(v) })

	arrayTool(s, "stats_mean", offload+"Returns the arithmetic mean of the values. For most data the mean is sensitive to outliers; consider stats_median if the distribution is skewed.",
		func(v []float64) float64 { return stat.Mean(v, nil) })

	arrayTool(s, "stats_median", offload+"Returns the median (textbook definition): the middle value for odd-length input, or the average of the two middle values for even-length input. Robust to outliers.",
		median)

	arrayTool(s, "stats_min", offload+"Returns the minimum value.",
		func(v []float64) float64 { return floats.Min(v) })

	arrayTool(s, "stats_max", offload+"Returns the maximum value.",
		func(v []float64) float64 { return floats.Max(v) })

	arrayTool(s, "stats_variance", offload+
		"Returns the UNBIASED SAMPLE variance (divides by n-1). Use this when the input is a sample drawn from a larger population (the typical case). "+
		"Use stats_pop_variance only when the input IS the entire population. Returns NaN for n<2.",
		func(v []float64) float64 { return stat.Variance(v, nil) })

	arrayTool(s, "stats_pop_variance", offload+
		"Returns the POPULATION variance (divides by n). Only correct when the input is the entire population, not a sample. "+
		"If you'd describe the data as 'a sample of …', use stats_variance instead.",
		func(v []float64) float64 { return stat.PopVariance(v, nil) })

	arrayTool(s, "stats_stddev", offload+
		"Returns the UNBIASED SAMPLE standard deviation (sqrt of sample variance, n-1 denominator). Use this for the typical case where data is a sample. "+
		"Matches Excel's STDEV.S and NumPy's std(ddof=1). Returns NaN for n<2.",
		func(v []float64) float64 { return stat.StdDev(v, nil) })

	arrayTool(s, "stats_pop_stddev", offload+
		"Returns the POPULATION standard deviation (sqrt of population variance, n denominator). Only correct when the input is the entire population. "+
		"Matches Excel's STDEV.P and NumPy's std() default.",
		func(v []float64) float64 { return stat.PopStdDev(v, nil) })

	registerPercentile(s)
	registerCorrelation(s)
}

// median implements the textbook definition: middle value for odd n, average
// of the two middle values for even n. gonum's empirical Quantile at p=0.5
// returns the lower order statistic for even n, which is not what callers
// expect when they ask for "the median".
func median(v []float64) float64 {
	sorted := slices.Clone(v)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// linearPercentile implements the NumPy/Excel "linear" percentile method
// (R's type 7). For p in [0, 100] and sorted x of length n:
//
//	h = (n-1) * p/100
//	result = x[floor(h)] + (h - floor(h)) * (x[ceil(h)] - x[floor(h)])
//
// This is what callers expect when they ask for "the 75th percentile" — it
// matches np.percentile(x, 75) and Excel's PERCENTILE.INC. Gonum's empirical
// Quantile returns an order statistic instead, which disagrees on small n.
func linearPercentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	h := float64(n-1) * p / 100
	lo := int(h)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := h - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

func registerPercentile(s *mcp.Server) {
	const desc = "Use this tool to compute a percentile precisely instead of estimating. " +
		"Uses linear interpolation between order statistics (R's type 7) — matches np.percentile(x, p) " +
		"and Excel's PERCENTILE.INC. Example: 75th percentile of [1,2,3,4] = 3.25. " +
		"percentile is in [0, 100] (so 50 = median, not 0.5)."
	h := func(_ context.Context, _ *mcp.CallToolRequest, in PercentileIn) (*mcp.CallToolResult, Result, error) {
		if len(in.Values) == 0 {
			return nil, Result{}, fmt.Errorf("stats_percentile: %w", errEmpty)
		}
		if in.Percentile < 0 || in.Percentile > 100 {
			return nil, Result{}, fmt.Errorf("stats_percentile: %w (got %v)", errPercent, in.Percentile)
		}
		sorted := slices.Clone(in.Values)
		slices.Sort(sorted)
		q := linearPercentile(sorted, in.Percentile)
		return nil, Result{
			Function: "stats_percentile",
			N:        len(in.Values),
			Inputs:   in.Values,
			Result:   q,
		}, nil
	}
	mcp.AddTool(s, &mcp.Tool{Name: "stats_percentile", Description: desc}, h)
}

// CorrelationIn is the input for the Pearson correlation tool.
type CorrelationIn struct {
	X []float64 `json:"x" jsonschema:"first series; must be non-empty and same length as y"`
	Y []float64 `json:"y" jsonschema:"second series; must be non-empty and same length as x"`
}

// CorrelationOut is the output for the Pearson correlation tool.
type CorrelationOut struct {
	Function string    `json:"function"`
	N        int       `json:"n"`
	X        []float64 `json:"x"`
	Y        []float64 `json:"y"`
	Result   float64   `json:"result" jsonschema:"Pearson correlation coefficient in [-1, 1]"`
}

func registerCorrelation(s *mcp.Server) {
	const desc = "Use this tool to compute the PEARSON (linear) correlation coefficient between two equal-length series instead of eyeballing it. " +
		"Result is in [-1, 1]: 1 = perfect positive linear, -1 = perfect negative linear, 0 = no linear relationship. " +
		"NOTE: Pearson only detects LINEAR relationships — y = x^2 over a symmetric range gives r ≈ 0 even though y is fully determined by x. " +
		"For rank/ordinal data or non-linear monotonic relationships, this tool is the wrong choice."
	h := func(_ context.Context, _ *mcp.CallToolRequest, in CorrelationIn) (*mcp.CallToolResult, CorrelationOut, error) {
		if len(in.X) == 0 || len(in.Y) == 0 {
			return nil, CorrelationOut{}, fmt.Errorf("stats_correlation: %w", errEmpty)
		}
		if len(in.X) != len(in.Y) {
			return nil, CorrelationOut{}, fmt.Errorf("stats_correlation: x (len=%d) and y (len=%d) must be the same length", len(in.X), len(in.Y))
		}
		r := stat.Correlation(in.X, in.Y, nil)
		return nil, CorrelationOut{
			Function: "stats_correlation",
			N:        len(in.X),
			X:        in.X,
			Y:        in.Y,
			Result:   r,
		}, nil
	}
	mcp.AddTool(s, &mcp.Tool{Name: "stats_correlation", Description: desc}, h)
}
