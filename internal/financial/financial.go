// Package financial registers MCP tools that wrap github.com/razorpay/go-financial
// for time-value-of-money calculations. Decimal precision is preserved end-to-end
// via shopspring/decimal.
package financial

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	finance "github.com/razorpay/go-financial"
	"github.com/razorpay/go-financial/enums/paymentperiod"
	"github.com/shopspring/decimal"
)

// Cardinal sign convention from go-financial:
//   - Money you RECEIVE (loans paid to you, account balance you own) is POSITIVE.
//   - Money you PAY OUT (regular payments, deposits) is NEGATIVE.
//
// The "when" field is optional and defaults to "end" (ordinary annuity — interest
// accrues before each payment, the standard mortgage convention). Use "begin" for
// annuities-due where payments occur at the start of each period.

const offload = "Use this tool to compute the value precisely instead of estimating. " +
	"All money inputs and outputs use decimal strings for full precision (no float64 rounding). " +
	"Sign convention: money received is POSITIVE, money paid out is NEGATIVE. "

const whenDoc = `optional, "end" (default) for ordinary annuity / "begin" for annuity-due`

// PmtIn / PmtOut: equal periodic payment.
type PmtIn struct {
	Rate string `json:"rate" jsonschema:"interest rate per period as a decimal string (e.g. \"0.05\" for 5%)"`
	Nper int64  `json:"nper" jsonschema:"total number of payment periods"`
	PV   string `json:"pv" jsonschema:"present value (loan principal received as a positive number)"`
	FV   string `json:"fv,omitempty" jsonschema:"future value at end of term (default \"0\")"`
	When string `json:"when,omitempty" jsonschema:"end (default) or begin"`
}

// FinResult is the standard envelope returned by financial tools.
type FinResult struct {
	Function    string         `json:"function"`
	Inputs      map[string]any `json:"inputs"`
	Result      string         `json:"result" jsonschema:"the computed value as a precise decimal string"`
	ResultFloat float64        `json:"result_float" jsonschema:"the computed value as a float64 (precision lost; provided for convenience only)"`
}

// IPmtIn: interest portion of payment for a given period.
type IPmtIn struct {
	Rate string `json:"rate" jsonschema:"interest rate per period as a decimal string"`
	Per  int64  `json:"per" jsonschema:"the period for which interest is being calculated (1-indexed)"`
	Nper int64  `json:"nper" jsonschema:"total number of payment periods"`
	PV   string `json:"pv" jsonschema:"present value (loan principal)"`
	FV   string `json:"fv,omitempty" jsonschema:"future value at end of term (default \"0\")"`
	When string `json:"when,omitempty" jsonschema:"end (default) or begin"`
}

// PPmtIn: principal portion of payment for a given period.
type PPmtIn = IPmtIn

// FvIn: future value.
type FvIn struct {
	Rate string `json:"rate" jsonschema:"interest rate per period as a decimal string"`
	Nper int64  `json:"nper" jsonschema:"total number of payment periods"`
	Pmt  string `json:"pmt" jsonschema:"payment per period (negative if paying out)"`
	PV   string `json:"pv,omitempty" jsonschema:"present value (default \"0\")"`
	When string `json:"when,omitempty" jsonschema:"end (default) or begin"`
}

// PvIn: present value.
type PvIn struct {
	Rate string `json:"rate" jsonschema:"interest rate per period as a decimal string"`
	Nper int64  `json:"nper" jsonschema:"total number of payment periods"`
	Pmt  string `json:"pmt" jsonschema:"payment per period (negative if paying out)"`
	FV   string `json:"fv,omitempty" jsonschema:"future value (default \"0\")"`
	When string `json:"when,omitempty" jsonschema:"end (default) or begin"`
}

// NpvIn: net present value of an irregular cash-flow stream.
type NpvIn struct {
	Rate     string   `json:"rate" jsonschema:"discount rate per period as a decimal string"`
	CashFlow []string `json:"cash_flow" jsonschema:"cash flows per period as decimal strings, starting at period 0"`
}

