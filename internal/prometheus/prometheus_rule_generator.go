package prometheus

import (
	"fmt"
	"strconv"
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
	{ShortWindow: "1h", LongWindow: "6h", BurnRate: 6, Severity: severityWarning, AlertSuffix: "Degraded", For: "15m"},
	{ShortWindow: "6h", LongWindow: "3d", BurnRate: 1, Severity: severityWarning, AlertSuffix: "Warning", For: "1h"},
}

const severityWarning = "warning"

func parseAlertWindowHours(window string) (float64, error) {
	if len(window) < 2 {
		return 0, fmt.Errorf("invalid duration %q", window)
	}

	value, err := strconv.ParseFloat(window[:len(window)-1], 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid duration %q", window)
	}

	switch window[len(window)-1] {
	case 'd':
		return value * 24, nil
	case 'h':
		return value, nil
	case 'm':
		return value / 60, nil
	case 's':
		return value / 3600, nil
	default:
		return 0, fmt.Errorf("invalid duration %q", window)
	}
}

func burnRateThreshold(alert observabilityv1alpha1.BurnRateAlert, sloWindow string) (float64, error) {
	sloWindowHours, err := parseAlertWindowHours(sloWindow)
	if err != nil {
		return 0, fmt.Errorf("invalid SLO window: %w", err)
	}

	consumeWindowHours, err := parseAlertWindowHours(alert.ConsumeWindow)
	if err != nil {
		return 0, fmt.Errorf("invalid consume window: %w", err)
	}

	return (alert.ConsumePercent / 100) * sloWindowHours / consumeWindowHours, nil
}

