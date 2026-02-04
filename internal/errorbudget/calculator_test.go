package errorbudget

import (
	"testing"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/burnrate"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name           string
		objective      observabilityv1alpha1.Objective
		sliErrorRate   float64
		expectedBudget *Budget
		expectError    bool
	}{
		{
			name: "Valid percentage calculation with 30d window",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.9,
				Window: "30d",
				Sli:    observabilityv1alpha1.SLI{Query: observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliErrorRate=0.005 → actual=99.5, target=99.9 → budget exhausted
			sliErrorRate: 0.005,
			expectedBudget: &Budget{
				Total:            "43.2m",
				Consumed:         "216.0m",
				Remaining:        "0.0m",
				PercentRemaining: 0,
			},
			expectError: false,
		},
		{
			name: "Valid percentage calculation with 7d window",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.0,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Query: observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliErrorRate=0.005 → actual=99.5, target=99.0 → 50% budget remaining
			sliErrorRate: 0.005,
			expectedBudget: &Budget{
				Total:            "100.8m",
				Consumed:         "50.4m",
				Remaining:        "50.4m",
				PercentRemaining: 50,
			},
			expectError: false,
		},
		{
			name: "Low budget scenario",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.5,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Query: observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliErrorRate=0.00455 → actual=99.545, target=99.5 → ~9% budget remaining
			sliErrorRate: 0.00455,
			expectedBudget: &Budget{
				Total:            "50.4m",
				Consumed:         "45.9m",
				Remaining:        "4.5m",
				PercentRemaining: 9,
			},
			expectError: false,
		},
		{
			name: "Zero error rate returns full budget",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.9,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Query: observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliErrorRate=0 → actual=100, full budget remaining
			sliErrorRate: 0,
			expectedBudget: &Budget{
				Total:            "10.1m",
				Consumed:         "0.0m",
				Remaining:        "10.1m",
				PercentRemaining: 100,
			},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget, _, err := Calculate(tt.objective, tt.sliErrorRate)
			if (err != nil) != tt.expectError {
				t.Errorf("Calculate() error = %v, expectError %v", err, tt.expectError)
				return
			}
			boolEqual := budget.Total == tt.expectedBudget.Total &&
				budget.Consumed == tt.expectedBudget.Consumed &&
				budget.Remaining == tt.expectedBudget.Remaining &&
				budget.PercentRemaining == tt.expectedBudget.PercentRemaining
			if !boolEqual {
				t.Errorf("Calculate() = %v, expected %v", budget, tt.expectedBudget)
			}
		})
	}
}

func TestDetermineStatus(t *testing.T) {
	lowBurnRates := []burnrate.BurnRate{
		{ShortBurnRate: 0.5, LongBurnRate: 0.5, ShortWindow: "5m", LongWindow: "1h"},
		{ShortBurnRate: 0.5, LongBurnRate: 0.5, ShortWindow: "1h", LongWindow: "6h"},
		{ShortBurnRate: 0.5, LongBurnRate: 0.5, ShortWindow: "6h", LongWindow: "3d"},
		{ShortBurnRate: 0.5, LongBurnRate: 0.5, ShortWindow: "7d", LongWindow: "30d"},
	}

	tests := []struct {
		name             string
		target           float64
		actual           float64
		percentRemaining float64
		burnRates        []burnrate.BurnRate
		expectedStatus   string
	}{
		{
			name:             "Met status - healthy with low burn rates",
			target:           99.9,
			actual:           99.95,
			percentRemaining: 80,
			burnRates:        lowBurnRates,
			expectedStatus:   "met",
		},
		{
			name:             "Violated status - budget exhausted",
			target:           99.9,
			actual:           99.85,
			percentRemaining: 0,
			burnRates:        lowBurnRates,
			expectedStatus:   "violated",
		},
		{
			name:             "Critical status - outage burn rate on 5m/1h",
			target:           99.9,
			actual:           99.95,
			percentRemaining: 80,
			burnRates: []burnrate.BurnRate{
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "5m", LongWindow: "1h"},
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "1h", LongWindow: "6h"},
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "6h", LongWindow: "3d"},
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "7d", LongWindow: "30d"},
			},
			expectedStatus: "critical",
		},
		{
			name:             "Degraded status - high burn rate on 1h/6h",
			target:           99.9,
			actual:           99.95,
			percentRemaining: 80,
			burnRates: []burnrate.BurnRate{
				{ShortBurnRate: 10, LongBurnRate: 10, ShortWindow: "5m", LongWindow: "1h"},
				{ShortBurnRate: 10, LongBurnRate: 10, ShortWindow: "1h", LongWindow: "6h"},
				{ShortBurnRate: 10, LongBurnRate: 10, ShortWindow: "6h", LongWindow: "3d"},
				{ShortBurnRate: 10, LongBurnRate: 10, ShortWindow: "7d", LongWindow: "30d"},
			},
			expectedStatus: "degraded",
		},
		{
			name:             "Warning status - erosion burn rate on 6h/3d",
			target:           99.9,
			actual:           99.95,
			percentRemaining: 80,
			burnRates: []burnrate.BurnRate{
				{ShortBurnRate: 3, LongBurnRate: 3, ShortWindow: "5m", LongWindow: "1h"},
				{ShortBurnRate: 3, LongBurnRate: 3, ShortWindow: "1h", LongWindow: "6h"},
				{ShortBurnRate: 3, LongBurnRate: 3, ShortWindow: "6h", LongWindow: "3d"},
				{ShortBurnRate: 3, LongBurnRate: 3, ShortWindow: "7d", LongWindow: "30d"},
			},
			expectedStatus: "warning",
		},
		{
			name:             "Violated takes priority over high burn rate",
			target:           99.9,
			actual:           99.5,
			percentRemaining: 0,
			burnRates: []burnrate.BurnRate{
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "5m", LongWindow: "1h"},
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "1h", LongWindow: "6h"},
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "6h", LongWindow: "3d"},
				{ShortBurnRate: 20, LongBurnRate: 20, ShortWindow: "7d", LongWindow: "30d"},
			},
			expectedStatus: "violated",
		},
		{
			name:             "Met with empty burn rates",
			target:           99.9,
			actual:           99.95,
			percentRemaining: 80,
			burnRates:        []burnrate.BurnRate{},
			expectedStatus:   "met",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := DetermineStatus(tt.target, tt.actual, tt.percentRemaining, tt.burnRates)
			if status != tt.expectedStatus {
				t.Errorf("DetermineStatus() = %v, expected %v", status, tt.expectedStatus)
			}
		})
	}
}
