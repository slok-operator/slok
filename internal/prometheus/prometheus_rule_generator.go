package prometheus

import (
	"fmt"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreatePrometheusRule(sloName string, sloNamespace string, objectiveName string, budgetAlert []observabilityv1alpha1.BudgetAlert, burnRateAlerts []observabilityv1alpha1.BurnRateAlert) (monitoringv1.PrometheusRule, error) {
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
	if len(budgetAlert) == 0 {
		prometheusRule.Spec.Groups[0].Rules = []monitoringv1.Rule{
			{
				Alert: "SLOObjectiveAtRisk",
				Expr:  intstr.FromString(fmt.Sprintf("optimization_request_objective_percent_remaining{namespace=\"%s\", service_level_objective=\"%s\", objective_name=\"%s\"} > 0 and optimization_request_objective_percent_remaining{namespace=\"%s\", service_level_objective=\"%s\", objective_name=\"%s\"} < 10", sloNamespace, sloName, objectiveName, sloNamespace, sloName, objectiveName)),
				For:   monitoringv1.DurationPointer("3m"),
				Labels: map[string]string{
					"severity": "warning",
				},
			},
			{
				Alert: "SLOObjectiveViolated",
				Expr:  intstr.FromString(fmt.Sprintf("optimization_request_objective_percent_remaining{namespace=\"%s\", service_level_objective=\"%s\", objective_name=\"%s\"} <= 0", sloNamespace, sloName, objectiveName)),
				For:   monitoringv1.DurationPointer("1m"),
				Labels: map[string]string{
					"severity": "critical",
				},
			},
		}
	} else {
		for _, threshold := range budgetAlert {
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
	// BURN RATE ALERTS TO BE IMPLEMENTED
	return prometheusRule, nil
}
