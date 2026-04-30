package backtest

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

type fakePrometheusClient struct {
	query string
	value float64
	err   error
}

func (f *fakePrometheusClient) QuerySLI(_ context.Context, query string) (float64, error) {
	f.query = query
	return f.value, f.err
}

func (f *fakePrometheusClient) CheckConnection(context.Context) error { return nil }

func (f *fakePrometheusClient) QuerySLINotNormalized(context.Context, string) (float64, error) {
	return 0, nil
}

func (f *fakePrometheusClient) GetURL() string { return "http://prometheus.example" }

func TestRunComputesTargetResults(t *testing.T) {
	client := &fakePrometheusClient{value: 0.0005}
	result, err := New(client).Run(context.Background(), Config{
		Namespace:     "default",
		Name:          "checkout",
		ObjectiveName: "availability",
		Range:         "30d",
		Targets:       []float64{99.9, 99.95},
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if result.SLOName != "checkout" || result.Namespace != "default" || result.ObjectiveName != "availability" || result.Range != "30d" {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
	if len(result.Targets) != 2 {
		t.Fatalf("expected 2 target results, got %d", len(result.Targets))
	}
	if got := result.Targets[0]; got.Status != "PASS" || !almostEqual(got.Availability, 99.95) || !almostEqual(got.BudgetBurned, 50) || !almostEqual(got.BudgetRemaining, 50) {
		t.Fatalf("unexpected 99.9 target result: %#v", got)
	}
	if got := result.Targets[1]; got.Status != "FAIL" || got.BudgetBurned <= 100 || got.BudgetRemaining >= 0 {
		t.Fatalf("unexpected 99.95 target result: %#v", got)
	}
}

func TestRunEscapesPrometheusLabelValues(t *testing.T) {
	client := &fakePrometheusClient{value: 0.01}
	_, err := New(client).Run(context.Background(), Config{
		Namespace:     `team\"a`,
		Name:          `checkout\"api`,
		ObjectiveName: `availability\"main`,
		Range:         "7d",
		Targets:       []float64{99},
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	for _, want := range []string{
		`slo_name="checkout\\\"api"`,
		`slo_namespace="team\\\"a"`,
		`objective_name="availability\\\"main"`,
		`[7d]`,
	} {
		if !strings.Contains(client.query, want) {
			t.Fatalf("query %q does not contain %q", client.query, want)
		}
	}
}

func TestRunReturnsPrometheusErrors(t *testing.T) {
	wantErr := errors.New("prometheus unavailable")
	_, err := New(&fakePrometheusClient{err: wantErr}).Run(context.Background(), Config{
		Namespace:     "default",
		Name:          "checkout",
		ObjectiveName: "availability",
		Range:         "30d",
		Targets:       []float64{99.9},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "has the operator run at least once") || !errors.Is(err, wantErr) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComputeTargetResult(t *testing.T) {
	result := computeTargetResult(99.9, 0.0015)

	if result.Status != "FAIL" {
		t.Fatalf("expected FAIL, got %s", result.Status)
	}
	if !almostEqual(result.Availability, 99.85) {
		t.Fatalf("expected availability 99.85, got %f", result.Availability)
	}
	if result.BudgetBurned < 149.99 || result.BudgetBurned > 150.01 {
		t.Fatalf("expected budget burned around 150, got %f", result.BudgetBurned)
	}
	if result.BudgetRemaining < -50.01 || result.BudgetRemaining > -49.99 {
		t.Fatalf("expected budget remaining around -50, got %f", result.BudgetRemaining)
	}
}

func almostEqual(left, right float64) bool {
	return math.Abs(left-right) < 0.000001
}