// NperIn: number of periods to reach a target.
type NperIn struct {
	Rate string `json:"rate" jsonschema:"interest rate per period as a decimal string"`
	Pmt  string `json:"pmt" jsonschema:"payment per period (negative if paying out)"`
	PV   string `json:"pv" jsonschema:"present value"`
	FV   string `json:"fv,omitempty" jsonschema:"future value (default \"0\")"`
	When string `json:"when,omitempty" jsonschema:"end (default) or begin"`
}

// RateIn: solve for periodic interest rate via Newton-Raphson.
type RateIn struct {
	PV           string `json:"pv" jsonschema:"present value"`
	FV           string `json:"fv" jsonschema:"future value"`
	Pmt          string `json:"pmt" jsonschema:"payment per period (negative if paying out)"`
	Nper         int64  `json:"nper" jsonschema:"number of payment periods"`
	When         string `json:"when,omitempty" jsonschema:"end (default) or begin"`
	MaxIter      int64  `json:"max_iter,omitempty" jsonschema:"maximum Newton-Raphson iterations (default 100)"`
	Tolerance    string `json:"tolerance,omitempty" jsonschema:"convergence tolerance as a decimal string (default \"1e-6\")"`
	InitialGuess string `json:"initial_guess,omitempty" jsonschema:"starting rate guess as a decimal string (default \"0.1\")"`
}

func parseDec(field, s string, defaultVal string) (decimal.Decimal, error) {
	if s == "" {
		s = defaultVal
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%s: invalid decimal %q: %w", field, s, err)
	}
	return d, nil
}

func parseWhen(s string) (paymentperiod.Type, error) {
	switch s {
	case "", "end", "ending", "ENDING":
		return paymentperiod.ENDING, nil
	case "begin", "beginning", "BEGINNING":
		return paymentperiod.BEGINNING, nil
	default:
		return 0, fmt.Errorf("when: must be \"end\" or \"begin\", got %q", s)
	}
}

// schemaWithEnums builds the input schema for T and patches in enum constraints
// for the given properties. Panics if schema inference fails or any named
// property is missing — both are programming errors that should fail at
// registration time, not when the LLM calls the tool.
func schemaWithEnums[T any](enums map[string][]any) *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Errorf("schemaWithEnums: infer %T: %w", *new(T), err))
	}
	for name, values := range enums {
		p, ok := s.Properties[name]
		if !ok {
			panic(fmt.Errorf("schemaWithEnums: %T has no property %q", *new(T), name))
		}
		p.Enum = values
	}
	return s
}

var whenEnum = map[string][]any{"when": {"end", "begin"}}

func envelope(name string, inputs map[string]any, result decimal.Decimal) FinResult {
	f, _ := result.Float64()
	return FinResult{
		Function:    name,
		Inputs:      inputs,
		Result:      result.String(),
		ResultFloat: f,
	}
}

// Register adds all financial tools to the server.
func Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "financial_pmt",
		Description: offload + "Computes the equal periodic payment (principal + interest) for a loan or sinking fund. " + whenDoc,
		InputSchema: schemaWithEnums[PmtIn](whenEnum),
	}, pmtHandler)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "financial_ipmt",
		Description: offload + "Computes the interest portion of the periodic payment for the given period (1-indexed).",
		InputSchema: schemaWithEnums[IPmtIn](whenEnum),
	}, ipmtHandler)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "financial_ppmt",
		Description: offload + "Computes the principal portion of the periodic payment for the given period (1-indexed).",
		InputSchema: schemaWithEnums[PPmtIn](whenEnum),
	}, ppmtHandler)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "financial_fv",
		Description: offload + "Computes the future value of an investment with regular payments.",
		InputSchema: schemaWithEnums[FvIn](whenEnum),
	}, fvHandler)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "financial_pv",
		Description: offload + "Computes the present value of a series of future cash flows.",
		InputSchema: schemaWithEnums[PvIn](whenEnum),
	}, pvHandler)
	mcp.AddTool(s, &mcp.Tool{
		Name: "financial_npv",
		Description: offload + "Computes the net present value of an irregular cash-flow stream at the given discount rate. " +
			"CONVENTION: cash_flow[0] is the period-0 cash flow (NOT discounted). cash_flow[i] is divided by (1+rate)^i. " +
			"This matches numpy_financial; Excel's NPV() treats cash_flow[0] as period 1 — if porting from Excel, " +
			"either include the initial outlay as cash_flow[0] (this tool's convention) or compute Excel-style as " +
			"initial_outlay + NPV(rate, future_flows).",
	}, npvHandler)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "financial_nper",
		Description: offload + "Computes the number of payment periods needed to reach a target.",
		InputSchema: schemaWithEnums[NperIn](whenEnum),
	}, nperHandler)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "financial_rate",
		Description: offload + "Solves for the periodic interest rate via Newton-Raphson iteration. Returns an error if the solver does not converge within max_iter.",
		InputSchema: schemaWithEnums[RateIn](whenEnum),
	}, rateHandler)
}

