package prometheus

import (
	"strings"
	"testing"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeSLO(name, namespace, objectiveName string) observabilityv1alpha1.ServiceLevelObjective {
	return observabilityv1alpha1.ServiceLevelObjective{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: observabilityv1alpha1.ServiceLevelObjectiveSpec{
			DisplayName: name,
			Objective: observabilityv1alpha1.Objective{
				Name:   objectiveName,
				Target: 99.9,
				Window: "30d",
				Sli:    observabilityv1alpha1.SLI{},
			},
		},
	}
}

func TestCreateAggregatedPrometheusRule_AND_MIN(t *testing.T) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("payment-api", "production", "availability"),
		makeSLO("inventory-api", "production", "availability"),
	}

	rule, err := CreateAggregatedPrometheusRule("checkout-flow", "production", "AND_MIN", slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Metadata
	if rule.Name != "slok-checkout-flow-production-aggregated" {
		t.Errorf("name: got %q, want %q", rule.Name, "slok-checkout-flow-production-aggregated")
	}
	if rule.Namespace != "production" {
		t.Errorf("namespace: got %q, want %q", rule.Namespace, "production")
	}
	if rule.Labels["slok.io/slo_composition"] != "checkout-flow" {
		t.Errorf("label slok.io/slo_composition: got %q, want %q", rule.Labels["slok.io/slo_composition"], "checkout-flow")
	}

	// One group
	if len(rule.Spec.Groups) != 1 {
		t.Fatalf("groups: got %d, want 1", len(rule.Spec.Groups))
	}
	if rule.Spec.Groups[0].Name != "slok-checkout-flow-aggregated" {
		t.Errorf("group name: got %q, want %q", rule.Spec.Groups[0].Name, "slok-checkout-flow-aggregated")
	}

	// One recording rule per window
	rules := rule.Spec.Groups[0].Rules
	if len(rules) != len(recordingWindows) {
		t.Fatalf("rules count: got %d, want %d", len(rules), len(recordingWindows))
	}

	for i, window := range recordingWindows {
		r := rules[i]

		// Record name
		expectedRecord := "slok:sli_error_rate:" + window
		if r.Record != expectedRecord {
			t.Errorf("rule[%d] record: got %q, want %q", i, r.Record, expectedRecord)
		}

		expr := r.Expr.String()

		// Uses max by ()
		if !strings.HasPrefix(expr, "max by ()") {
			t.Errorf("rule[%d] expr should start with 'max by ()': %s", i, expr)
		}
		// References the correct window
		if !strings.Contains(expr, window) {
			t.Errorf("rule[%d] expr missing window %q: %s", i, window, expr)
		}
		// Contains both objective IDs
		if !strings.Contains(expr, "payment-api/availability") {
			t.Errorf("rule[%d] expr missing 'payment-api/availability': %s", i, expr)
		}
		if !strings.Contains(expr, "inventory-api/availability") {
			t.Errorf("rule[%d] expr missing 'inventory-api/availability': %s", i, expr)
		}
		// Uses regex match
		if !strings.Contains(expr, `objective_id=~`) {
			t.Errorf("rule[%d] expr should use regex match (=~): %s", i, expr)
		}

		// Labels
		if r.Labels["slo_composition_name"] != "checkout-flow" {
			t.Errorf("rule[%d] label slo_composition_name: got %q, want %q", i, r.Labels["slo_composition_name"], "checkout-flow")
		}
		if r.Labels["slo_composition_namespace"] != "production" {
			t.Errorf("rule[%d] label slo_composition_namespace: got %q, want %q", i, r.Labels["slo_composition_namespace"], "production")
		}
		if r.Labels["slok_window"] != window {
			t.Errorf("rule[%d] label slok_window: got %q, want %q", i, r.Labels["slok_window"], window)
		}
	}
}

func TestCreateAggregatedPrometheusRule_SingleSLO(t *testing.T) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("api-gateway", "default", "latency"),
	}

	rule, err := CreateAggregatedPrometheusRule("single", "default", "AND_MIN", slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expr := rule.Spec.Groups[0].Rules[0].Expr.String()
	if !strings.Contains(expr, "api-gateway/latency") {
		t.Errorf("expr missing 'api-gateway/latency': %s", expr)
	}
}

func TestCreateAggregatedPrometheusRule_UnsupportedType(t *testing.T) {
	_, err := CreateAggregatedPrometheusRule("my-composition", "default", "OR_MAX", nil)
	if err == nil {
		t.Fatal("expected error for unsupported composition type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported composition type") {
		t.Errorf("unexpected error message: %v", err)
	}
}
