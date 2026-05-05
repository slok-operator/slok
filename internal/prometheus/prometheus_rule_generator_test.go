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

func makeCompositionSpec(compositionType string, alerting *observabilityv1alpha1.Alerting) observabilityv1alpha1.SLOCompositionSpec {
	return observabilityv1alpha1.SLOCompositionSpec{
		Target:      99.9,
		Window:      "30d",
		Composition: observabilityv1alpha1.Composition{Type: compositionType},
		Alerting:    alerting,
	}
}

func TestCreateAggregatedPrometheusRule_AND_MIN(t *testing.T) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("payment-api", "production", "availability"),
		makeSLO("inventory-api", "production", "availability"),
	}
	spec := makeCompositionSpec("AND_MIN", nil)

	rule, err := CreateAggregatedPrometheusRule("checkout-flow", "production", spec, slos)
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

	// One group (no alerting)
	if len(rule.Spec.Groups) != 1 {
		t.Fatalf("groups: got %d, want 1", len(rule.Spec.Groups))
	}
	if rule.Spec.Groups[0].Name != "slok-checkout-flow-aggregated" {
		t.Errorf("group name: got %q, want %q", rule.Spec.Groups[0].Name, "slok-checkout-flow-aggregated")
	}

	// Expected rules: len(recordingWindows) sli_error_composition_rate
	//               + 1 objective_target + 1 error_budget_target
	//               + len(recordingWindows) burn_rate
	expectedRules := len(recordingWindows)*2 + 2
	rules := rule.Spec.Groups[0].Rules
	if len(rules) != expectedRules {
		t.Fatalf("rules count: got %d, want %d", len(rules), expectedRules)
	}

	// First len(recordingWindows) rules: slok:sli_error_composition_rate:{window}
	for i, window := range recordingWindows {
		r := rules[i]

		expectedRecord := "slok:sli_error_composition_rate:" + window
		if r.Record != expectedRecord {
			t.Errorf("rule[%d] record: got %q, want %q", i, r.Record, expectedRecord)
		}

		expr := r.Expr.String()

		if !strings.HasPrefix(expr, "max by ()") {
			t.Errorf("rule[%d] expr should start with 'max by ()': %s", i, expr)
		}
		if !strings.Contains(expr, window) {
			t.Errorf("rule[%d] expr missing window %q: %s", i, window, expr)
		}
		if !strings.Contains(expr, "payment-api/availability") {
			t.Errorf("rule[%d] expr missing 'payment-api/availability': %s", i, expr)
		}
		if !strings.Contains(expr, "inventory-api/availability") {
			t.Errorf("rule[%d] expr missing 'inventory-api/availability': %s", i, expr)
		}
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

	// objective_target_composition rule
	objTargetRule := rules[len(recordingWindows)]
	if objTargetRule.Record != "slok:objective_target_composition" {
		t.Errorf("objective_target record: got %q, want %q", objTargetRule.Record, "slok:objective_target_composition")
	}
	if !strings.Contains(objTargetRule.Expr.String(), "vector(") {
		t.Errorf("objective_target expr should be a vector: %s", objTargetRule.Expr.String())
	}

	// error_budget_target_composition rule
	budgetTargetRule := rules[len(recordingWindows)+1]
	if budgetTargetRule.Record != "slok:error_budget_target_composition" {
		t.Errorf("error_budget_target record: got %q, want %q", budgetTargetRule.Record, "slok:error_budget_target_composition")
	}

	// burn_rate_composition rules
	burnRateOffset := len(recordingWindows) + 2
	for i, window := range recordingWindows {
		r := rules[burnRateOffset+i]
		expectedRecord := "slok:burn_rate_composition:" + window
		if r.Record != expectedRecord {
			t.Errorf("burn_rate rule[%d] record: got %q, want %q", i, r.Record, expectedRecord)
		}
		if !strings.Contains(r.Expr.String(), "slok:sli_error_composition_rate:"+window) {
			t.Errorf("burn_rate rule[%d] expr missing sli_error_composition_rate:%s: %s", i, window, r.Expr.String())
		}
	}
}

