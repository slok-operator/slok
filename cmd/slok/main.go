package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

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
		timeoutStr    string
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

File mode currently reads the SLO identity, target, and window from YAML,
then backtests against existing SloK recording rules in Prometheus.
The SLO must already have been applied at least once for those rules to exist.
True pre-apply backtesting directly from SLI total/error queries is planned.

What-if with multiple targets:
  slok backtest -f slo.yaml --targets 99,99.5,99.9,99.95`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacktest(namespace, name, file, prometheusURL, rangeStr, targetsStr, timeoutStr)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Namespace of the SLO (cluster mode)")
	cmd.Flags().StringVar(&name, "name", "", "Name of the ServiceLevelObjective (cluster mode)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to SLO YAML file (file mode)")
	cmd.Flags().StringVar(&prometheusURL, "prometheus-url", "http://localhost:9090", "Prometheus base URL")
	cmd.Flags().StringVar(&rangeStr, "range", "", "Historical range to evaluate (e.g. 30d, 7d, 24h). Defaults to the SLO objective window")
	cmd.Flags().StringVar(&targetsStr, "targets", "", "Comma-separated target % values for what-if (e.g. 99,99.5,99.9)")
	cmd.Flags().StringVar(&timeoutStr, "timeout", "30s", "Timeout for Kubernetes and Prometheus requests")

	return cmd
}

func runBacktest(namespace, name, file, prometheusURL, rangeStr, targetsStr, timeoutStr string) error {
	timeout, err := parseTimeout(timeoutStr)
	if err != nil {
		return fmt.Errorf("parsing --timeout: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slo, err := resolveSLO(ctx, namespace, name, file)
	if err != nil {
		return err
	}
	if rangeStr == "" {
		rangeStr = slo.Window
	}
	if rangeStr == "" {
		return fmt.Errorf("SLO objective window is empty; provide --range")
	}

	targets, err := parseTargets(targetsStr, slo.Target)
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
		Namespace:     slo.Namespace,
		Name:          slo.Name,
		ObjectiveName: slo.ObjectiveName,
		Range:         rangeStr,
		Targets:       targets,
	})
	if err != nil {
		return err
	}

	backtest.Print(os.Stdout, result)
	return nil
}

type resolvedSLO struct {
	Name          string
	Namespace     string
	ObjectiveName string
	Target        float64
	Window        string
}

// resolveSLO returns SLO metadata from file or cluster.
func resolveSLO(ctx context.Context, namespace, name, file string) (resolvedSLO, error) {
	if file != "" {
		return resolveSLOFromFile(file)
	}
	if name == "" {
		return resolvedSLO{}, fmt.Errorf("provide --name (cluster mode) or --file / -f (file mode)")
	}
	return resolveSLOFromCluster(ctx, namespace, name)
}

func resolveSLOFromFile(path string) (resolvedSLO, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return resolvedSLO{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var slo observabilityv1alpha1.ServiceLevelObjective
	if err := sigsyaml.Unmarshal(data, &slo); err != nil {
		return resolvedSLO{}, fmt.Errorf("parsing SLO YAML: %w", err)
	}
	ns := slo.Namespace
	if ns == "" {
		ns = "default"
	}
	return resolvedSLO{
		Name:          slo.Name,
		Namespace:     ns,
		ObjectiveName: slo.Spec.Objective.Name,
		Target:        slo.Spec.Objective.Target,
		Window:        slo.Spec.Objective.Window,
	}, nil
}

func resolveSLOFromCluster(ctx context.Context, namespace, name string) (resolvedSLO, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return resolvedSLO{}, err
	}
	if err := observabilityv1alpha1.AddToScheme(scheme); err != nil {
		return resolvedSLO{}, err
	}

	cfg, err := ctrl.GetConfig()
	if err != nil {
		return resolvedSLO{}, fmt.Errorf("getting kubeconfig: %w", err)
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return resolvedSLO{}, fmt.Errorf("creating k8s client: %w", err)
	}

	var slo observabilityv1alpha1.ServiceLevelObjective
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, &slo); err != nil {
		return resolvedSLO{}, fmt.Errorf("fetching SLO %s/%s: %w", namespace, name, err)
	}

	return resolvedSLO{
		Name:          slo.Name,
		Namespace:     slo.Namespace,
		ObjectiveName: slo.Spec.Objective.Name,
		Target:        slo.Spec.Objective.Target,
		Window:        slo.Spec.Objective.Window,
	}, nil
}

func parseTimeout(value string) (time.Duration, error) {
	timeout, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("timeout must be greater than zero")
	}
	return timeout, nil
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
