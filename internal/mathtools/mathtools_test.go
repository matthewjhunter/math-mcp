package mathtools_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/matthewjhunter/math-mcp/internal/mathtools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newSession(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{Name: "math-mcp-test", Version: "test"}, nil)
	mathtools.Register(server)

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return ctx, clientSession
}

func callTool(t *testing.T, ctx context.Context, sess *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: transport error: %v", name, err)
	}
	return res
}

// extractResult pulls the structured Result out of a successful tool call.
func extractResult(t *testing.T, res *mcp.CallToolResult) mathtools.Result {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got error: %+v", res.Content)
	}
	if res.StructuredContent == nil {
		t.Fatalf("expected structured content, got nil")
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out mathtools.Result
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal Result: %v", err)
	}
	return out
}

func almostEqual(a, b, tol float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) && math.IsNaN(b)
	}
	return math.Abs(a-b) <= tol
}

func TestUnaryMathTools(t *testing.T) {
	ctx, sess := newSession(t)

	cases := []struct {
		name string
		x    float64
		want float64
	}{
		{"math_abs", -3.5, 3.5},
		{"math_ceil", 1.2, 2},
		{"math_floor", 1.8, 1},
		{"math_round", 2.5, 3},
		{"math_round", -2.5, -3},
		{"math_trunc", 1.9, 1},
		{"math_sqrt", 144, 12},
		{"math_cbrt", 27, 3},
		{"math_exp", 0, 1},
		{"math_exp", 1, math.E},
		{"math_log", math.E, 1},
		{"math_log2", 8, 3},
		{"math_log10", 1000, 3},
		{"math_sin", 0, 0},
		{"math_sin", math.Pi / 2, 1},
		{"math_cos", 0, 1},
		{"math_cos", math.Pi, -1},
		{"math_tan", 0, 0},
		{"math_asin", 1, math.Pi / 2},
		{"math_acos", 0, math.Pi / 2},
		{"math_atan", 1, math.Pi / 4},
		{"math_sinh", 0, 0},
		{"math_cosh", 0, 1},
		{"math_tanh", 0, 0},
		{"math_acosh", 1, 0},
		{"math_atanh", 0, 0},
		{"math_gamma", 5, 24}, // 4!
		{"math_erf", 0, 0},
		{"math_deg_to_rad", 180, math.Pi},
		{"math_rad_to_deg", math.Pi, 180},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callTool(t, ctx, sess, tc.name, map[string]any{"x": tc.x})
			out := extractResult(t, res)
			if out.Function != tc.name {
				t.Errorf("function name: got %q, want %q", out.Function, tc.name)
			}
			if got := out.Inputs["x"]; got != tc.x {
				t.Errorf("input echo: got %v, want %v", got, tc.x)
			}
			if !almostEqual(out.Result, tc.want, 1e-9) {
				t.Errorf("result: got %v, want %v", out.Result, tc.want)
			}
		})
	}
}

func TestBinaryMathTools(t *testing.T) {
	ctx, sess := newSession(t)

	cases := []struct {
		name string
		x, y float64
		want float64
	}{
		{"math_pow", 2, 10, 1024},
		{"math_pow", 9, 0.5, 3},
		{"math_hypot", 3, 4, 5},
		{"math_mod", 10, 3, 1},
		{"math_atan2", 1, 0, 0},           // angle of (1, 0) = 0
		{"math_atan2", 0, 1, math.Pi / 2}, // angle of (0, 1) = pi/2
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callTool(t, ctx, sess, tc.name, map[string]any{"x": tc.x, "y": tc.y})
			out := extractResult(t, res)
			if !almostEqual(out.Result, tc.want, 1e-9) {
				t.Errorf("result: got %v, want %v", out.Result, tc.want)
			}
			if out.Inputs["x"] != tc.x || out.Inputs["y"] != tc.y {
				t.Errorf("input echo: got x=%v y=%v, want x=%v y=%v",
					out.Inputs["x"], out.Inputs["y"], tc.x, tc.y)
			}
		})
	}
}

func TestDomainErrors(t *testing.T) {
	ctx, sess := newSession(t)

	cases := []struct {
		name string
		args map[string]any
	}{
		{"math_sqrt", map[string]any{"x": -1.0}},
		{"math_log", map[string]any{"x": 0.0}},
		{"math_log", map[string]any{"x": -2.0}},
		{"math_log2", map[string]any{"x": 0.0}},
		{"math_log10", map[string]any{"x": -1.0}},
		{"math_log1p", map[string]any{"x": -2.0}},
		{"math_asin", map[string]any{"x": 2.0}},
		{"math_acos", map[string]any{"x": -2.0}},
		{"math_acosh", map[string]any{"x": 0.5}},
		{"math_atanh", map[string]any{"x": 1.0}},
		{"math_atanh", map[string]any{"x": -1.0}},
		{"math_mod", map[string]any{"x": 1.0, "y": 0.0}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callTool(t, ctx, sess, tc.name, tc.args)
			if !res.IsError {
				t.Errorf("expected IsError=true for %s with args %v", tc.name, tc.args)
			}
		})
	}
}

// TestDomainErrorIncludesOffendingValue: when the LLM passes a bad argument it
// needs to see the value it sent in the error so it can self-correct without a
// round-trip just to ask "what did I send?".
func TestDomainErrorIncludesOffendingValue(t *testing.T) {
	ctx, sess := newSession(t)

	res := callTool(t, ctx, sess, "math_sqrt", map[string]any{"x": -42.5})
	if !res.IsError {
		t.Fatalf("expected error, got success: %+v", res.StructuredContent)
	}
	var combined string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			combined += tc.Text
		}
	}
	if !strings.Contains(combined, "-42.5") {
		t.Errorf("domain error should include offending value -42.5, got: %q", combined)
	}
}

func TestConstants(t *testing.T) {
	ctx, sess := newSession(t)

	res := callTool(t, ctx, sess, "math_constants", map[string]any{})
	if res.IsError {
		t.Fatalf("constants returned error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var c mathtools.ConstantsOut
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Pi != math.Pi {
		t.Errorf("pi mismatch: got %v want %v", c.Pi, math.Pi)
	}
	if c.E != math.E {
		t.Errorf("e mismatch: got %v want %v", c.E, math.E)
	}
	if c.Phi != math.Phi {
		t.Errorf("phi mismatch: got %v want %v", c.Phi, math.Phi)
	}
}
