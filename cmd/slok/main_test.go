package main

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
	_, _, _, _, err := resolveSLO("default", "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "provide --name") {
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

	name, namespace, objectiveName, target, err := resolveSLOFromFile(path)
	if err != nil {
		t.Fatalf("resolveSLOFromFile returned error: %v", err)
	}
	if name != "checkout" || namespace != "payments" || objectiveName != "availability" || target != 99.9 {
		t.Fatalf("unexpected SLO fields: name=%q namespace=%q objective=%q target=%f", name, namespace, objectiveName, target)
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

	_, namespace, _, _, err := resolveSLOFromFile(path)
	if err != nil {
		t.Fatalf("resolveSLOFromFile returned error: %v", err)
	}
	if namespace != "default" {
		t.Fatalf("expected default namespace, got %q", namespace)
	}
}

func TestResolveSLOFromFileReturnsReadAndParseErrors(t *testing.T) {
	if _, _, _, _, err := resolveSLOFromFile(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected read error, got nil")
	}

	path := writeTempSLO(t, `metadata: [`)
	if _, _, _, _, err := resolveSLOFromFile(path); err == nil {
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
