package prometheus

import (
	"fmt"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
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
	{ShortWindow: "5m", LongWindow: "1h", BurnRate: 14, Severity: "critical", AlertSuffix: "Outage", For: "2m"},
	{ShortWindow: "1h", LongWindow: "6h", BurnRate: 6, Severity: "critical", AlertSuffix: "High", For: "15m"},
	{ShortWindow: "6h", LongWindow: "3d", BurnRate: 1, Severity: "warning", AlertSuffix: "Erosion", For: "1h"},
	{ShortWindow: "7d", LongWindow: "30d", BurnRate: 0.5, Severity: "warning", AlertSuffix: "Slow", For: "3h"},
}

func CreatePrometheusRule(sloName string, sloNamespace string, objective observabilityv1alpha1.Objective) (monitoringv1.PrometheusRule, error) {
	objectiveName := objective.Name
	prometheusRule := monitoringv1.PrometheusRule{}
	prometheusRule.ObjectMeta = metav1.ObjectMeta{
		Name:      fmt.Sprintf("slok-%s-%s", sloName, objectiveName),
		Namespace: sloNamespace,
		Labels: map[string]string{
			"release":           "prometheus",
			"slok.io/slo":       sloName,
			"slok.io/objective": objectiveName,
		},
	}
	prometheusRule.Spec.Groups = append(prometheusRule.Spec.Groups, monitoringv1.RuleGroup{
		Name: fmt.Sprintf("slok.%s.%s", sloName, objectiveName),
	})


	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:sli_error_rate:5m"),
		Expr: intstr.FromString(fmt.Sprintf("(sum(rate(%s[5m])) OR (sum(rate(%s[5m])) * 0)) / clamp_min(sum(rate(%s[5m])), 1e-12)", objective.Sli.Query.ErrorQuery, objective.Sli.Query.TotalQuery, objective.Sli.Query.TotalQuery)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "5m",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:sli_error_rate:1h"),
		Expr: intstr.FromString(fmt.Sprintf("(sum(rate(%s[1h])) OR (sum(rate(%s[1h])) * 0)) / clamp_min(sum(rate(%s[1h])), 1e-12)", objective.Sli.Query.ErrorQuery, objective.Sli.Query.TotalQuery, objective.Sli.Query.TotalQuery)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "1h",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:sli_error_rate:6h"),
		Expr: intstr.FromString(fmt.Sprintf("(sum(rate(%s[6h])) OR (sum(rate(%s[6h])) * 0)) / clamp_min(sum(rate(%s[6h])), 1e-12)", objective.Sli.Query.ErrorQuery, objective.Sli.Query.TotalQuery, objective.Sli.Query.TotalQuery)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "6h",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:sli_error_rate:3d"),
		Expr: intstr.FromString(fmt.Sprintf("(sum(rate(%s[3d])) OR (sum(rate(%s[3d])) * 0)) / clamp_min(sum(rate(%s[3d])), 1e-12)", objective.Sli.Query.ErrorQuery, objective.Sli.Query.TotalQuery, objective.Sli.Query.TotalQuery)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "3d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:sli_error_rate:7d"),
		Expr: intstr.FromString(fmt.Sprintf("(sum(rate(%s[7d])) OR (sum(rate(%s[7d])) * 0)) / clamp_min(sum(rate(%s[7d])), 1e-12)", objective.Sli.Query.ErrorQuery, objective.Sli.Query.TotalQuery, objective.Sli.Query.TotalQuery)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "7d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:sli_error_rate:30d"),
		Expr: intstr.FromString(fmt.Sprintf("(sum(rate(%s[30d])) OR (sum(rate(%s[30d])) * 0)) / clamp_min(sum(rate(%s[30d])), 1e-12)", objective.Sli.Query.ErrorQuery, objective.Sli.Query.TotalQuery, objective.Sli.Query.TotalQuery)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "30d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:objective_target"),
		Expr: intstr.FromString(fmt.Sprintf("vector(%g)", objective.Target/100)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "30d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:error_budget_target"),
		Expr: intstr.FromString(fmt.Sprintf("vector(%g)", 1-objective.Target/100)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "30d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:error_budget_target"),
		Expr: intstr.FromString(fmt.Sprintf("vector(%g)", 1-objective.Target/100)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "30d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:burn_rate:5m"),
		Expr: intstr.FromString(fmt.Sprintf("slok:sli_error_rate:5m{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} / on (slo_name, slo_namespace, objective_name) slok:error_budget_target{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"}", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "5m",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:burn_rate:1h"),
		Expr: intstr.FromString(fmt.Sprintf("slok:sli_error_rate:1h{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} / on (slo_name, slo_namespace, objective_name) slok:error_budget_target{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"}", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "1h",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:burn_rate:6h"),
		Expr: intstr.FromString(fmt.Sprintf("slok:sli_error_rate:6h{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} / on (slo_name, slo_namespace, objective_name) slok:error_budget_target{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"}", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "6h",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:burn_rate:3d"),
		Expr: intstr.FromString(fmt.Sprintf("slok:sli_error_rate:3d{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} / on (slo_name, slo_namespace, objective_name) slok:error_budget_target{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"}", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "3d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:burn_rate:7d"),
		Expr: intstr.FromString(fmt.Sprintf("slok:sli_error_rate:7d{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} / on (slo_name, slo_namespace, objective_name) slok:error_budget_target{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"}", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "7d",
		},
	})

	prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
		Record: fmt.Sprint("slok:burn_rate:30d"),
		Expr: intstr.FromString(fmt.Sprintf("slok:sli_error_rate:30d{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} / on (slo_name, slo_namespace, objective_name) slok:error_budget_target{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"}", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
		Labels: map[string]string{
			"slo_name":       sloName,
			"slo_namespace":  sloNamespace,
			"objective_name": objectiveName,
			"slok_window":     "30d",
		},
	})

	if objective.Alerting.BudgetErrorAlerts.Enabled {
		// Creation of default budget alerts
		prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
			Alert: "SLOObjectiveAtRisk",
			Expr:  intstr.FromString(fmt.Sprintf("optimization_request_objective_percent_remaining{namespace=\"%s\", service_level_objective=\"%s\", objective_name=\"%s\"} > 0 and optimization_request_objective_percent_remaining{namespace=\"%s\", service_level_objective=\"%s\", objective_name=\"%s\"} < 10", sloNamespace, sloName, objectiveName, sloNamespace, sloName, objectiveName)),
			For:   monitoringv1.DurationPointer("3m"),
			Labels: map[string]string{
				"severity": "warning",
			},
		})
		prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
			Alert: "SLOObjectiveViolatedWarning",
			Expr:  intstr.FromString(fmt.Sprintf("optimization_request_objective_percent_remaining{namespace=\"%s\", service_level_objective=\"%s\", objective_name=\"%s\"} <= 0", sloNamespace, sloName, objectiveName)),
			For:   monitoringv1.DurationPointer("5m"),
			Labels: map[string]string{
				"severity": "warning",
			},
		})

		// Custom budget alerts from the SLO spec
		for _, threshold := range objective.Alerting.BudgetErrorAlerts.Alerts {
			prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
				Alert: threshold.Name,
				Expr:  intstr.FromString(fmt.Sprintf("optimization_request_objective_percent_remaining{namespace=\"%s\", service_level_objective=\"%s\", objective_name=\"%s\"} < %.2f", sloNamespace, sloName, objectiveName, threshold.Percent)),
				For:   monitoringv1.DurationPointer("1m"),
				Labels: map[string]string{
					"severity": threshold.Severity,
				},
			})
		}
	}

	if objective.Alerting.BurnRateAlerts.Enabled {
		prometheusRule.Spec.Groups = append(prometheusRule.Spec.Groups, monitoringv1.RuleGroup{
			Name: fmt.Sprintf("slok.%s.%s-burnRateAlerts", sloName, objectiveName),
		})
		prometheusRule.Spec.Groups[1].Rules = append(prometheusRule.Spec.Groups[1].Rules, monitoringv1.Rule{
			Alert: fmt.Sprintf("Objective: %s SLOBurnRateHigh - Critical", objectiveName),
			Expr: intstr.FromString(fmt.Sprintf("slok:burn_rate:5m{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} > 14 AND slok:burn_rate:1h{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} > 14", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
			Labels: map[string]string{
				"slo_name":       sloName,
				"slo_namespace":  sloNamespace,
				"objective_name": objectiveName,
				"slok_window":     "5m",
				"severity":      "critical",
			},
		})

		prometheusRule.Spec.Groups[1].Rules = append(prometheusRule.Spec.Groups[1].Rules, monitoringv1.Rule{
			Alert: fmt.Sprintf("Objective: %s SLOBurnRateHigh - Degraded", objectiveName),
			Expr: intstr.FromString(fmt.Sprintf("slok:burn_rate:1h{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} > 6 AND slok:burn_rate:6h{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} > 6", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
			Labels: map[string]string{
				"slo_name":       sloName,
				"slo_namespace":  sloNamespace,
				"objective_name": objectiveName,
				"slok_window":     "6h",
				"severity":      "warning",
			},
		})

		prometheusRule.Spec.Groups[1].Rules = append(prometheusRule.Spec.Groups[1].Rules, monitoringv1.Rule{
			Alert: fmt.Sprintf("Objective: %s SLOBurnRateHigh - Warningn", objectiveName),
			Expr: intstr.FromString(fmt.Sprintf("slok:burn_rate:6h{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} > 1 AND slok:burn_rate:3d{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} > 1", sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
			Labels: map[string]string{
				"slo_name":       sloName,
				"slo_namespace":  sloNamespace,
				"objective_name": objectiveName,
				"slok_window":     "3d",
				"severity":      "warning",
			},
		})

		prometheusRule.Spec.Groups[1].Rules = append(prometheusRule.Spec.Groups[1].Rules, monitoringv1.Rule{
			Alert: fmt.Sprintf("Objective: %s ErrorBudget Finished - Violated", objectiveName),
			Expr: intstr.FromString(fmt.Sprintf("slok:sli_error_rate:%s{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"} > slok:error_budget_target{slo_name=\"%s\", slo_namespace=\"%s\", objective_name=\"%s\"}", objective.Window, sloName, sloNamespace, objectiveName, sloName, sloNamespace, objectiveName)),
			Labels: map[string]string{
				"slo_name":       sloName,
				"slo_namespace":  sloNamespace,
				"objective_name": objectiveName,
				"slok_window":     objective.Window,
				"severity":      "warning",
			},
		})
	}
	return prometheusRule, nil
}
