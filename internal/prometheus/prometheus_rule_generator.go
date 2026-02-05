package prometheus

import (
	"fmt"
	"strings"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/templates"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// burnRatePreset defines a predefined multi-window burn rate alerting tier
// based on the Google SRE Workbook approach.
type burnRatePreset struct {
	ShortWindow string
	LongWindow  string
	BurnRate    float64
	Severity    string
	AlertSuffix string
	For         string
}

// Default burn rate presets (multi-window, multi-burn-rate).
//
//	Short   Long   Burn   Meaning
//	5m      1h     >14x   outage
//	1h      6h     >6x    high burn
//	6h      3d     >1x    erosion
//	7d      30d    >0.5x  slow burn
var defaultBurnRatePresets = []burnRatePreset{
	{ShortWindow: "5m", LongWindow: "1h", BurnRate: 14, Severity: "critical", AlertSuffix: "Critical", For: "2m"},
	{ShortWindow: "1h", LongWindow: "6h", BurnRate: 6, Severity: "warning", AlertSuffix: "Degraded", For: "15m"},
	{ShortWindow: "6h", LongWindow: "3d", BurnRate: 1, Severity: "warning", AlertSuffix: "Warning", For: "1h"},
}

// Windows for which we generate recording rules
var recordingWindows = []string{"5m", "1h", "6h", "3d", "7d", "30d"}

// baseLabels returns the common labels used across all rules
func baseLabels(sloName, sloNamespace, objectiveName, window string) map[string]string {
	return map[string]string{
		"slo_name":       sloName,
		"slo_namespace":  sloNamespace,
		"objective_name": objectiveName,
		"objective_id":   fmt.Sprintf("%s/%s", sloName, objectiveName),
		"slok_window":    window,
	}
}

// sliErrorRateExpr builds the zero-traffic safe SLI error rate expression
func sliErrorRateExpr(errorQuery, totalQuery, window string) string {
	return fmt.Sprintf(
		"(sum(rate(%s[%s])) OR (sum(rate(%s[%s])) * 0)) / clamp_min(sum(rate(%s[%s])), 1e-12)",
		errorQuery, window, totalQuery, window, totalQuery, window,
	)
}

// burnRateExpr builds the burn rate expression referencing recording rules
func burnRateExpr(sloName, sloNamespace, objectiveName, window string) string {
	selector := fmt.Sprintf(`slo_name="%s", slo_namespace="%s", objective_name="%s"`, sloName, sloNamespace, objectiveName)
	return fmt.Sprintf(
		"slok:sli_error_rate:%s{%s} / on (slo_name, slo_namespace, objective_name) slok:error_budget_target{%s}",
		window, selector, selector,
	)
}

// burnRateAlertExpr builds the multi-window burn rate alert expression
func burnRateAlertExpr(sloName, sloNamespace, objectiveName string, preset burnRatePreset) string {
	selector := fmt.Sprintf(`slo_name="%s", slo_namespace="%s", objective_name="%s"`, sloName, sloNamespace, objectiveName)
	return fmt.Sprintf(
		"slok:burn_rate:%s{%s} > %g AND slok:burn_rate:%s{%s} > %g",
		preset.ShortWindow, selector, preset.BurnRate,
		preset.LongWindow, selector, preset.BurnRate,
	)
}

func CreatePrometheusRule(sloName, sloNamespace string, objective observabilityv1alpha1.Objective) (monitoringv1.PrometheusRule, error) {
	objectiveName := objective.Name

	// Resolve queries from template or use manual queries
	resolved, err := templates.Resolve(objective.Sli)
	if err != nil {
		return monitoringv1.PrometheusRule{}, fmt.Errorf("failed to resolve SLI queries: %w", err)
	}

	prometheusRule := monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slok-%s-%s", sloName, objectiveName),
			Namespace: sloNamespace,
			Labels: map[string]string{
				"release":           "prometheus",
				"slok.io/slo":       sloName,
				"slok.io/objective": objectiveName,
			},
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{Name: fmt.Sprintf("slok.%s.%s", sloName, objectiveName)},
			},
		},
	}

	rules := &prometheusRule.Spec.Groups[0].Rules

	// SLI error rate recording rules for each window
	for _, window := range recordingWindows {
		var expr string
		if resolved.IsRawExpression() {
			// Use raw expression with window placeholder replaced
			expr = strings.ReplaceAll(resolved.RawExpr, "{{window}}", window)
		} else {
			// Use standard error/total rate calculation
			expr = sliErrorRateExpr(resolved.ErrorQuery, resolved.TotalQuery, window)
		}
		*rules = append(*rules, monitoringv1.Rule{
			Record: fmt.Sprintf("slok:sli_error_rate:%s", window),
			Expr:   intstr.FromString(expr),
			Labels: baseLabels(sloName, sloNamespace, objectiveName, window),
		})
	}

	// Objective target constant
	*rules = append(*rules, monitoringv1.Rule{
		Record: "slok:objective_target",
		Expr:   intstr.FromString(fmt.Sprintf("vector(%g)", objective.Target/100)),
		Labels: baseLabels(sloName, sloNamespace, objectiveName, "30d"),
	})

	// Error budget target constant (1 - target)
	*rules = append(*rules, monitoringv1.Rule{
		Record: "slok:error_budget_target",
		Expr:   intstr.FromString(fmt.Sprintf("vector(%g)", 1-objective.Target/100)),
		Labels: baseLabels(sloName, sloNamespace, objectiveName, "30d"),
	})

	// Burn rate recording rules for each window
	for _, window := range recordingWindows {
		*rules = append(*rules, monitoringv1.Rule{
			Record: fmt.Sprintf("slok:burn_rate:%s", window),
			Expr:   intstr.FromString(burnRateExpr(sloName, sloNamespace, objectiveName, window)),
			Labels: baseLabels(sloName, sloNamespace, objectiveName, window),
		})
	}

	// Budget error alerts
	if objective.Alerting.BudgetErrorAlerts.Enabled {
		budgetSelector := fmt.Sprintf(
			`namespace="%s", service_level_objective="%s", objective_name="%s"`,
			sloNamespace, sloName, objectiveName,
		)

		*rules = append(*rules,
			monitoringv1.Rule{
				Alert: "SLOObjectiveAtRisk",
				Expr: intstr.FromString(fmt.Sprintf(
					"optimization_request_objective_percent_remaining{%s} > 0 and optimization_request_objective_percent_remaining{%s} < 10",
					budgetSelector, budgetSelector,
				)),
				For:    monitoringv1.DurationPointer("3m"),
				Labels: map[string]string{"severity": "warning"},
			},
			monitoringv1.Rule{
				Alert: "SLOObjectiveViolatedWarning",
				Expr: intstr.FromString(fmt.Sprintf(
					"optimization_request_objective_percent_remaining{%s} <= 0",
					budgetSelector,
				)),
				For:    monitoringv1.DurationPointer("5m"),
				Labels: map[string]string{"severity": "warning"},
			},
		)

		// Custom budget alerts from spec
		for _, threshold := range objective.Alerting.BudgetErrorAlerts.Alerts {
			*rules = append(*rules, monitoringv1.Rule{
				Alert: threshold.Name,
				Expr: intstr.FromString(fmt.Sprintf(
					"optimization_request_objective_percent_remaining{%s} < %.2f",
					budgetSelector, threshold.Percent,
				)),
				For:    monitoringv1.DurationPointer("1m"),
				Labels: map[string]string{"severity": threshold.Severity},
			})
		}
	}

	// Burn rate alerts using presets
	if objective.Alerting.BurnRateAlerts.Enabled {
		alertGroup := monitoringv1.RuleGroup{
			Name: fmt.Sprintf("slok.%s.%s-burnRateAlerts", sloName, objectiveName),
		}

		// Multi-window burn rate alerts from presets
		for _, preset := range defaultBurnRatePresets {
			alertGroup.Rules = append(alertGroup.Rules, monitoringv1.Rule{
				Alert:  fmt.Sprintf("Objective: %s SLOBurnRateHigh - %s", objectiveName, preset.AlertSuffix),
				Expr:   intstr.FromString(burnRateAlertExpr(sloName, sloNamespace, objectiveName, preset)),
				Labels: baseLabels(sloName, sloNamespace, objectiveName, preset.ShortWindow),
			})
			alertGroup.Rules[len(alertGroup.Rules)-1].Labels["severity"] = preset.Severity
		}

		// Error budget exhausted alert
		selector := fmt.Sprintf(`slo_name="%s", slo_namespace="%s", objective_name="%s"`, sloName, sloNamespace, objectiveName)
		alertGroup.Rules = append(alertGroup.Rules, monitoringv1.Rule{
			Alert: fmt.Sprintf("Objective: %s ErrorBudget Finished - Violated", objectiveName),
			Expr: intstr.FromString(fmt.Sprintf(
				"slok:sli_error_rate:%s{%s} > slok:error_budget_target{%s}",
				objective.Window, selector, selector,
			)),
			Labels: baseLabels(sloName, sloNamespace, objectiveName, objective.Window),
		})
		alertGroup.Rules[len(alertGroup.Rules)-1].Labels["severity"] = "warning"

		prometheusRule.Spec.Groups = append(prometheusRule.Spec.Groups, alertGroup)
	}

	return prometheusRule, nil
}