func pmtHandler(_ context.Context, _ *mcp.CallToolRequest, in PmtIn) (*mcp.CallToolResult, FinResult, error) {
	rate, err := parseDec("rate", in.Rate, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pv, err := parseDec("pv", in.PV, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	fv, err := parseDec("fv", in.FV, "0")
	if err != nil {
		return nil, FinResult{}, err
	}
	when, err := parseWhen(in.When)
	if err != nil {
		return nil, FinResult{}, err
	}
	if in.Nper <= 0 {
		return nil, FinResult{}, errors.New("nper must be positive")
	}
	res := finance.Pmt(rate, in.Nper, pv, fv, when)
	return nil, envelope("financial_pmt", map[string]any{
		"rate": rate.String(), "nper": in.Nper, "pv": pv.String(),
		"fv": fv.String(), "when": whenLabel(when),
	}, res), nil
}

func ipmtHandler(_ context.Context, _ *mcp.CallToolRequest, in IPmtIn) (*mcp.CallToolResult, FinResult, error) {
	return periodPmtHandler("financial_ipmt", in, finance.IPmt)
}

func ppmtHandler(_ context.Context, _ *mcp.CallToolRequest, in PPmtIn) (*mcp.CallToolResult, FinResult, error) {
	return periodPmtHandler("financial_ppmt", in, finance.PPmt)
}

func periodPmtHandler(
	name string,
	in IPmtIn,
	fn func(rate decimal.Decimal, per, nper int64, pv, fv decimal.Decimal, when paymentperiod.Type) decimal.Decimal,
) (*mcp.CallToolResult, FinResult, error) {
	rate, err := parseDec("rate", in.Rate, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pv, err := parseDec("pv", in.PV, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	fv, err := parseDec("fv", in.FV, "0")
	if err != nil {
		return nil, FinResult{}, err
	}
	when, err := parseWhen(in.When)
	if err != nil {
		return nil, FinResult{}, err
	}
	if in.Nper <= 0 {
		return nil, FinResult{}, errors.New("nper must be positive")
	}
	if in.Per <= 0 || in.Per > in.Nper {
		return nil, FinResult{}, fmt.Errorf("per must be in 1..nper (got per=%d, nper=%d)", in.Per, in.Nper)
	}
	res := fn(rate, in.Per, in.Nper, pv, fv, when)
	return nil, envelope(name, map[string]any{
		"rate": rate.String(), "per": in.Per, "nper": in.Nper,
		"pv": pv.String(), "fv": fv.String(), "when": whenLabel(when),
	}, res), nil
}

func fvHandler(_ context.Context, _ *mcp.CallToolRequest, in FvIn) (*mcp.CallToolResult, FinResult, error) {
	rate, err := parseDec("rate", in.Rate, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pmt, err := parseDec("pmt", in.Pmt, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pv, err := parseDec("pv", in.PV, "0")
	if err != nil {
		return nil, FinResult{}, err
	}
	when, err := parseWhen(in.When)
	if err != nil {
		return nil, FinResult{}, err
	}
	if in.Nper <= 0 {
		return nil, FinResult{}, errors.New("nper must be positive")
	}
	res := finance.Fv(rate, in.Nper, pmt, pv, when)
	return nil, envelope("financial_fv", map[string]any{
		"rate": rate.String(), "nper": in.Nper, "pmt": pmt.String(),
		"pv": pv.String(), "when": whenLabel(when),
	}, res), nil
}

func pvHandler(_ context.Context, _ *mcp.CallToolRequest, in PvIn) (*mcp.CallToolResult, FinResult, error) {
	rate, err := parseDec("rate", in.Rate, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pmt, err := parseDec("pmt", in.Pmt, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	fv, err := parseDec("fv", in.FV, "0")
	if err != nil {
		return nil, FinResult{}, err
	}
	when, err := parseWhen(in.When)
	if err != nil {
		return nil, FinResult{}, err
	}
	if in.Nper <= 0 {
		return nil, FinResult{}, errors.New("nper must be positive")
	}
	res := finance.Pv(rate, in.Nper, pmt, fv, when)
	return nil, envelope("financial_pv", map[string]any{
		"rate": rate.String(), "nper": in.Nper, "pmt": pmt.String(),
		"fv": fv.String(), "when": whenLabel(when),
	}, res), nil
}

func npvHandler(_ context.Context, _ *mcp.CallToolRequest, in NpvIn) (*mcp.CallToolResult, FinResult, error) {
	rate, err := parseDec("rate", in.Rate, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	if len(in.CashFlow) == 0 {
		return nil, FinResult{}, errors.New("cash_flow must be non-empty")
	}
	cf := make([]decimal.Decimal, len(in.CashFlow))
	for i, s := range in.CashFlow {
		d, err := parseDec(fmt.Sprintf("cash_flow[%d]", i), s, "")
		if err != nil {
			return nil, FinResult{}, err
		}
		cf[i] = d
	}
	res := finance.Npv(rate, cf)
	echo := make([]string, len(cf))
	for i, d := range cf {
		echo[i] = d.String()
	}
	return nil, envelope("financial_npv", map[string]any{
		"rate": rate.String(), "cash_flow": echo,
	}, res), nil
}

func nperHandler(_ context.Context, _ *mcp.CallToolRequest, in NperIn) (*mcp.CallToolResult, FinResult, error) {
	rate, err := parseDec("rate", in.Rate, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pmt, err := parseDec("pmt", in.Pmt, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pv, err := parseDec("pv", in.PV, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	fv, err := parseDec("fv", in.FV, "0")
	if err != nil {
		return nil, FinResult{}, err
	}
	when, err := parseWhen(in.When)
	if err != nil {
		return nil, FinResult{}, err
	}
	res, ferr := finance.Nper(rate, pmt, pv, fv, when)
	if ferr != nil {
		return nil, FinResult{}, fmt.Errorf("nper: %w", ferr)
	}
	return nil, envelope("financial_nper", map[string]any{
		"rate": rate.String(), "pmt": pmt.String(), "pv": pv.String(),
		"fv": fv.String(), "when": whenLabel(when),
	}, res), nil
}

func rateHandler(_ context.Context, _ *mcp.CallToolRequest, in RateIn) (*mcp.CallToolResult, FinResult, error) {
	pv, err := parseDec("pv", in.PV, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	fv, err := parseDec("fv", in.FV, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	pmt, err := parseDec("pmt", in.Pmt, "")
	if err != nil {
		return nil, FinResult{}, err
	}
	when, err := parseWhen(in.When)
	if err != nil {
		return nil, FinResult{}, err
	}
	if in.Nper <= 0 {
		return nil, FinResult{}, errors.New("nper must be positive")
	}
	maxIter := in.MaxIter
	if maxIter <= 0 {
		maxIter = 100
	}
	tol, err := parseDec("tolerance", in.Tolerance, "0.000001")
	if err != nil {
		return nil, FinResult{}, err
	}
	guess, err := parseDec("initial_guess", in.InitialGuess, "0.1")
	if err != nil {
		return nil, FinResult{}, err
	}
	res, ferr := finance.Rate(pv, fv, pmt, in.Nper, when, maxIter, tol, guess)
	if ferr != nil {
		return nil, FinResult{}, fmt.Errorf("rate did not converge: %w", ferr)
	}
	return nil, envelope("financial_rate", map[string]any{
		"pv": pv.String(), "fv": fv.String(), "pmt": pmt.String(),
		"nper": in.Nper, "when": whenLabel(when), "max_iter": maxIter,
		"tolerance": tol.String(), "initial_guess": guess.String(),
	}, res), nil
}

func whenLabel(t paymentperiod.Type) string {
	if t == paymentperiod.BEGINNING {
		return "begin"
	}
	return "end"
}