func TestCreateAggregatedPrometheusRule_WithBurnRateAlerts(t *testing.T) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("payment-api", "production", "availability"),
	}
	spec := makeCompositionSpec("AND_MIN", &observabilityv1alpha1.Alerting{
		BurnRateAlerts: &observabilityv1alpha1.BurnRates{Enabled: true},
	})

	rule, err := CreateAggregatedPrometheusRule("checkout-flow", "production", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two groups: recording + alert
	if len(rule.Spec.Groups) != 2 {
		t.Fatalf("groups: got %d, want 2", len(rule.Spec.Groups))
	}

	alertGroup := rule.Spec.Groups[1]
	if alertGroup.Name != "slok-checkout-flow-aggregated-burnRateAlerts" {
		t.Errorf("alert group name: got %q, want %q", alertGroup.Name, "slok-checkout-flow-aggregated-burnRateAlerts")
	}

	// len(defaultBurnRatePresets) preset alerts + 1 budget exhausted alert
	expectedAlerts := len(defaultBurnRatePresets) + 1
	if len(alertGroup.Rules) != expectedAlerts {
		t.Fatalf("alert rules count: got %d, want %d", len(alertGroup.Rules), expectedAlerts)
	}

	// Each preset alert references slok:burn_rate_composition
	for i, preset := range defaultBurnRatePresets {
		expr := alertGroup.Rules[i].Expr.String()
		if !strings.Contains(expr, "slok:burn_rate_composition:"+preset.ShortWindow) {
			t.Errorf("alert[%d] expr missing burn_rate_composition:%s: %s", i, preset.ShortWindow, expr)
		}
		if alertGroup.Rules[i].Labels["severity"] != preset.Severity {
			t.Errorf("alert[%d] severity: got %q, want %q", i, alertGroup.Rules[i].Labels["severity"], preset.Severity)
		}
	}

	// Budget exhausted alert references slok:sli_error_composition_rate and the composition window
	lastAlert := alertGroup.Rules[len(alertGroup.Rules)-1]
	if !strings.Contains(lastAlert.Expr.String(), "slok:sli_error_composition_rate:30d") {
		t.Errorf("budget alert expr missing sli_error_composition_rate:30d: %s", lastAlert.Expr.String())
	}
	if lastAlert.Labels["severity"] != severityWarning {
		t.Errorf("budget alert severity: got %q, want warning", lastAlert.Labels["severity"])
	}
}

func TestCreateAggregatedPrometheusRule_SingleSLO(t *testing.T) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("api-gateway", "default", "latency"),
	}
	spec := makeCompositionSpec("AND_MIN", nil)

	rule, err := CreateAggregatedPrometheusRule("single", "default", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expr := rule.Spec.Groups[0].Rules[0].Expr.String()
	if !strings.Contains(expr, "api-gateway/latency") {
		t.Errorf("expr missing 'api-gateway/latency': %s", expr)
	}
}

