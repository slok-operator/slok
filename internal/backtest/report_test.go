package backtest

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintSingleTarget(t *testing.T) {
	var out bytes.Buffer
	Print(&out, &Result{
		SLOName:       "checkout",
		Namespace:     "default",
		ObjectiveName: "availability",
		Range:         "30d",
		Targets: []TargetResult{{
			Target:          99.9,
			Availability:    99.95,
			BudgetBurned:    50,
			BudgetRemaining: 50,
			Status:          "PASS",
		}},
	})

	for _, want := range []string{
		"SLO:    checkout/availability",
		"NS:     default",
		"Window: 30d",
		"Target: 99.90%",
		"Availability:        99.9500%",
		"Error budget burned: 50.00%",
		"Budget remaining:    50.00%",
		"Status:              PASS",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output does not contain %q:\n%s", want, out.String())
		}
	}
}

func TestPrintMultipleTargets(t *testing.T) {
	var out bytes.Buffer
	Print(&out, &Result{
		SLOName:       "checkout",
		Namespace:     "default",
		ObjectiveName: "availability",
		Range:         "30d",
		Targets: []TargetResult{
			{Target: 99, Availability: 99.95, BudgetRemaining: 95, Status: "PASS"},
			{Target: 99.99, Availability: 99.95, BudgetRemaining: -400, Status: "FAIL"},
		},
	})

	for _, want := range []string{
		"SLO: checkout/availability  |  Namespace: default  |  Window: 30d",
		"Target",
		"Availability",
		"Budget remaining",
		"99.00%",
		"99.9500%",
		"PASS",
		"FAIL",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output does not contain %q:\n%s", want, out.String())
		}
	}
}
