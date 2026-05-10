package financial_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/matthewjhunter/math-mcp/internal/financial"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newSession(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{Name: "financial-test", Version: "test"}, nil)
	financial.Register(server)

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

func decode(t *testing.T, res *mcp.CallToolResult) financial.FinResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out financial.FinResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// TestPmt: classic 30-yr fixed mortgage example.
// $200,000 loan, 6% APR (0.5% monthly), 360 months -> monthly payment ≈ -$1199.10.
func TestPmt(t *testing.T) {
	ctx, sess := newSession(t)
	res := call(t, ctx, sess, "financial_pmt", map[string]any{
		"rate": "0.005",
		"nper": 360,
		"pv":   "200000",
	})
	out := decode(t, res)
	if !almostEqual(out.ResultFloat, -1199.10, 0.05) {
		t.Errorf("PMT: got %s (%.4f), want approx -1199.10", out.Result, out.ResultFloat)
	}
	if out.Inputs["nper"] != float64(360) { // JSON numbers decode as float64
		t.Errorf("nper echo: got %v", out.Inputs["nper"])
	}
}

// TestFv: $100/month for 10 years at 6% APR (0.5%/month) = ~$16387.93.
func TestFv(t *testing.T) {
	ctx, sess := newSession(t)
	res := call(t, ctx, sess, "financial_fv", map[string]any{
		"rate": "0.005",
		"nper": 120,
		"pmt":  "-100",
	})
	out := decode(t, res)
	// Save: paying out -100 each period from a starting PV=0, FV is positive (you have it).
	if !almostEqual(out.ResultFloat, 16387.93, 1) {
		t.Errorf("FV: got %s (%.4f), want approx 16387.93", out.Result, out.ResultFloat)
	}
}

// TestPv: PV of receiving $1000/year for 5 years at 5% discount rate ≈ -$4329.48.
func TestPv(t *testing.T) {
	ctx, sess := newSession(t)
	res := call(t, ctx, sess, "financial_pv", map[string]any{
		"rate": "0.05",
		"nper": 5,
		"pmt":  "1000",
	})
	out := decode(t, res)
	if !almostEqual(out.ResultFloat, -4329.48, 0.05) {
		t.Errorf("PV: got %s (%.4f), want approx -4329.48", out.Result, out.ResultFloat)
	}
}

// TestNpv: razorpay convention treats cash_flow[0] as period 0 (no discount).
// -1000/1 + 200/1.1 + 300/1.1^2 + 400/1.1^3 + 500/1.1^4 ≈ 71.78.
func TestNpv(t *testing.T) {
	ctx, sess := newSession(t)
	res := call(t, ctx, sess, "financial_npv", map[string]any{
		"rate":      "0.10",
		"cash_flow": []string{"-1000", "200", "300", "400", "500"},
	})
	out := decode(t, res)
	if !almostEqual(out.ResultFloat, 71.78, 0.05) {
		t.Errorf("NPV: got %s (%.4f), want approx 71.78", out.Result, out.ResultFloat)
	}

	// Also confirm: same answer reached Excel-style by separating the initial outlay
	// from a 4-element future cash-flow series prefixed with a 0 period.
	res2 := call(t, ctx, sess, "financial_npv", map[string]any{
		"rate":      "0.10",
		"cash_flow": []string{"0", "200", "300", "400", "500"},
	})
	out2 := decode(t, res2)
	if !almostEqual(-1000+out2.ResultFloat, out.ResultFloat, 1e-9) {
		t.Errorf("convention reconstruction mismatch: %.6f != %.6f",
			-1000+out2.ResultFloat, out.ResultFloat)
	}
}

// TestIPmtPPmt: in any period, ipmt + ppmt should equal pmt.
func TestIPmtPPmtSum(t *testing.T) {
	ctx, sess := newSession(t)
	args := map[string]any{
		"rate": "0.005",
		"per":  1,
		"nper": 360,
		"pv":   "200000",
	}
	ipmt := decode(t, call(t, ctx, sess, "financial_ipmt", args))
	ppmt := decode(t, call(t, ctx, sess, "financial_ppmt", args))
	pmt := decode(t, call(t, ctx, sess, "financial_pmt", map[string]any{
		"rate": "0.005", "nper": 360, "pv": "200000",
	}))
	sum := ipmt.ResultFloat + ppmt.ResultFloat
	if !almostEqual(sum, pmt.ResultFloat, 1e-6) {
		t.Errorf("ipmt+ppmt should equal pmt: %v + %v = %v, want %v",
			ipmt.ResultFloat, ppmt.ResultFloat, sum, pmt.ResultFloat)
	}
}

// TestNper: how many monthly payments of $1199.10 to pay off $200k at 6% APR? ~360.
func TestNper(t *testing.T) {
	ctx, sess := newSession(t)
	res := call(t, ctx, sess, "financial_nper", map[string]any{
		"rate": "0.005",
		"pmt":  "-1199.10",
		"pv":   "200000",
	})
	out := decode(t, res)
	if !almostEqual(out.ResultFloat, 360, 1) {
		t.Errorf("Nper: got %s (%.4f), want approx 360", out.Result, out.ResultFloat)
	}
}

