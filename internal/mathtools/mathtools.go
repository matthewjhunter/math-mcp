// Package mathtools registers MCP tools that wrap the Go standard math package.
package mathtools

import (
	"context"
	"fmt"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UnaryIn is the input shape for single-argument math tools.
type UnaryIn struct {
	X float64 `json:"x" jsonschema:"input value"`
}

// BinaryIn is the input shape for two-argument math tools.
type BinaryIn struct {
	X float64 `json:"x" jsonschema:"first input"`
	Y float64 `json:"y" jsonschema:"second input"`
}

// Result is the output shape for math tools. Inputs are echoed back so the
// caller can verify that arguments were sent and interpreted correctly.
type Result struct {
	Function string             `json:"function" jsonschema:"name of the math function that was evaluated"`
	Inputs   map[string]float64 `json:"inputs" jsonschema:"the inputs the function was applied to"`
	Result   float64            `json:"result" jsonschema:"computed result as a float64"`
}

func unary(s *mcp.Server, name, desc string, fn func(float64) float64, domain func(float64) error) {
	h := func(_ context.Context, _ *mcp.CallToolRequest, in UnaryIn) (*mcp.CallToolResult, Result, error) {
		if domain != nil {
			if err := domain(in.X); err != nil {
				return nil, Result{}, fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil, Result{
			Function: name,
			Inputs:   map[string]float64{"x": in.X},
			Result:   fn(in.X),
		}, nil
	}
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, h)
}

func binary(s *mcp.Server, name, desc string, fn func(float64, float64) float64, domain func(float64, float64) error) {
	h := func(_ context.Context, _ *mcp.CallToolRequest, in BinaryIn) (*mcp.CallToolResult, Result, error) {
		if domain != nil {
			if err := domain(in.X, in.Y); err != nil {
				return nil, Result{}, fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil, Result{
			Function: name,
			Inputs:   map[string]float64{"x": in.X, "y": in.Y},
			Result:   fn(in.X, in.Y),
		}, nil
	}
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, h)
}

func mustNonNegative(x float64) error {
	if x < 0 {
		return fmt.Errorf("input must be non-negative (got %v)", x)
	}
	return nil
}

func mustPositive(x float64) error {
	if x <= 0 {
		return fmt.Errorf("input must be positive (got %v)", x)
	}
	return nil
}

func mustInUnitInterval(x float64) error {
	if x < -1 || x > 1 {
		return fmt.Errorf("input must be in [-1, 1] (got %v)", x)
	}
	return nil
}

// Register adds all math stdlib tools to the server.
//
//nolint:funlen // long, but linear: one line per registered tool.
func Register(s *mcp.Server) {
	const offload = "Use this tool to compute the value precisely instead of estimating. "

	// Basic.
	unary(s, "math_abs", offload+"Returns |x| (absolute value of x).", math.Abs, nil)
	unary(s, "math_ceil", offload+"Returns the least integer value >= x as a float64.", math.Ceil, nil)
	unary(s, "math_floor", offload+"Returns the greatest integer value <= x as a float64.", math.Floor, nil)
	unary(s, "math_round", offload+"Rounds x to the nearest integer using round-half-away-from-zero (NOT banker's rounding — differs from Python's built-in round).", math.Round, nil)
	unary(s, "math_trunc", offload+"Returns the integer value of x (truncated toward zero).", math.Trunc, nil)
	binary(s, "math_mod", offload+"Returns the floating-point remainder of x/y. The result has the sign of x (Go/C convention; differs from Python's % which follows the sign of y). y must be non-zero.",
		math.Mod, func(_, y float64) error {
			if y == 0 {
				return fmt.Errorf("y must be non-zero")
			}
			return nil
		})

	// Roots and powers.
	unary(s, "math_sqrt", offload+"Returns the square root of x. x must be non-negative.",
		math.Sqrt, mustNonNegative)
	unary(s, "math_cbrt", offload+"Returns the cube root of x.", math.Cbrt, nil)
	binary(s, "math_pow", offload+"Returns x raised to the power y (x^y).", math.Pow, nil)
	binary(s, "math_hypot", offload+"Returns sqrt(x*x + y*y), avoiding overflow/underflow.",
		math.Hypot, nil)

	// Exp/Log.
	unary(s, "math_exp", offload+"Returns e^x.", math.Exp, nil)
	unary(s, "math_exp2", offload+"Returns 2^x.", math.Exp2, nil)
	unary(s, "math_expm1", offload+"Returns e^x - 1, accurate for x near zero.", math.Expm1, nil)
	unary(s, "math_log", offload+"Returns the NATURAL logarithm (ln, base e) of x. For other bases use math_log2 or math_log10. x must be positive.",
		math.Log, mustPositive)
	unary(s, "math_log2", offload+"Returns the base-2 logarithm of x. x must be positive.",
		math.Log2, mustPositive)
	unary(s, "math_log10", offload+"Returns the base-10 logarithm of x. x must be positive.",
		math.Log10, mustPositive)
	unary(s, "math_log1p", offload+"Returns ln(1+x), accurate for x near zero. (1+x) must be positive.",
		math.Log1p, func(x float64) error {
			if 1+x <= 0 {
				return fmt.Errorf("1 + x must be positive (got x=%v, 1+x=%v)", x, 1+x)
			}
			return nil
		})

	// Trigonometric (radians).
	const radNote = " x is in RADIANS, not degrees — use math_deg_to_rad if you have degrees."
	const radOut = " Result is in radians."
	unary(s, "math_sin", offload+"Returns sin(x)."+radNote, math.Sin, nil)
	unary(s, "math_cos", offload+"Returns cos(x)."+radNote, math.Cos, nil)
	unary(s, "math_tan", offload+"Returns tan(x)."+radNote, math.Tan, nil)
	unary(s, "math_asin", offload+"Returns arcsin(x)."+radOut+" x must be in [-1, 1].",
		math.Asin, mustInUnitInterval)
	unary(s, "math_acos", offload+"Returns arccos(x)."+radOut+" x must be in [-1, 1].",
		math.Acos, mustInUnitInterval)
	unary(s, "math_atan", offload+"Returns arctan(x)."+radOut, math.Atan, nil)
	binary(s, "math_atan2", offload+"Returns the angle (in radians) of the point (x, y) measured from the positive x-axis. "+
		"Equivalent to atan2(y, x) in most languages; uses the signs of both x and y to choose the correct quadrant.",
		func(x, y float64) float64 { return math.Atan2(y, x) }, nil)

	// Hyperbolic.
	unary(s, "math_sinh", offload+"Returns the hyperbolic sine of x.", math.Sinh, nil)
	unary(s, "math_cosh", offload+"Returns the hyperbolic cosine of x.", math.Cosh, nil)
	unary(s, "math_tanh", offload+"Returns the hyperbolic tangent of x.", math.Tanh, nil)
	unary(s, "math_asinh", offload+"Returns the inverse hyperbolic sine of x.", math.Asinh, nil)
	unary(s, "math_acosh", offload+"Returns the inverse hyperbolic cosine of x. x must be >= 1.",
		math.Acosh, func(x float64) error {
			if x < 1 {
				return fmt.Errorf("input must be >= 1 (got %v)", x)
			}
			return nil
		})
	unary(s, "math_atanh", offload+"Returns the inverse hyperbolic tangent of x. x must be strictly in (-1, 1).",
		math.Atanh, func(x float64) error {
			if x <= -1 || x >= 1 {
				return fmt.Errorf("input must be strictly in (-1, 1) (got %v)", x)
			}
			return nil
		})

	// Special.
	unary(s, "math_gamma", offload+"Returns the Gamma function of x.", math.Gamma, nil)
	unary(s, "math_lgamma", offload+"Returns the natural log of |Gamma(x)|.",
		func(x float64) float64 { v, _ := math.Lgamma(x); return v }, nil)
	unary(s, "math_erf", offload+"Returns the error function of x.", math.Erf, nil)
	unary(s, "math_erfc", offload+"Returns the complementary error function of x.", math.Erfc, nil)

	// Conversion helpers.
	unary(s, "math_deg_to_rad", offload+"Converts degrees to radians.",
		func(x float64) float64 { return x * math.Pi / 180 }, nil)
	unary(s, "math_rad_to_deg", offload+"Converts radians to degrees.",
		func(x float64) float64 { return x * 180 / math.Pi }, nil)

	registerConstants(s)
}

// ConstantsOut lists the named mathematical constants exposed by math_constants.
type ConstantsOut struct {
	Pi      float64 `json:"pi"`
	E       float64 `json:"e"`
	Phi     float64 `json:"phi"`
	Sqrt2   float64 `json:"sqrt2"`
	SqrtE   float64 `json:"sqrt_e"`
	SqrtPi  float64 `json:"sqrt_pi"`
	SqrtPhi float64 `json:"sqrt_phi"`
	Ln2     float64 `json:"ln2"`
	Log2E   float64 `json:"log2_e"`
	Ln10    float64 `json:"ln10"`
	Log10E  float64 `json:"log10_e"`
}

func registerConstants(s *mcp.Server) {
	type empty struct{}
	h := func(_ context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, ConstantsOut, error) {
		return nil, ConstantsOut{
			Pi:      math.Pi,
			E:       math.E,
			Phi:     math.Phi,
			Sqrt2:   math.Sqrt2,
			SqrtE:   math.SqrtE,
			SqrtPi:  math.SqrtPi,
			SqrtPhi: math.SqrtPhi,
			Ln2:     math.Ln2,
			Log2E:   math.Log2E,
			Ln10:    math.Ln10,
			Log10E:  math.Log10E,
		}, nil
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "math_constants",
		Description: "Returns the standard mathematical constants (pi, e, phi, sqrt2, ln2, etc.) at full float64 precision. Use this instead of approximating constants.",
	}, h)
}
