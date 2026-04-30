package backtest

import (
	"context"
	"fmt"

	prometheusclient "github.com/federicolepera/slok/internal/prometheus"
)

// Config holds all parameters for a backtest run.
type Config struct {
	Namespace     string
	Name          string
	ObjectiveName string
	Range         string
	Targets       []float64
}

// TargetResult holds the computed metrics for a single SLO target value.
type TargetResult struct {
	Target          float64
	Availability    float64
	BudgetBurned    float64
	BudgetRemaining float64
	Status          string
}

// Result is the full output of a backtest run.
type Result struct {
	SLOName       string
	Namespace     string
	ObjectiveName string
	Range         string
	Targets       []TargetResult
}

// Backtester queries Prometheus and computes historical SLO compliance.
type Backtester struct {
	client prometheusclient.PrometheusClient
}

// New creates a Backtester backed by the given Prometheus client.
func New(client prometheusclient.PrometheusClient) *Backtester {
	return &Backtester{client: client}
}

// Run executes the backtest for all configured targets.
func (b *Backtester) Run(ctx context.Context, cfg Config) (*Result, error) {
	errorRate, err := b.queryAvgErrorRate(ctx, cfg)
	if err != nil {
		return nil, err
	}

	res := &Result{
		SLOName:       cfg.Name,
		Namespace:     cfg.Namespace,
		ObjectiveName: cfg.ObjectiveName,
		Range:         cfg.Range,
	}
	for _, t := range cfg.Targets {
		res.Targets = append(res.Targets, computeTargetResult(t, errorRate))
	}
	return res, nil
}

// queryAvgErrorRate fetches avg_over_time of the SLI error rate recording rule.
// Requires the operator to have run at least once so that recording rules exist.
func (b *Backtester) queryAvgErrorRate(ctx context.Context, cfg Config) (float64, error) {
	selector := fmt.Sprintf(
		`slo_name=%q,slo_namespace=%q,objective_name=%q`,
		cfg.Name, cfg.Namespace, cfg.ObjectiveName,
	)
	query := fmt.Sprintf(`avg_over_time(slok:sli_error_rate:5m{%s}[%s])`, selector, cfg.Range)
	val, err := b.client.QuerySLI(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("querying Prometheus (has the operator run at least once?): %w", err)
	}
	return val, nil
}

// computeTargetResult derives all SLO compliance metrics for one target value.
func computeTargetResult(target, errorRate float64) TargetResult {
	errorBudgetTarget := 1 - target/100
	availability := (1 - errorRate) * 100

	var budgetBurned float64
	if errorBudgetTarget > 0 {
		budgetBurned = (errorRate / errorBudgetTarget) * 100
	}
	budgetRemaining := 100 - budgetBurned

	status := "PASS"
	if budgetBurned > 100 {
		status = "FAIL"
	}

	return TargetResult{
		Target:          target,
		Availability:    availability,
		BudgetBurned:    budgetBurned,
		BudgetRemaining: budgetRemaining,
		Status:          status,
	}
}
