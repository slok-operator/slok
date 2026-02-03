package prometheus

import (
	"fmt"
	"strings"

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
		// Creation of default burn rate alerts based on presets
		for _, preset := range defaultBurnRatePresets {
			successLong := strings.ReplaceAll(objective.Sli.Query.Success, "$window", preset.LongWindow)
			totalLong := strings.ReplaceAll(objective.Sli.Query.Total, "$window", preset.LongWindow)
			successShort := strings.ReplaceAll(objective.Sli.Query.Success, "$window", preset.ShortWindow)
			totalShort := strings.ReplaceAll(objective.Sli.Query.Total, "$window", preset.ShortWindow)

			expr := fmt.Sprintf(
				"(1 - (%s / %s)) / (1 - (%g / 100)) > %g AND (1 - (%s / %s)) / (1 - (%g / 100)) > %g",
				successLong, totalLong, objective.Target, preset.BurnRate,
				successShort, totalShort, objective.Target, preset.BurnRate,
			)

			prometheusRule.Spec.Groups[0].Rules = append(prometheusRule.Spec.Groups[0].Rules, monitoringv1.Rule{
				Alert: fmt.Sprintf("SLOBurnRate%s", preset.AlertSuffix),
				Expr:  intstr.FromString(expr),
				For:   monitoringv1.DurationPointer(preset.For),
				Labels: map[string]string{
					"severity":  preset.Severity,
					"slo":       sloName,
					"objective": objectiveName,
				},
				Annotations: map[string]string{
					"summary": fmt.Sprintf(
						"%s/%s: burn rate >%gx over %s/%s window",
						sloName, objectiveName, preset.BurnRate, preset.LongWindow, preset.ShortWindow,
					),
				},
			})
		}
	}
	return prometheusRule, nil
}