// TestRate: invert PMT — given the payment we just used, can we recover the rate?
func TestRate(t *testing.T) {
	ctx, sess := newSession(t)
	res := call(t, ctx, sess, "financial_rate", map[string]any{
		"pv":   "200000",
		"fv":   "0",
		"pmt":  "-1199.10",
		"nper": 360,
	})
	out := decode(t, res)
	if !almostEqual(out.ResultFloat, 0.005, 1e-5) {
		t.Errorf("Rate: got %s (%.6f), want approx 0.005", out.Result, out.ResultFloat)
	}
}

func TestInputValidation(t *testing.T) {
	ctx, sess := newSession(t)

	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"bad rate", "financial_pmt", map[string]any{"rate": "not-a-number", "nper": 12, "pv": "1000"}},
		{"zero nper", "financial_pmt", map[string]any{"rate": "0.05", "nper": 0, "pv": "1000"}},
		{"negative nper", "financial_pmt", map[string]any{"rate": "0.05", "nper": -1, "pv": "1000"}},
		{"bad when", "financial_pmt", map[string]any{"rate": "0.05", "nper": 12, "pv": "1000", "when": "midnight"}},
		{"empty cash flow", "financial_npv", map[string]any{"rate": "0.05", "cash_flow": []string{}}},
		{"per > nper", "financial_ipmt", map[string]any{"rate": "0.05", "per": 13, "nper": 12, "pv": "1000"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := call(t, ctx, sess, tc.tool, tc.args)
			if !res.IsError {
				t.Errorf("expected error for %s with %v", tc.tool, tc.args)
			}
		})
	}
}

// TestDecimalPrecision: ensure the result string preserves precision, not just float64.
func TestDecimalPrecision(t *testing.T) {
	ctx, sess := newSession(t)
	res := call(t, ctx, sess, "financial_pmt", map[string]any{
		"rate": "0.0033333333333", // 4% APR / 12, very long fraction
		"nper": 360,
		"pv":   "350000",
	})
	out := decode(t, res)
	// Spot check that result string is not just a short float; should have many sig figs.
	if len(out.Result) < 10 {
		t.Errorf("expected high-precision decimal string, got %q", out.Result)
	}
}

// TestWhenEnumInSchema: the "when" property on every relevant tool must advertise
// an enum constraint of ["end", "begin"] in its input schema, so the LLM sees the
// valid choices in the tool list rather than discovering them by trial and error.
func TestWhenEnumInSchema(t *testing.T) {
	ctx, sess := newSession(t)

	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	wantTools := map[string]bool{
		"financial_pmt": true, "financial_ipmt": true, "financial_ppmt": true,
		"financial_fv": true, "financial_pv": true,
		"financial_nper": true, "financial_rate": true,
	}

	for _, tool := range tools.Tools {
		if !wantTools[tool.Name] {
			continue
		}
		delete(wantTools, tool.Name)

		// InputSchema arrives over the wire as a parsed object; marshal/unmarshal
		// it through a generic map so we don't depend on the SDK's typed schema.
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal schema: %v", tool.Name, err)
		}
		var schema struct {
			Properties map[string]struct {
				Enum []any `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal schema: %v", tool.Name, err)
		}

		when, ok := schema.Properties["when"]
		if !ok {
			t.Errorf("%s: input schema missing \"when\" property", tool.Name)
			continue
		}
		if len(when.Enum) != 2 {
			t.Errorf("%s: when.enum length = %d, want 2; got %v", tool.Name, len(when.Enum), when.Enum)
			continue
		}
		got := map[string]bool{}
		for _, v := range when.Enum {
			if s, ok := v.(string); ok {
				got[s] = true
			}
		}
		if !got["end"] || !got["begin"] {
			t.Errorf("%s: when.enum = %v, want [\"end\", \"begin\"]", tool.Name, when.Enum)
		}
	}

	for name := range wantTools {
		t.Errorf("expected tool %q not found in ListTools", name)
	}
}

// TestBeginVsEnd: payment-due timing affects PMT magnitude.
func TestBeginVsEnd(t *testing.T) {
	ctx, sess := newSession(t)
	endRes := decode(t, call(t, ctx, sess, "financial_pmt", map[string]any{
		"rate": "0.005", "nper": 360, "pv": "200000",
	}))
	beginRes := decode(t, call(t, ctx, sess, "financial_pmt", map[string]any{
		"rate": "0.005", "nper": 360, "pv": "200000", "when": "begin",
	}))
	// PMT at beginning of period is smaller in magnitude (less interest accrued).
	if math.Abs(beginRes.ResultFloat) >= math.Abs(endRes.ResultFloat) {
		t.Errorf("expected begin PMT (%v) magnitude < end PMT (%v) magnitude",
			beginRes.ResultFloat, endRes.ResultFloat)
	}
}