func burnRatePresetsForObjective(objective observabilityv1alpha1.Objective) ([]burnRatePreset, error) {
	if objective.Alerting == nil ||
		objective.Alerting.BurnRateAlerts == nil ||
		len(objective.Alerting.BurnRateAlerts.Alerts) == 0 {
		return defaultBurnRatePresets, nil
	}

	burnRates := objective.Alerting.BurnRateAlerts
	presets := make([]burnRatePreset, 0, len(burnRates.Alerts))

	for _, alert := range burnRates.Alerts {
		threshold, err := burnRateThreshold(alert, objective.Window)
		if err != nil {
			return nil, fmt.Errorf("invalid burn-rate alert %q: %w", alert.Name, err)
		}

		presets = append(presets, burnRatePreset{
			ShortWindow: alert.ShortWindow,
			LongWindow:  alert.LongWindow,
			BurnRate:    threshold,
			Severity:    alert.Severity,
			AlertSuffix: alert.Name,
			For:         alert.ConsumeWindow,
		})
	}

	return presets, nil
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

// baseLabelsComposition returns the common labels for aggregated composition rules.
// Follows the same pattern as baseLabels but uses composition-scoped label keys.
func baseLabelsComposition(compositionName, compositionNamespace, window string) map[string]string {
	return map[string]string{
		"slo_composition_name":      compositionName,
		"slo_composition_namespace": compositionNamespace,
		"composition_id":            fmt.Sprintf("%s/%s", compositionName, compositionNamespace),
		"slok_window":               window,
	}
}

// sliErrorRateExpr builds the zero-traffic safe SLI error rate expression.
// When the service is completely down (no metrics), returns 1 (100% error rate)
// via the OR absent(...) fallback.
func sliErrorRateExpr(errorQuery, totalQuery, window string) string {
	return fmt.Sprintf(
		"((sum(rate(%s[%s])) OR (sum(rate(%s[%s])) * 0)) / clamp_min(sum(rate(%s[%s])), 1e-12)) OR absent(sum(rate(%s[%s])))",
		errorQuery, window, totalQuery, window, totalQuery, window, totalQuery, window,
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

// burnRateExprComposition builds the burn rate expression for a composition,
// referencing slok:sli_error_composition_rate and slok:error_budget_target_composition.
func burnRateExprComposition(compositionName, compositionNamespace, window string) string {
	selector := fmt.Sprintf(`slo_composition_name="%s", slo_composition_namespace="%s"`, compositionName, compositionNamespace)
	return fmt.Sprintf(
		"slok:sli_error_composition_rate:%s{%s} / on (slo_composition_name, slo_composition_namespace) slok:error_budget_target_composition{%s}",
		window, selector, selector,
	)
}

// burnRateAlertExprComposition builds the multi-window burn rate alert expression for a composition.
func burnRateAlertExprComposition(compositionName, compositionNamespace string, preset burnRatePreset) string {
	selector := fmt.Sprintf(`slo_composition_name="%s", slo_composition_namespace="%s"`, compositionName, compositionNamespace)
	return fmt.Sprintf(
		"slok:burn_rate_composition:%s{%s} > %g AND slok:burn_rate_composition:%s{%s} > %g",
		preset.ShortWindow, selector, preset.BurnRate,
		preset.LongWindow, selector, preset.BurnRate,
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

func CreateAggregatedPrometheusRule(sloCompositionName, sloCompositionNamespace string, spec observabilityv1alpha1.SLOCompositionSpec, slos []observabilityv1alpha1.ServiceLevelObjective) (monitoringv1.PrometheusRule, error) {
	switch spec.Composition.Type {
	case "AND_MIN":
		prometheusRule := monitoringv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("slok-%s-%s-aggregated", sloCompositionName, sloCompositionNamespace),
				Namespace: sloCompositionNamespace,
				Labels: map[string]string{
					"release":                 "prometheus",
					"slok.io/slo_composition": sloCompositionName,
				},
			},
			Spec: monitoringv1.PrometheusRuleSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: fmt.Sprintf("slok-%s-aggregated", sloCompositionName),
					},
				},
			},
		}

		rules := &prometheusRule.Spec.Groups[0].Rules

		// Build regex selector: "slo1/objectiveName|slo2/objectiveName"
		ids := make([]string, 0, len(slos))
		for _, slo := range slos {
			ids = append(ids, fmt.Sprintf("%s/%s", slo.Name, slo.Spec.Objective.Name))
		}
		objectiveIDSelector := strings.Join(ids, "|")

		// SLI error composition rate recording rules (one per window)
		for _, window := range recordingWindows {
			expr := fmt.Sprintf(
				`max by () (slok:sli_error_rate:%s{objective_id=~"%s"})`,
				window, objectiveIDSelector,
			)
			*rules = append(*rules, monitoringv1.Rule{
				Record: fmt.Sprintf("slok:sli_error_composition_rate:%s", window),
				Expr:   intstr.FromString(expr),
				Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, window),
			})
		}

		// Objective target constant
		*rules = append(*rules, monitoringv1.Rule{
			Record: "slok:objective_target_composition",
			Expr:   intstr.FromString(fmt.Sprintf("vector(%g)", spec.Target/100)),
			Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, "30d"),
		})

		// Error budget target constant (1 - target)
		*rules = append(*rules, monitoringv1.Rule{
			Record: "slok:error_budget_target_composition",
			Expr:   intstr.FromString(fmt.Sprintf("vector(%g)", 1-spec.Target/100)),
			Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, "30d"),
		})

		// Burn rate recording rules (one per window)
		for _, window := range recordingWindows {
			*rules = append(*rules, monitoringv1.Rule{
				Record: fmt.Sprintf("slok:burn_rate_composition:%s", window),
				Expr:   intstr.FromString(burnRateExprComposition(sloCompositionName, sloCompositionNamespace, window)),
				Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, window),
			})
		}

		// Burn rate alerts
		if spec.Alerting != nil && spec.Alerting.BurnRateAlerts != nil && spec.Alerting.BurnRateAlerts.Enabled {
			alertGroup := monitoringv1.RuleGroup{
				Name: fmt.Sprintf("slok-%s-aggregated-burnRateAlerts", sloCompositionName),
			}

			for _, preset := range defaultBurnRatePresets {
				alertGroup.Rules = append(alertGroup.Rules, monitoringv1.Rule{
					Alert:  fmt.Sprintf("Composition: %s SLOBurnRateHigh - %s", sloCompositionName, preset.AlertSuffix),
					Expr:   intstr.FromString(burnRateAlertExprComposition(sloCompositionName, sloCompositionNamespace, preset)),
					Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, preset.ShortWindow),
				})
				alertGroup.Rules[len(alertGroup.Rules)-1].Labels["severity"] = preset.Severity
			}

			// Error budget exhausted alert
			selector := fmt.Sprintf(`slo_composition_name="%s", slo_composition_namespace="%s"`, sloCompositionName, sloCompositionNamespace)
			alertGroup.Rules = append(alertGroup.Rules, monitoringv1.Rule{
				Alert: fmt.Sprintf("Composition: %s ErrorBudget Finished - Violated", sloCompositionName),
				Expr: intstr.FromString(fmt.Sprintf(
					"slok:sli_error_composition_rate:%s{%s} > slok:error_budget_target_composition{%s}",
					spec.Window, selector, selector,
				)),
				Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, spec.Window),
			})
			alertGroup.Rules[len(alertGroup.Rules)-1].Labels["severity"] = severityWarning

			prometheusRule.Spec.Groups = append(prometheusRule.Spec.Groups, alertGroup)
		}

		return prometheusRule, nil
	case "WEIGHTED_ROUTES":
		prometheusRule := monitoringv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("slok-%s-%s-aggregated", sloCompositionName, sloCompositionNamespace),
				Namespace: sloCompositionNamespace,
				Labels: map[string]string{
					"release":                 "prometheus",
					"slok.io/slo_composition": sloCompositionName,
				},
			},
			Spec: monitoringv1.PrometheusRuleSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: fmt.Sprintf("slok-%s-aggregated", sloCompositionName),
					},
				},
			},
		}

		rules := &prometheusRule.Spec.Groups[0].Rules

		// Build mapping: component alias → (sloName, sloNamespace, objectiveName).
		// The controller appends slos in the same order as spec.Objectives.
		type sloInfo struct {
			name      string
			namespace string
			objective string
		}
		aliasToSLO := make(map[string]sloInfo, len(spec.Objectives))
		for i, obj := range spec.Objectives {
			if i < len(slos) {
				aliasToSLO[obj.Name] = sloInfo{
					name:      slos[i].Name,
					namespace: slos[i].Namespace,
					objective: slos[i].Spec.Objective.Name,
				}
			}
		}

		if spec.Composition.Params == nil {
			return monitoringv1.PrometheusRule{}, fmt.Errorf("params is required for WEIGHTED_ROUTES composition")
		}
		routes := spec.Composition.Params.Routes

		// SLI error composition rate recording rules (one per window).
		//
		// Formula (sequential chain = product of success rates):
		//   e_total = 1 - sum_i( weight_i * prod_j(1 - e_j) )
		//
		// PromQL equivalent per window:
		//   1 - (
		//     route1.weight * ((1-scalar(e_a)) * (1-scalar(e_b)))
		//     + route2.weight * ((1-scalar(e_a)) * (1-scalar(e_c)))
		//     + ...
		//   )
		for _, window := range recordingWindows {
			routeExprs := make([]string, 0, len(routes))
			for _, route := range routes {
				chainParts := make([]string, 0, len(route.Chain))
				for _, alias := range route.Chain {
					info, ok := aliasToSLO[alias]
					if !ok {
						return monitoringv1.PrometheusRule{}, fmt.Errorf("component alias %q not found in objectives", alias)
					}
					chainParts = append(chainParts, fmt.Sprintf(
						`(1 - scalar(slok:sli_error_rate:%s{slo_name="%s",slo_namespace="%s",objective_name="%s"}))`,
						window, info.name, info.namespace, info.objective,
					))
				}
				routeExprs = append(routeExprs, fmt.Sprintf(
					"%g * (%s)",
					route.Weight, strings.Join(chainParts, " * "),
				))
			}
			expr := fmt.Sprintf("1 - (\n  %s\n)", strings.Join(routeExprs, "\n  + "))
			*rules = append(*rules, monitoringv1.Rule{
				Record: fmt.Sprintf("slok:sli_error_composition_rate:%s", window),
				Expr:   intstr.FromString(expr),
				Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, window),
			})
		}

		// Objective target constant
		*rules = append(*rules, monitoringv1.Rule{
			Record: "slok:objective_target_composition",
			Expr:   intstr.FromString(fmt.Sprintf("vector(%g)", spec.Target/100)),
			Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, "30d"),
		})

		// Error budget target constant (1 - target)
		*rules = append(*rules, monitoringv1.Rule{
			Record: "slok:error_budget_target_composition",
			Expr:   intstr.FromString(fmt.Sprintf("vector(%g)", 1-spec.Target/100)),
			Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, "30d"),
		})

		// Burn rate recording rules (one per window)
		for _, window := range recordingWindows {
			*rules = append(*rules, monitoringv1.Rule{
				Record: fmt.Sprintf("slok:burn_rate_composition:%s", window),
				Expr:   intstr.FromString(burnRateExprComposition(sloCompositionName, sloCompositionNamespace, window)),
				Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, window),
			})
		}

		// Burn rate alerts (stessa struttura di AND_MIN)
		if spec.Alerting != nil && spec.Alerting.BurnRateAlerts != nil && spec.Alerting.BurnRateAlerts.Enabled {
			alertGroup := monitoringv1.RuleGroup{
				Name: fmt.Sprintf("slok-%s-aggregated-burnRateAlerts", sloCompositionName),
			}
			for _, preset := range defaultBurnRatePresets {
				alertGroup.Rules = append(alertGroup.Rules, monitoringv1.Rule{
					Alert:  fmt.Sprintf("Composition: %s SLOBurnRateHigh - %s", sloCompositionName, preset.AlertSuffix),
					Expr:   intstr.FromString(burnRateAlertExprComposition(sloCompositionName, sloCompositionNamespace, preset)),
					Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, preset.ShortWindow),
				})
				alertGroup.Rules[len(alertGroup.Rules)-1].Labels["severity"] = preset.Severity
			}
			selector := fmt.Sprintf(`slo_composition_name="%s", slo_composition_namespace="%s"`, sloCompositionName, sloCompositionNamespace)
			alertGroup.Rules = append(alertGroup.Rules, monitoringv1.Rule{
				Alert: fmt.Sprintf("Composition: %s ErrorBudget Finished - Violated", sloCompositionName),
				Expr: intstr.FromString(fmt.Sprintf(
					"slok:sli_error_composition_rate:%s{%s} > slok:error_budget_target_composition{%s}",
					spec.Window, selector, selector,
				)),
				Labels: baseLabelsComposition(sloCompositionName, sloCompositionNamespace, spec.Window),
			})
			alertGroup.Rules[len(alertGroup.Rules)-1].Labels["severity"] = severityWarning
			prometheusRule.Spec.Groups = append(prometheusRule.Spec.Groups, alertGroup)
		}

		return prometheusRule, nil
	default:
		return monitoringv1.PrometheusRule{}, fmt.Errorf("unsupported composition type: %s", spec.Composition.Type)
	}
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
	if objective.Alerting != nil && objective.Alerting.BudgetErrorAlerts != nil && objective.Alerting.BudgetErrorAlerts.Enabled {
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
				Labels: map[string]string{"severity": severityWarning},
			},
			monitoringv1.Rule{
				Alert: "SLOObjectiveViolatedWarning",
				Expr: intstr.FromString(fmt.Sprintf(
					"optimization_request_objective_percent_remaining{%s} <= 0",
					budgetSelector,
				)),
				For:    monitoringv1.DurationPointer("5m"),
				Labels: map[string]string{"severity": severityWarning},
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

	// Burn rate alerts using defaults or custom objective config.
	if objective.Alerting != nil && objective.Alerting.BurnRateAlerts != nil && objective.Alerting.BurnRateAlerts.Enabled {
		alertGroup := monitoringv1.RuleGroup{
			Name: fmt.Sprintf("slok.%s.%s-burnRateAlerts", sloName, objectiveName),
		}

		presets, err := burnRatePresetsForObjective(objective)
		if err != nil {
			return monitoringv1.PrometheusRule{}, err
		}

		for _, preset := range presets {
			alertGroup.Rules = append(alertGroup.Rules, monitoringv1.Rule{
				Alert:  fmt.Sprintf("Objective: %s SLOBurnRateHigh - %s", objectiveName, preset.AlertSuffix),
				Expr:   intstr.FromString(burnRateAlertExpr(sloName, sloNamespace, objectiveName, preset)),
				For:    monitoringv1.DurationPointer(preset.For),
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
		alertGroup.Rules[len(alertGroup.Rules)-1].Labels["severity"] = severityWarning

		prometheusRule.Spec.Groups = append(prometheusRule.Spec.Groups, alertGroup)
	}

	return prometheusRule, nil
}
