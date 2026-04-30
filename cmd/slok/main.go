package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/backtest"
	prometheusclient "github.com/federicolepera/slok/internal/prometheus"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"
)

func main() {
	root := &cobra.Command{
		Use:   "slok",
		Short: "SloK CLI — backtest and analyze your SLOs",
	}
	root.AddCommand(newBacktestCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newBacktestCmd() *cobra.Command {
	var (
		namespace     string
		name          string
		file          string
		prometheusURL string
		rangeStr      string
		targetsStr    string
	)

	cmd := &cobra.Command{
		Use:   "backtest",
		Short: "Backtest an SLO against historical Prometheus data",
		Long: `Backtest queries Prometheus history and computes whether your SLO
would have passed over the given time range.

Read from cluster:
  slok backtest --namespace default --name my-slo --range 30d

Read from file:
  slok backtest -f slo.yaml --range 30d

What-if with multiple targets:
  slok backtest -f slo.yaml --targets 99,99.5,99.9,99.95`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacktest(namespace, name, file, prometheusURL, rangeStr, targetsStr)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Namespace of the SLO (cluster mode)")
	cmd.Flags().StringVar(&name, "name", "", "Name of the ServiceLevelObjective (cluster mode)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to SLO YAML file (file mode)")
	cmd.Flags().StringVar(&prometheusURL, "prometheus-url", "http://localhost:9090", "Prometheus base URL")
	cmd.Flags().StringVar(&rangeStr, "range", "30d", "Historical range to evaluate (e.g. 30d, 7d, 24h)")
	cmd.Flags().StringVar(&targetsStr, "targets", "", "Comma-separated target % values for what-if (e.g. 99,99.5,99.9)")

	return cmd
}

func runBacktest(namespace, name, file, prometheusURL, rangeStr, targetsStr string) error {
	ctx := context.Background()

	sloName, sloNamespace, objectiveName, defaultTarget, err := resolveSLO(namespace, name, file)
	if err != nil {
		return err
	}

	targets, err := parseTargets(targetsStr, defaultTarget)
	if err != nil {
		return fmt.Errorf("parsing --targets: %w", err)
	}

	pc, err := prometheusclient.NewClient(prometheusURL)
	if err != nil {
		return fmt.Errorf("creating Prometheus client: %w", err)
	}
	if err := pc.CheckConnection(ctx); err != nil {
		return fmt.Errorf("cannot reach Prometheus at %s: %w", prometheusURL, err)
	}

	result, err := backtest.New(pc).Run(ctx, backtest.Config{
		Namespace:     sloNamespace,
		Name:          sloName,
		ObjectiveName: objectiveName,
		Range:         rangeStr,
		Targets:       targets,
	})
	if err != nil {
		return err
	}

	backtest.Print(os.Stdout, result)
	return nil
}

// resolveSLO returns (sloName, sloNamespace, objectiveName, target) from file or cluster.
func resolveSLO(namespace, name, file string) (string, string, string, float64, error) {
	if file != "" {
		return resolveSLOFromFile(file)
	}
	if name == "" {
		return "", "", "", 0, fmt.Errorf("provide --name (cluster mode) or --file / -f (file mode)")
	}
	return resolveSLOFromCluster(namespace, name)
}

func resolveSLOFromFile(path string) (string, string, string, float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("reading %s: %w", path, err)
	}
	var slo observabilityv1alpha1.ServiceLevelObjective
	if err := sigsyaml.Unmarshal(data, &slo); err != nil {
		return "", "", "", 0, fmt.Errorf("parsing SLO YAML: %w", err)
	}
	ns := slo.Namespace
	if ns == "" {
		ns = "default"
	}
	return slo.Name, ns, slo.Spec.Objective.Name, slo.Spec.Objective.Target, nil
}

func resolveSLOFromCluster(namespace, name string) (string, string, string, float64, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return "", "", "", 0, err
	}
	if err := observabilityv1alpha1.AddToScheme(scheme); err != nil {
		return "", "", "", 0, err
	}

	cfg, err := ctrl.GetConfig()
	if err != nil {
		return "", "", "", 0, fmt.Errorf("getting kubeconfig: %w", err)
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return "", "", "", 0, fmt.Errorf("creating k8s client: %w", err)
	}

	var slo observabilityv1alpha1.ServiceLevelObjective
	if err := k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, &slo); err != nil {
		return "", "", "", 0, fmt.Errorf("fetching SLO %s/%s: %w", namespace, name, err)
	}

	return slo.Name, slo.Namespace, slo.Spec.Objective.Name, slo.Spec.Objective.Target, nil
}

// parseTargets parses a comma-separated list of targets, falling back to defaultTarget.
func parseTargets(targetsStr string, defaultTarget float64) ([]float64, error) {
	if targetsStr == "" {
		if err := validateTarget(defaultTarget); err != nil {
			return nil, fmt.Errorf("invalid default target: %w", err)
		}
		return []float64{defaultTarget}, nil
	}
	parts := strings.Split(targetsStr, ",")
	targets := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		t, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", p, err)
		}
		if err := validateTarget(t); err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", p, err)
		}
		targets = append(targets, t)
	}
	return targets, nil
}

func validateTarget(target float64) error {
	if math.IsNaN(target) || math.IsInf(target, 0) || target <= 0 || target >= 100 {
		return fmt.Errorf("target must be greater than 0 and lower than 100")
	}
	return nil
}
