package stats_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/matthewjhunter/math-mcp/internal/stats"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newSession(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{Name: "stats-test", Version: "test"}, nil)
	stats.Register(server)

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return ctx, cs
}

func call(t *testing.T, ctx context.Context, sess *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res
}

func decode[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestArrayAggregates(t *testing.T) {
	ctx, sess := newSession(t)

	cases := []struct {
		tool string
		in   []float64
		want float64
	}{
		{"stats_sum", []float64{1, 2, 3, 4, 5}, 15},
		{"stats_mean", []float64{1, 2, 3, 4, 5}, 3},
		{"stats_median", []float64{1, 2, 3, 4, 5}, 3},
		{"stats_median", []float64{5, 1, 4, 2, 3}, 3}, // unsorted input
		{"stats_min", []float64{3, 1, 4, 1, 5, 9, 2, 6}, 1},
		{"stats_max", []float64{3, 1, 4, 1, 5, 9, 2, 6}, 9},
		// Sample variance of {2,4,4,4,5,5,7,9}: sum of squared deviations = 32, /(n-1)=7 => 32/7 ≈ 4.5714
		{"stats_variance", []float64{2, 4, 4, 4, 5, 5, 7, 9}, 32.0 / 7.0},
		// Population variance of same set: 32/8 = 4.0
		{"stats_pop_variance", []float64{2, 4, 4, 4, 5, 5, 7, 9}, 4.0},
		// Population stddev = 2.0
		{"stats_pop_stddev", []float64{2, 4, 4, 4, 5, 5, 7, 9}, 2.0},
		// Sample stddev = sqrt(32/7) ≈ 2.13809
		{"stats_stddev", []float64{2, 4, 4, 4, 5, 5, 7, 9}, math.Sqrt(32.0 / 7.0)},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			res := call(t, ctx, sess, tc.tool, map[string]any{"values": tc.in})
			out := decode[stats.Result](t, res)
			if !almostEqual(out.Result, tc.want, 1e-9) {
				t.Errorf("%s(%v) = %v, want %v", tc.tool, tc.in, out.Result, tc.want)
			}
			if out.N != len(tc.in) {
				t.Errorf("N: got %d want %d", out.N, len(tc.in))
			}
		})
	}
}

// TestMedianEvenLength: textbook median of even-length input averages the two
// middle values. gonum's Empirical Quantile would return the lower one (2),
// which is the wrong answer for a tool labeled "median".
func TestMedianEvenLength(t *testing.T) {
	ctx, sess := newSession(t)

	cases := []struct {
		in   []float64
		want float64
	}{
		{[]float64{1, 2, 3, 4}, 2.5},
		{[]float64{4, 2, 1, 3}, 2.5}, // unsorted input
		{[]float64{10, 20}, 15},      // smallest even case
		{[]float64{-2, -1, 1, 2}, 0}, // straddles zero
		{[]float64{1, 2, 3, 4, 5, 6}, 3.5},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			res := call(t, ctx, sess, "stats_median", map[string]any{"values": tc.in})
			out := decode[stats.Result](t, res)
			if !almostEqual(out.Result, tc.want, 1e-9) {
				t.Errorf("median(%v) = %v, want %v", tc.in, out.Result, tc.want)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	ctx, sess := newSession(t)

	// All expected values cross-checked against np.percentile (NumPy's default
	// "linear" method == R type 7 == Excel PERCENTILE.INC).
	cases := []struct {
		p    float64
		in   []float64
		want float64
	}{
		{50, []float64{1, 2, 3, 4, 5}, 3},
		{0, []float64{1, 2, 3, 4, 5}, 1},
		{100, []float64{1, 2, 3, 4, 5}, 5},
		// These four would all return different values under gonum's Empirical
		// — they pin the linear-interpolation contract.
		{25, []float64{1, 2, 3, 4}, 1.75},
		{50, []float64{1, 2, 3, 4}, 2.5},
		{75, []float64{1, 2, 3, 4}, 3.25},
		{90, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}, 8.2},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			res := call(t, ctx, sess, "stats_percentile", map[string]any{
				"values":     tc.in,
				"percentile": tc.p,
			})
			out := decode[stats.Result](t, res)
			if !almostEqual(out.Result, tc.want, 1e-9) {
				t.Errorf("percentile(%v, p=%v) = %v, want %v", tc.in, tc.p, out.Result, tc.want)
			}
		})
	}

	// Out-of-range percentile, with offending value in the message.
	res := call(t, ctx, sess, "stats_percentile", map[string]any{
		"values": []float64{1, 2, 3}, "percentile": 150,
	})
	if !res.IsError {
		t.Errorf("expected error for percentile=150")
	}
}

func TestEmptyValues(t *testing.T) {
	ctx, sess := newSession(t)
	for _, tool := range []string{"stats_sum", "stats_mean", "stats_median", "stats_min", "stats_max", "stats_variance", "stats_stddev"} {
		t.Run(tool, func(t *testing.T) {
			res := call(t, ctx, sess, tool, map[string]any{"values": []float64{}})
			if !res.IsError {
				t.Errorf("expected error for empty input to %s", tool)
			}
		})
	}
}

func TestCorrelation(t *testing.T) {
	ctx, sess := newSession(t)

	// Perfect positive correlation.
	res := call(t, ctx, sess, "stats_correlation", map[string]any{
		"x": []float64{1, 2, 3, 4, 5},
		"y": []float64{2, 4, 6, 8, 10},
	})
	out := decode[stats.CorrelationOut](t, res)
	if !almostEqual(out.Result, 1.0, 1e-9) {
		t.Errorf("perfect positive: got %v want 1.0", out.Result)
	}

	// Perfect negative correlation.
	res = call(t, ctx, sess, "stats_correlation", map[string]any{
		"x": []float64{1, 2, 3, 4, 5},
		"y": []float64{10, 8, 6, 4, 2},
	})
	out = decode[stats.CorrelationOut](t, res)
	if !almostEqual(out.Result, -1.0, 1e-9) {
		t.Errorf("perfect negative: got %v want -1.0", out.Result)
	}

	// Length mismatch.
	res = call(t, ctx, sess, "stats_correlation", map[string]any{
		"x": []float64{1, 2, 3},
		"y": []float64{1, 2},
	})
	if !res.IsError {
		t.Errorf("expected error for length mismatch")
	}
}
