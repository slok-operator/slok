package main

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBacktestRangeDefaultsToSLOWindow(t *testing.T) {
	cmd := newBacktestCmd()
	flag := cmd.Flags().Lookup("range")
	if flag == nil {
		t.Fatal("expected --range flag to exist")
	}
	if flag.DefValue != "" {
		t.Fatalf("expected --range to default to empty so the SLO objective window can be used, got %q", flag.DefValue)
	}
}

func TestBacktestCommandExposesTimeoutFlag(t *testing.T) {
	cmd := newBacktestCmd()
	flag := cmd.Flags().Lookup("timeout")
	if flag == nil {
		t.Fatal("expected --timeout flag to exist")
	}
	if flag.DefValue != "30s" {
		t.Fatalf("expected default timeout 30s, got %q", flag.DefValue)
	}
}

func TestBacktestCommandExposesPreApplyFlag(t *testing.T) {
	cmd := newBacktestCmd()
	flag := cmd.Flags().Lookup("pre-apply")
	if flag == nil {
		t.Fatal("expected --pre-apply flag to exist")
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected default pre-apply false, got %q", flag.DefValue)
	}
}

func TestBacktestHelpClarifiesFileModeRequiresExistingRecordingRules(t *testing.T) {
	cmd := newBacktestCmd()
	help := cmd.Long + "\n" + cmd.Short
	for _, want := range []string{"existing SloK recording rules", "--pre-apply", "totalQuery/errorQuery"} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected backtest help to mention %q, got:\n%s", want, help)
		}
	}
}

func TestParseTargetsFallsBackToDefault(t *testing.T) {
	targets, err := parseTargets("", 99.9)
	if err != nil {
		t.Fatalf("parseTargets returned error: %v", err)
	}
	if !reflect.DeepEqual(targets, []float64{99.9}) {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestParseTargetsParsesCommaSeparatedValues(t *testing.T) {
	targets, err := parseTargets("99, 99.5,99.9,99.95", 99)
	if err != nil {
		t.Fatalf("parseTargets returned error: %v", err)
	}
	want := []float64{99, 99.5, 99.9, 99.95}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("expected %#v, got %#v", want, targets)
	}
}

func TestParseTargetsRejectsInvalidValues(t *testing.T) {
	tests := []string{"0", "100", "101", "-1", "NaN", "+Inf", "not-a-number"}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if _, err := parseTargets(tt, 99.9); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestParseTargetsRejectsInvalidDefaultTarget(t *testing.T) {
	if _, err := parseTargets("", math.NaN()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveSLORequiresNameOrFile(t *testing.T) {
	_, err := resolveSLO(context.Background(), "default", "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "provide --name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTimeout(t *testing.T) {
	timeout, err := parseTimeout("45s")
	if err != nil {
		t.Fatalf("parseTimeout returned error: %v", err)
	}
	if timeout != 45*time.Second {
		t.Fatalf("expected 45s, got %s", timeout)
	}
}

func TestParseTimeoutRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "0s", "-1s", "not-a-duration"} {
		t.Run(value, func(t *testing.T) {
			if _, err := parseTimeout(value); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestRunBacktestPreApplyRequiresFile(t *testing.T) {
	err := runBacktest("default", "checkout", "", "http://prometheus.example", "30d", "99.9", "1s", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--pre-apply requires --file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSLOFromFile(t *testing.T) {
	path := writeTempSLO(t, `
apiVersion: slok.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: checkout
  namespace: payments
spec:
  objective:
    name: availability
    target: 99.9
    window: 30d
`)

	slo, err := resolveSLOFromFile(path)
	if err != nil {
		t.Fatalf("resolveSLOFromFile returned error: %v", err)
	}
	if slo.Name != "checkout" || slo.Namespace != "payments" || slo.ObjectiveName != "availability" ||
		slo.Target != 99.9 || slo.Window != "30d" {
		t.Fatalf("unexpected SLO fields: %#v", slo)
	}
}

func TestResolveSLOFromFileReadsRawSLIQueries(t *testing.T) {
	path := writeTempSLO(t, `
apiVersion: slok.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: checkout
spec:
  objective:
    name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        totalQuery: http_requests_total{job="checkout"}
        errorQuery: http_requests_total{job="checkout",status=~"5.."}
`)

	slo, err := resolveSLOFromFile(path)
	if err != nil {
		t.Fatalf("resolveSLOFromFile returned error: %v", err)
	}
	if slo.TotalQuery != `http_requests_total{job="checkout"}` ||
		slo.ErrorQuery != `http_requests_total{job="checkout",status=~"5.."}` {
		t.Fatalf("unexpected raw SLI queries: %#v", slo)
	}
}

func TestResolveSLOFromFileDefaultsNamespace(t *testing.T) {
	path := writeTempSLO(t, `
apiVersion: slok.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: checkout
spec:
  objective:
    name: availability
    target: 99.9
    window: 30d
`)

	slo, err := resolveSLOFromFile(path)
	if err != nil {
		t.Fatalf("resolveSLOFromFile returned error: %v", err)
	}
	if slo.Namespace != "default" {
		t.Fatalf("expected default namespace, got %q", slo.Namespace)
	}
}

func TestResolveSLOFromFileReturnsReadAndParseErrors(t *testing.T) {
	if _, err := resolveSLOFromFile(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected read error, got nil")
	}

	path := writeTempSLO(t, `metadata: [`)
	if _, err := resolveSLOFromFile(path); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func writeTempSLO(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "slo.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o600); err != nil {
		t.Fatalf("write temp SLO: %v", err)
	}
	return path
}
