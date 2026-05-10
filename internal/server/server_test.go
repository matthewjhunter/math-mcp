package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/matthewjhunter/math-mcp/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestServerListsAllTools is a smoke test: it spins up the full server, lists
// tools, and asserts that all expected names from each surface are registered.
func TestServerListsAllTools(t *testing.T) {
	ctx := context.Background()

	s := server.New("math-mcp-test", "test")
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}

	expected := []string{
		// math sample
		"math_sqrt", "math_pow", "math_sin", "math_log", "math_atan2",
		"math_constants", "math_deg_to_rad",
		// stats sample
		"stats_mean", "stats_median", "stats_stddev", "stats_percentile",
		"stats_correlation",
		// financial sample
		"financial_pmt", "financial_ipmt", "financial_ppmt", "financial_fv",
		"financial_pv", "financial_npv", "financial_nper", "financial_rate",
	}
	for _, name := range expected {
		if !got[name] {
			t.Errorf("expected tool %q not registered", name)
		}
	}

	// Sanity: prefixed counts so we know each surface registered something substantial.
	var nMath, nStats, nFin int
	for name := range got {
		switch {
		case strings.HasPrefix(name, "math_"):
			nMath++
		case strings.HasPrefix(name, "stats_"):
			nStats++
		case strings.HasPrefix(name, "financial_"):
			nFin++
		}
	}
	if nMath < 25 {
		t.Errorf("expected at least 25 math_ tools, got %d", nMath)
	}
	if nStats < 9 {
		t.Errorf("expected at least 9 stats_ tools, got %d", nStats)
	}
	if nFin < 8 {
		t.Errorf("expected at least 8 financial_ tools, got %d", nFin)
	}
}
