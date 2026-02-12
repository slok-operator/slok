package errorbudget

import (
	"testing"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/burnrate"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name                string
		objective           observabilityv1alpha1.Objective
		sliBurnRateWindowed float64
		sliErrorRate        float64
		expectedBudget      *Budget
		expectedActual      float64
		expectError         bool
	}{
		{
			name: "Budget exhausted with 30d window",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.9,
				Window: "30d",
				Sli:    observabilityv1alpha1.SLI{Query: &observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliBurnRateWindowed=1.0 → 100% consumed, 0% remaining
			sliBurnRateWindowed: 1.0,
			sliErrorRate:        0.005,
			expectedBudget: &Budget{
				Total:            "43200.0m",
				Consumed:         "43200.0m",
				Remaining:        "0.0m",
				PercentRemaining: 0,
			},
			expectedActual: 99.5,
			expectError:    false,
		},
		{
			name: "50% budget remaining with 7d window",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.0,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Query: &observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliBurnRateWindowed=0.5 → 50% consumed, 50% remaining
			sliBurnRateWindowed: 0.5,
			sliErrorRate:        0.005,
			expectedBudget: &Budget{
				Total:            "10080.0m",
				Consumed:         "5040.0m",
				Remaining:        "5040.0m",
				PercentRemaining: 50,
			},
			expectedActual: 99.5,
			expectError:    false,
		},
		{
			name: "Low budget scenario - 9% remaining",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.5,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Query: &observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliBurnRateWindowed=0.91 → 91% consumed, 9% remaining
			sliBurnRateWindowed: 0.91,
			sliErrorRate:        0.00455,
			expectedBudget: &Budget{
				Total:            "10080.0m",
				Consumed:         "9172.8m",
				Remaining:        "907.2m",
				PercentRemaining: 9,
			},
			expectedActual: 99.55,
			expectError:    false,
		},
		{
			name: "Zero burn rate returns full budget",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.9,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Query: &observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliBurnRateWindowed=0 → 0% consumed, 100% remaining
			sliBurnRateWindowed: 0,
			sliErrorRate:        0,
			expectedBudget: &Budget{
				Total:            "10080.0m",
				Consumed:         "0.0m",
				Remaining:        "10080.0m",
				PercentRemaining: 100,
			},
			expectedActual: 100,
			expectError:    false,
		},
		{
			name: "Over-burned budget clamps to zero",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.9,
				Window: "1d",
				Sli:    observabilityv1alpha1.SLI{Query: &observabilityv1alpha1.Query{TotalQuery: "dummy_total", ErrorQuery: "dummy_error"}},
			},
			// sliBurnRateWindowed=1.5 → over 100% consumed, remaining clamped to 0
			sliBurnRateWindowed: 1.5,
			sliErrorRate:        0.01,
			expectedBudget: &Budget{
				Total:            "1440.0m",
				Consumed:         "2160.0m",
				Remaining:        "0.0m",
				PercentRemaining: 0,
			},
			expectedActual: 99,
			expectError:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget, actual, err := Calculate(tt.objective, tt.sliBurnRateWindowed, tt.sliErrorRate)
			if (err != nil) != tt.expectError {
				t.Errorf("Calculate() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if actual != tt.expectedActual {
				t.Errorf("Calculate() actual = %v, expected %v", actual, tt.expectedActual)
			}
			boolEqual := budget.Total == tt.expectedBudget.Total &&
				budget.Consumed == tt.expectedBudget.Consumed &&
				budget.Remaining == tt.expectedBudget.Remaining &&
				budget.PercentRemaining == tt.expectedBudget.PercentRemaining
			if !boolEqual {
				t.Errorf("Calculate() budget = %v, expected %v", budget, tt.expectedBudget)
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
