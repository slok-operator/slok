package backtest

import (
	"context"
	"fmt"
	"strings"

	prometheusclient "github.com/federicolepera/slok/internal/prometheus"
)

const (
	SourceRecordingRules = "existing SloK recording rules"
	SourceRawSLIQueries  = "raw SLI queries from YAML"
)

// Config holds all parameters for a backtest run.
type Config struct {
	Namespace     string
	Name          string
	ObjectiveName string
	Range         string
	Targets       []float64
	TotalQuery    string
	ErrorQuery    string
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
	Source        string
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
	errorRate, source, err := b.queryAvgErrorRate(ctx, cfg)
	if err != nil {
		return nil, err
	}

	res := &Result{
		SLOName:       cfg.Name,
		Namespace:     cfg.Namespace,
		ObjectiveName: cfg.ObjectiveName,
		Range:         cfg.Range,
		Source:        source,
	}
	for _, t := range cfg.Targets {
		res.Targets = append(res.Targets, computeTargetResult(t, errorRate))
	}
	return res, nil
}

// queryAvgErrorRate fetches avg_over_time of the SLI error rate recording rule.
// Requires the operator to have run at least once so that recording rules exist.
func (b *Backtester) queryAvgErrorRate(ctx context.Context, cfg Config) (float64, string, error) {
	query, source, err := buildErrorRateQuery(cfg)
	if err != nil {
		return 0, "", err
	}

	val, err := b.client.QuerySLI(ctx, query)
	if err != nil {
		if source == SourceRawSLIQueries {
			return 0, "", fmt.Errorf("querying Prometheus using raw SLI queries from YAML: %w", err)
		}
		return 0, "", fmt.Errorf("querying Prometheus (has the operator run at least once?): %w", err)
	}
	return val, source, nil
}

func buildErrorRateQuery(cfg Config) (string, string, error) {
	if cfg.TotalQuery != "" || cfg.ErrorQuery != "" {
		query, err := buildRawSLIErrorRateQuery(cfg.TotalQuery, cfg.ErrorQuery, cfg.Range)
		return query, SourceRawSLIQueries, err
	}

	return buildRecordingRuleErrorRateQuery(cfg), SourceRecordingRules, nil
}

func buildRecordingRuleErrorRateQuery(cfg Config) string {
	selector := fmt.Sprintf(
		`slo_name=%q,slo_namespace=%q,objective_name=%q`,
		cfg.Name, cfg.Namespace, cfg.ObjectiveName,
	)
	return fmt.Sprintf(`avg_over_time(slok:sli_error_rate:5m{%s}[%s])`, selector, cfg.Range)
}

func buildRawSLIErrorRateQuery(totalQuery, errorQuery, rangeStr string) (string, error) {
	totalQuery = strings.TrimSpace(totalQuery)
	errorQuery = strings.TrimSpace(errorQuery)
	rangeStr = strings.TrimSpace(rangeStr)

	if totalQuery == "" || errorQuery == "" {
		return "", fmt.Errorf("pre-apply backtesting requires both totalQuery and errorQuery")
	}
	if rangeStr == "" {
		return "", fmt.Errorf("pre-apply backtesting requires a range")
	}

	return fmt.Sprintf(
		`sum(increase(%s[%s])) / clamp_min(sum(increase(%s[%s])), 1e-12)`,
		errorQuery, rangeStr, totalQuery, rangeStr,
	), nil
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
