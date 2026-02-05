package templates

import (
	"fmt"
	"strings"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

// Template names
const (
	HTTPAvailability    = "http-availability"
	HTTPLatency         = "http-latency"
	KubernetesAPIServer = "kubernetes-apiserver"
)

// ResolvedQuery holds the resolved queries from a template.
// For simple templates (http-availability), TotalQuery and ErrorQuery are metric selectors.
// For complex templates (http-latency), RawExpr contains the full PromQL expression
// with {{window}} placeholder that will be replaced by the actual window.
type ResolvedQuery struct {
	// TotalQuery is the Prometheus metric selector for total events.
	// Used when RawExpr is empty.
	TotalQuery string

	// ErrorQuery is the Prometheus metric selector for error events.
	// Used when RawExpr is empty.
	ErrorQuery string

	// RawExpr is a complete PromQL expression for the SLI error rate.
	// Contains {{window}} placeholder that will be replaced by the actual window (e.g., "5m").
	// When set, TotalQuery and ErrorQuery are ignored.
	RawExpr string
}

// IsRawExpression returns true if this resolved query uses a raw PromQL expression
// instead of simple metric selectors.
func (r ResolvedQuery) IsRawExpression() bool {
	return r.RawExpr != ""
}

// Resolve takes an SLI and returns the resolved queries.
// If a template is specified, it generates the queries from the template.
// Otherwise, it returns the manually specified queries.
func Resolve(sli observabilityv1alpha1.SLI) (ResolvedQuery, error) {
	// If template is specified, use it
	if sli.Template.Name != "" {
		return resolveTemplate(sli.Template)
	}

	// Validate manual queries are provided
	if sli.Query.TotalQuery == "" || sli.Query.ErrorQuery == "" {
		return ResolvedQuery{}, fmt.Errorf("either template or both totalQuery and errorQuery must be specified")
	}

	// Use manual queries
	return ResolvedQuery{
		TotalQuery: sli.Query.TotalQuery,
		ErrorQuery: sli.Query.ErrorQuery,
	}, nil
}

// resolveTemplate generates queries based on the template name and labels
func resolveTemplate(template observabilityv1alpha1.TemplateStruct) (ResolvedQuery, error) {
	switch template.Name {
	case HTTPAvailability:
		return httpAvailabilityTemplate(template.Labels)
	case HTTPLatency:
		return httpLatencyTemplate(template.Labels, template.Params)
	case KubernetesAPIServer:
		return kubernetesAPIServerTemplate(template.Labels, template.Params)
	default:
		return ResolvedQuery{}, fmt.Errorf("unknown template: %s", template.Name)
	}
}

// httpAvailabilityTemplate generates queries for HTTP availability SLI.
//
// This template measures the ratio of successful HTTP requests (non-5xx) to total requests.
// It expects labels to filter the metric (e.g., service, namespace, job).
//
// Generated queries:
//   - totalQuery: http_requests_total{labels...}
//   - errorQuery: http_requests_total{labels..., status=~"5.."}
//
// Example usage:
//
//	template:
//	  name: http-availability
//	  labels:
//	    service: "payment-api"
func httpAvailabilityTemplate(labels map[string]string) (ResolvedQuery, error) {
	labelSelector := buildLabelSelector(labels)

	totalQuery := fmt.Sprintf("http_requests_total{%s}", labelSelector)
	errorQuery := fmt.Sprintf(`http_requests_total{%s,status=~"5.."}`, labelSelector)

	// Handle empty labels case
	if labelSelector == "" {
		totalQuery = "http_requests_total"
		errorQuery = `http_requests_total{status=~"5.."}`
	}

	return ResolvedQuery{
		TotalQuery: totalQuery,
		ErrorQuery: errorQuery,
	}, nil
}

// httpLatencyTemplate generates a raw PromQL expression for HTTP latency SLI.
//
// This template measures the ratio of slow requests (above threshold) to total requests.
// It uses histogram buckets to determine fast vs slow requests.
//
// Required params:
//   - threshold: latency threshold in seconds (e.g., "0.5" for 500ms)
//
// Generated expression (error rate = 1 - fast_ratio):
//
//	1 - (
//	  sum(rate(http_request_duration_seconds_bucket{labels,le="threshold"}[{{window}}]))
//	  /
//	  clamp_min(sum(rate(http_request_duration_seconds_count{labels}[{{window}}])), 1e-12)
//	)
//
// Example usage:
//
//	template:
//	  name: http-latency
//	  labels:
//	    service: "payment-api"
//	  params:
//	    threshold: "0.5"
func httpLatencyTemplate(labels map[string]string, params map[string]string) (ResolvedQuery, error) {
	threshold, ok := params["threshold"]
	if !ok || threshold == "" {
		return ResolvedQuery{}, fmt.Errorf("http-latency template requires 'threshold' param (e.g., \"0.5\" for 500ms)")
	}

	labelSelector := buildLabelSelector(labels)

	var bucketSelector, countSelector string
	if labelSelector == "" {
		bucketSelector = fmt.Sprintf(`le="%s"`, threshold)
		countSelector = ""
	} else {
		bucketSelector = fmt.Sprintf(`%s,le="%s"`, labelSelector, threshold)
		countSelector = labelSelector
	}

	// Build the raw expression with {{window}} placeholder
	// error_rate = 1 - (fast / total)
	rawExpr := fmt.Sprintf(
		`1 - (sum(rate(http_request_duration_seconds_bucket{%s}[{{window}}])) / clamp_min(sum(rate(http_request_duration_seconds_count{%s}[{{window}}])), 1e-12))`,
		bucketSelector,
		countSelector,
	)

	// Handle empty count selector
	if countSelector == "" {
		rawExpr = fmt.Sprintf(
			`1 - (sum(rate(http_request_duration_seconds_bucket{%s}[{{window}}])) / clamp_min(sum(rate(http_request_duration_seconds_count[{{window}}])), 1e-12))`,
			bucketSelector,
		)
	}

	return ResolvedQuery{
		RawExpr: rawExpr,
	}, nil
}

// kubernetesAPIServerTemplate generates queries for Kubernetes API server availability SLI.
//
// This template measures the ratio of successful API server requests to total requests.
// It uses the standard apiserver_request_total metric.
//
// Optional params:
//   - errorCodes: regex pattern for error status codes (default: "5..")
//
// Generated queries:
//   - totalQuery: apiserver_request_total{labels...}
//   - errorQuery: apiserver_request_total{labels..., code=~"errorCodes"}
//
// Example usage:
//
//	template:
//	  name: kubernetes-apiserver
//	  labels:
//	    verb: "GET"
//	    resource: "pods"
//	  params:
//	    errorCodes: "5.."
func kubernetesAPIServerTemplate(labels map[string]string, params map[string]string) (ResolvedQuery, error) {
	labelSelector := buildLabelSelector(labels)

	// Default error codes to 5xx
	errorCodes := params["errorCodes"]
	if errorCodes == "" {
		errorCodes = "5.."
	}

	var totalQuery, errorQuery string
	if labelSelector == "" {
		totalQuery = "apiserver_request_total"
		errorQuery = fmt.Sprintf(`apiserver_request_total{code=~"%s"}`, errorCodes)
	} else {
		totalQuery = fmt.Sprintf("apiserver_request_total{%s}", labelSelector)
		errorQuery = fmt.Sprintf(`apiserver_request_total{%s,code=~"%s"}`, labelSelector, errorCodes)
	}

	return ResolvedQuery{
		TotalQuery: totalQuery,
		ErrorQuery: errorQuery,
	}, nil
}

// buildLabelSelector creates a Prometheus label selector string from a map
// Example: {"service": "api", "namespace": "prod"} -> `service="api",namespace="prod"`
func buildLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, v))
	}

	return strings.Join(parts, ",")
}

// AvailableTemplates returns a list of all available template names
func AvailableTemplates() []string {
	return []string{
		HTTPAvailability,
		HTTPLatency,
		KubernetesAPIServer,
	}
}

// IsValidTemplate checks if a template name is valid
func IsValidTemplate(name string) bool {
	for _, t := range AvailableTemplates() {
		if t == name {
			return true
		}
	}
	return false
}
