package templates

import (
	"fmt"
	"strings"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

// Template names
const (
	HTTPAvailability = "http-availability"
	// Future templates:
	// GRPCAvailability = "grpc-availability"
	// HTTPLatency      = "http-latency"
)

// ResolvedQuery holds the resolved totalQuery and errorQuery from a template
type ResolvedQuery struct {
	TotalQuery string
	ErrorQuery string
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
//	    namespace: "production"
//
// Generates:
//   - totalQuery: http_requests_total{service="payment-api",namespace="production"}
//   - errorQuery: http_requests_total{service="payment-api",namespace="production",status=~"5.."}
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