func TestCreateAggregatedPrometheusRule_UnsupportedType(t *testing.T) {
	spec := makeCompositionSpec("OR_MAX", nil)
	_, err := CreateAggregatedPrometheusRule("my-composition", "default", spec, nil)
	if err == nil {
		t.Fatal("expected error for unsupported composition type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported composition type") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- WEIGHTED_ROUTES helpers ---

func makeSLORef(alias, sloName, namespace string) observabilityv1alpha1.SLORef {
	return observabilityv1alpha1.SLORef{
		Name: alias,
		Ref:  observabilityv1alpha1.SLOObjective{Name: sloName, Namespace: namespace},
	}
}

func makeWeightedSpec(window string, objectives []observabilityv1alpha1.SLORef, routes []observabilityv1alpha1.Route, alerting *observabilityv1alpha1.Alerting) observabilityv1alpha1.SLOCompositionSpec {
	return observabilityv1alpha1.SLOCompositionSpec{
		Target:     99.9,
		Window:     window,
		Objectives: objectives,
		Composition: observabilityv1alpha1.Composition{
			Type: "WEIGHTED_ROUTES",
			Params: &observabilityv1alpha1.CompositionParams{
				Routes: routes,
			},
		},
		Alerting: alerting,
	}
}

// checkoutWeightedFixture returns the SLOs and spec for the canonical checkout example:
//
//	route no-coupon  (weight 0.9): base → payments
//	route with-coupon (weight 0.1): base → coupon → payments
func checkoutWeightedFixture(alerting *observabilityv1alpha1.Alerting) (
	[]observabilityv1alpha1.ServiceLevelObjective,
	observabilityv1alpha1.SLOCompositionSpec,
) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("checkout-base-slo", "app", "availability"),
		makeSLO("payments-slo", "app", "availability"),
		makeSLO("coupon-slo", "app", "availability"),
	}
	objectives := []observabilityv1alpha1.SLORef{
		makeSLORef("base", "checkout-base-slo", "app"),
		makeSLORef("payments", "payments-slo", "app"),
		makeSLORef("coupon", "coupon-slo", "app"),
	}
	routes := []observabilityv1alpha1.Route{
		{Name: "no-coupon", Weight: 0.9, Chain: []string{"base", "payments"}},
		{Name: "with-coupon", Weight: 0.1, Chain: []string{"base", "coupon", "payments"}},
	}
	spec := makeWeightedSpec("30d", objectives, routes, alerting)
	return slos, spec
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_Metadata(t *testing.T) {
	slos, spec := checkoutWeightedFixture(nil)

	rule, err := CreateAggregatedPrometheusRule("checkout-weighted", "app", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rule.Name != "slok-checkout-weighted-app-aggregated" {
		t.Errorf("name: got %q, want %q", rule.Name, "slok-checkout-weighted-app-aggregated")
	}
	if rule.Namespace != "app" {
		t.Errorf("namespace: got %q, want %q", rule.Namespace, "app")
	}
	if rule.Labels["slok.io/slo_composition"] != "checkout-weighted" {
		t.Errorf("label slok.io/slo_composition: got %q, want %q", rule.Labels["slok.io/slo_composition"], "checkout-weighted")
	}
	if rule.Labels["release"] != "prometheus" {
		t.Errorf("label release: got %q, want prometheus", rule.Labels["release"])
	}
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_RuleCount(t *testing.T) {
	slos, spec := checkoutWeightedFixture(nil)

	rule, err := CreateAggregatedPrometheusRule("checkout-weighted", "app", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rule.Spec.Groups) != 1 {
		t.Fatalf("groups: got %d, want 1", len(rule.Spec.Groups))
	}
	if rule.Spec.Groups[0].Name != "slok-checkout-weighted-aggregated" {
		t.Errorf("group name: got %q, want %q", rule.Spec.Groups[0].Name, "slok-checkout-weighted-aggregated")
	}

	// len(recordingWindows) sli_error_composition_rate
	// + 1 objective_target + 1 error_budget_target
	// + len(recordingWindows) burn_rate
	expected := len(recordingWindows)*2 + 2
	got := len(rule.Spec.Groups[0].Rules)
	if got != expected {
		t.Fatalf("rules count: got %d, want %d", got, expected)
	}
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_ExprFormula(t *testing.T) {
	slos, spec := checkoutWeightedFixture(nil)

	rule, err := CreateAggregatedPrometheusRule("checkout-weighted", "app", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rules := rule.Spec.Groups[0].Rules

	for i, window := range recordingWindows {
		r := rules[i]

		if r.Record != "slok:sli_error_composition_rate:"+window {
			t.Errorf("rule[%d] record: got %q, want slok:sli_error_composition_rate:%s", i, r.Record, window)
		}

		expr := r.Expr.String()

		// Top-level structure: 1 - (...)
		if !strings.HasPrefix(expr, "1 - (") {
			t.Errorf("rule[%d] expr should start with '1 - (': %s", i, expr)
		}

		// Both route weights present
		if !strings.Contains(expr, "0.9 *") {
			t.Errorf("rule[%d] expr missing weight 0.9: %s", i, expr)
		}
		if !strings.Contains(expr, "0.1 *") {
			t.Errorf("rule[%d] expr missing weight 0.1: %s", i, expr)
		}

		// Success-rate form (1 - scalar(...))
		if !strings.Contains(expr, "(1 - scalar(") {
			t.Errorf("rule[%d] expr missing '(1 - scalar(': %s", i, expr)
		}

		// All three SLOs referenced with correct window
		for _, sloName := range []string{"checkout-base-slo", "payments-slo", "coupon-slo"} {
			want := `slo_name="` + sloName + `"`
			if !strings.Contains(expr, want) {
				t.Errorf("rule[%d] expr missing %q: %s", i, want, expr)
			}
		}
		if !strings.Contains(expr, "slok:sli_error_rate:"+window) {
			t.Errorf("rule[%d] expr missing window %q: %s", i, window, expr)
		}
	}
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_Labels(t *testing.T) {
	slos, spec := checkoutWeightedFixture(nil)

	rule, err := CreateAggregatedPrometheusRule("checkout-weighted", "app", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rules := rule.Spec.Groups[0].Rules

	for i, window := range recordingWindows {
		r := rules[i]
		if r.Labels["slo_composition_name"] != "checkout-weighted" {
			t.Errorf("rule[%d] slo_composition_name: got %q, want checkout-weighted", i, r.Labels["slo_composition_name"])
		}
		if r.Labels["slo_composition_namespace"] != "app" {
			t.Errorf("rule[%d] slo_composition_namespace: got %q, want app", i, r.Labels["slo_composition_namespace"])
		}
		if r.Labels["slok_window"] != window {
			t.Errorf("rule[%d] slok_window: got %q, want %q", i, r.Labels["slok_window"], window)
		}
	}

	// objective_target and error_budget_target use "30d" as window label
	objTarget := rules[len(recordingWindows)]
	if objTarget.Record != "slok:objective_target_composition" {
		t.Errorf("objective_target record: got %q", objTarget.Record)
	}
	budgetTarget := rules[len(recordingWindows)+1]
	if budgetTarget.Record != "slok:error_budget_target_composition" {
		t.Errorf("error_budget_target record: got %q", budgetTarget.Record)
	}
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_BurnRateRules(t *testing.T) {
	slos, spec := checkoutWeightedFixture(nil)

	rule, err := CreateAggregatedPrometheusRule("checkout-weighted", "app", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rules := rule.Spec.Groups[0].Rules
	burnRateOffset := len(recordingWindows) + 2

	for i, window := range recordingWindows {
		r := rules[burnRateOffset+i]
		expected := "slok:burn_rate_composition:" + window
		if r.Record != expected {
			t.Errorf("burn_rate[%d] record: got %q, want %q", i, r.Record, expected)
		}
		if !strings.Contains(r.Expr.String(), "slok:sli_error_composition_rate:"+window) {
			t.Errorf("burn_rate[%d] expr missing sli_error_composition_rate:%s: %s", i, window, r.Expr.String())
		}
	}
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_WithBurnRateAlerts(t *testing.T) {
	slos, spec := checkoutWeightedFixture(&observabilityv1alpha1.Alerting{
		BurnRateAlerts: &observabilityv1alpha1.BurnRates{Enabled: true},
	})

	rule, err := CreateAggregatedPrometheusRule("checkout-weighted", "app", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rule.Spec.Groups) != 2 {
		t.Fatalf("groups: got %d, want 2", len(rule.Spec.Groups))
	}

	alertGroup := rule.Spec.Groups[1]
	if alertGroup.Name != "slok-checkout-weighted-aggregated-burnRateAlerts" {
		t.Errorf("alert group name: got %q", alertGroup.Name)
	}

	expectedAlerts := len(defaultBurnRatePresets) + 1
	if len(alertGroup.Rules) != expectedAlerts {
		t.Fatalf("alert rules count: got %d, want %d", len(alertGroup.Rules), expectedAlerts)
	}

	for i, preset := range defaultBurnRatePresets {
		expr := alertGroup.Rules[i].Expr.String()
		if !strings.Contains(expr, "slok:burn_rate_composition:"+preset.ShortWindow) {
			t.Errorf("alert[%d] expr missing burn_rate_composition:%s: %s", i, preset.ShortWindow, expr)
		}
		if alertGroup.Rules[i].Labels["severity"] != preset.Severity {
			t.Errorf("alert[%d] severity: got %q, want %q", i, alertGroup.Rules[i].Labels["severity"], preset.Severity)
		}
	}

	// Budget exhausted alert
	lastAlert := alertGroup.Rules[len(alertGroup.Rules)-1]
	if !strings.Contains(lastAlert.Expr.String(), "slok:sli_error_composition_rate:30d") {
		t.Errorf("budget alert expr missing sli_error_composition_rate:30d: %s", lastAlert.Expr.String())
	}
	if lastAlert.Labels["severity"] != severityWarning {
		t.Errorf("budget alert severity: got %q, want warning", lastAlert.Labels["severity"])
	}
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_SingleRoute(t *testing.T) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("api-slo", "default", "availability"),
	}
	spec := makeWeightedSpec("30d",
		[]observabilityv1alpha1.SLORef{makeSLORef("api", "api-slo", "default")},
		[]observabilityv1alpha1.Route{
			{Name: "main", Weight: 1.0, Chain: []string{"api"}},
		},
		nil,
	)

	rule, err := CreateAggregatedPrometheusRule("single-weighted", "default", spec, slos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expr := rule.Spec.Groups[0].Rules[0].Expr.String()
	if !strings.HasPrefix(expr, "1 - (") {
		t.Errorf("expr should start with '1 - (': %s", expr)
	}
	if !strings.Contains(expr, `slo_name="api-slo"`) {
		t.Errorf("expr missing slo_name=api-slo: %s", expr)
	}
}

func TestCreateAggregatedPrometheusRule_WEIGHTED_ROUTES_UnknownAlias(t *testing.T) {
	slos := []observabilityv1alpha1.ServiceLevelObjective{
		makeSLO("api-slo", "default", "availability"),
	}
	spec := makeWeightedSpec("30d",
		[]observabilityv1alpha1.SLORef{makeSLORef("api", "api-slo", "default")},
		[]observabilityv1alpha1.Route{
			{Name: "bad-route", Weight: 1.0, Chain: []string{"api", "ghost"}}, // "ghost" not in objectives
		},
		nil,
	)

	_, err := CreateAggregatedPrometheusRule("bad-composition", "default", spec, slos)
	if err == nil {
		t.Fatal("expected error for unknown alias, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should mention the unknown alias 'ghost': %v", err)
	}
	if !strings.Contains(err.Error(), "not found in objectives") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestPromQLLogicalOperatorsAreLowercaseAndWindowSafe(t *testing.T) {
	errorRateExpr := sliErrorRateExpr("http_requests_total", `http_requests_total{status=~"5.."}`, "5m")
	if strings.Contains(errorRateExpr, " OR ") {
		t.Fatalf("expected lowercase or in SLI error rate expression, got: %s", errorRateExpr)
	}
	if !strings.Contains(errorRateExpr, " or ") {
		t.Fatalf("expected SLI error rate expression to contain lowercase or, got: %s", errorRateExpr)
	}

	preset := burnRatePreset{
		ShortWindow: "5m",
		LongWindow:  "1h",
		BurnRate:    14,
	}

	alertExpr := burnRateAlertExpr("checkout", "prod", "availability", preset)
	if strings.Contains(alertExpr, " AND ") {
		t.Fatalf("expected lowercase and in burn-rate alert expression, got: %s", alertExpr)
	}
	if !strings.Contains(alertExpr, " and on (slo_name, slo_namespace, objective_name) ") {
		t.Fatalf("expected burn-rate alert expression to match on SLO identity labels, got: %s", alertExpr)
	}

	compositionExpr := burnRateAlertExprComposition("checkout-flow", "prod", preset)
	if strings.Contains(compositionExpr, " AND ") {
		t.Fatalf("expected lowercase and in burn-rate composition expression, got: %s", compositionExpr)
	}
	if !strings.Contains(compositionExpr, " and on (slo_composition_name, slo_composition_namespace) ") {
		t.Fatalf("expected burn-rate composition expression to match on composition identity labels, got: %s", compositionExpr)
	}
}
