package errorbudget

import (
	"testing"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name           string
		objective      observabilityv1alpha1.Objective
		sliValue       float64
		expectedBudget *Budget
		expectError    bool
	}{
		{
			name: "Valid percentage calculation with 30d window",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.9,
				Window: "30d",
				Sli:    observabilityv1alpha1.SLI{Type: "percentage", Query: "dummy"},
			},
			sliValue: 99.5,
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
				Sli:    observabilityv1alpha1.SLI{Type: "percentage", Query: "dummy"},
			},
			sliValue: 99.5,
			expectedBudget: &Budget{
				Total:            "100.8m",
				Consumed:         "50.4m",
				Remaining:        "50.4m",
				PercentRemaining: 50,
			},
			expectError: false,
		},
		{
			name: "At risk scenario (percentage)",
			objective: observabilityv1alpha1.Objective{
				Name:   "availability",
				Target: 99.5,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Type: "percentage", Query: "dummy"},
			},
			sliValue: 99.545,
			expectedBudget: &Budget{
				Total:            "50.4m",
				Consumed:         "45.9m",
				Remaining:        "4.5m",
				PercentRemaining: 9,
			},
			expectError: false,
		},
		{
			name: "Threshold < operator - within budget",
			objective: observabilityv1alpha1.Objective{
				Name:   "latency-p99",
				Target: 500,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Type: "threshold", Query: "dummy", Operator: "<"},
			},
			sliValue: 200,
			expectedBudget: &Budget{
				Total:            "500.0",
				Consumed:         "200.0",
				Remaining:        "300.0",
				PercentRemaining: 40,
			},
			expectError: false,
		},
		{
			name: "Threshold < operator - budget exhausted",
			objective: observabilityv1alpha1.Objective{
				Name:   "latency-p99",
				Target: 500,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Type: "threshold", Query: "dummy", Operator: "<"},
			},
			sliValue: 600,
			expectedBudget: &Budget{
				Total:            "500.0",
				Consumed:         "600.0",
				Remaining:        "-100.0",
				PercentRemaining: 0,
			},
			expectError: false,
		},
		{
			name: "Threshold > operator - within budget",
			objective: observabilityv1alpha1.Objective{
				Name:   "throughput",
				Target: 1000,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Type: "threshold", Query: "dummy", Operator: ">"},
			},
			sliValue: 1500,
			expectedBudget: &Budget{
				Total:            "1000.0",
				Consumed:         "1500.0",
				Remaining:        "500.0",
				PercentRemaining: 50,
			},
			expectError: false,
		},
		{
			name: "Threshold > operator - budget exhausted",
			objective: observabilityv1alpha1.Objective{
				Name:   "throughput",
				Target: 1000,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Type: "threshold", Query: "dummy", Operator: ">"},
			},
			sliValue: 800,
			expectedBudget: &Budget{
				Total:            "1000.0",
				Consumed:         "800.0",
				Remaining:        "-200.0",
				PercentRemaining: 0,
			},
			expectError: false,
		},
		{
			name: "Threshold <= operator - at boundary",
			objective: observabilityv1alpha1.Objective{
				Name:   "latency",
				Target: 300,
				Window: "7d",
				Sli:    observabilityv1alpha1.SLI{Type: "threshold", Query: "dummy", Operator: "<="},
			},
			sliValue: 300,
			expectedBudget: &Budget{
				Total:            "300.0",
				Consumed:         "300.0",
				Remaining:        "0.0",
				PercentRemaining: 0,
			},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget, err := Calculate(tt.objective, tt.sliValue)
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
	tests := []struct {
		name             string
		target           float64
		actual           float64
		percentRemaining float64
		expectedStatus   string
	}{
		{
			name:             "Healthy status",
			target:           99.9,
			actual:           99.95,
			percentRemaining: 80,
			expectedStatus:   "met",
		},
		{
			name:             "Violated status due to low SLI",
			target:           99.9,
			actual:           99.85,
			percentRemaining: 0,
			expectedStatus:   "violated",
		},
		{
			name:             "At risk status",
			target:           99.5,
			actual:           99.545,
			percentRemaining: 9,
			expectedStatus:   "at-risk",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := DetermineStatus(tt.target, tt.actual, tt.percentRemaining)
			if status != tt.expectedStatus {
				t.Errorf("DetermineStatus() = %v, expected %v", status, tt.expectedStatus)
			}
		})
	}
}
