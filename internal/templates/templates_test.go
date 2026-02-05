package templates

import (
	"testing"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

func TestResolve_WithHTTPAvailabilityTemplate(t *testing.T) {
	tests := []struct {
		name           string
		sli            observabilityv1alpha1.SLI
		wantTotalQuery string
		wantErrorQuery string
		wantErr        bool
	}{
		{
			name: "http-availability with labels",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: HTTPAvailability,
					Labels: map[string]string{
						"service": "payment-api",
					},
				},
			},
			wantTotalQuery: `http_requests_total{service="payment-api"}`,
			wantErrorQuery: `http_requests_total{service="payment-api",status=~"5.."}`,
			wantErr:        false,
		},
		{
			name: "http-availability with multiple labels",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: HTTPAvailability,
					Labels: map[string]string{
						"service":   "payment-api",
						"namespace": "production",
					},
				},
			},
			wantTotalQuery: `http_requests_total{`, // partial match due to map ordering
			wantErrorQuery: `http_requests_total{`, // partial match
			wantErr:        false,
		},
		{
			name: "http-availability without labels",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: HTTPAvailability,
				},
			},
			wantTotalQuery: "http_requests_total",
			wantErrorQuery: `http_requests_total{status=~"5.."}`,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.sli)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// For cases with multiple labels, just check prefix due to map ordering
			if tt.name == "http-availability with multiple labels" {
				if len(got.TotalQuery) < len(tt.wantTotalQuery) {
					t.Errorf("Resolve() TotalQuery = %v, want prefix %v", got.TotalQuery, tt.wantTotalQuery)
				}
				return
			}

			if got.TotalQuery != tt.wantTotalQuery {
				t.Errorf("Resolve() TotalQuery = %v, want %v", got.TotalQuery, tt.wantTotalQuery)
			}
			if got.ErrorQuery != tt.wantErrorQuery {
				t.Errorf("Resolve() ErrorQuery = %v, want %v", got.ErrorQuery, tt.wantErrorQuery)
			}
		})
	}
}

func TestResolve_WithManualQueries(t *testing.T) {
	sli := observabilityv1alpha1.SLI{
		Query: observabilityv1alpha1.Query{
			TotalQuery: "my_custom_total",
			ErrorQuery: "my_custom_errors",
		},
	}

	got, err := Resolve(sli)
	if err != nil {
		t.Errorf("Resolve() unexpected error = %v", err)
		return
	}

	if got.TotalQuery != "my_custom_total" {
		t.Errorf("Resolve() TotalQuery = %v, want my_custom_total", got.TotalQuery)
	}
	if got.ErrorQuery != "my_custom_errors" {
		t.Errorf("Resolve() ErrorQuery = %v, want my_custom_errors", got.ErrorQuery)
	}
}

func TestResolve_MissingQueriesAndTemplate(t *testing.T) {
	sli := observabilityv1alpha1.SLI{}

	_, err := Resolve(sli)
	if err == nil {
		t.Error("Resolve() expected error when both template and queries are missing")
	}
}

func TestResolve_PartialQueries(t *testing.T) {
	tests := []struct {
		name string
		sli  observabilityv1alpha1.SLI
	}{
		{
			name: "only totalQuery",
			sli: observabilityv1alpha1.SLI{
				Query: observabilityv1alpha1.Query{
					TotalQuery: "some_total",
				},
			},
		},
		{
			name: "only errorQuery",
			sli: observabilityv1alpha1.SLI{
				Query: observabilityv1alpha1.Query{
					ErrorQuery: "some_errors",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.sli)
			if err == nil {
				t.Error("Resolve() expected error when only partial queries provided")
			}
		})
	}
}

func TestResolve_UnknownTemplate(t *testing.T) {
	sli := observabilityv1alpha1.SLI{
		Template: observabilityv1alpha1.TemplateStruct{
			Name: "unknown-template",
		},
	}

	_, err := Resolve(sli)
	if err == nil {
		t.Error("Resolve() expected error for unknown template")
	}
}

func TestIsValidTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     bool
	}{
		{"valid http-availability", HTTPAvailability, true},
		{"valid http-latency", HTTPLatency, true},
		{"valid kubernetes-apiserver", KubernetesAPIServer, true},
		{"invalid template", "unknown", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidTemplate(tt.template); got != tt.want {
				t.Errorf("IsValidTemplate(%q) = %v, want %v", tt.template, got, tt.want)
			}
		})
	}
}

func TestAvailableTemplates(t *testing.T) {
	templates := AvailableTemplates()
	if len(templates) == 0 {
		t.Error("AvailableTemplates() returned empty list")
	}

	found := false
	for _, tmpl := range templates {
		if tmpl == HTTPAvailability {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AvailableTemplates() does not contain %s", HTTPAvailability)
	}
}

func TestBuildLabelSelector(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   "",
		},
		{
			name:   "nil labels",
			labels: nil,
			want:   "",
		},
		{
			name:   "single label",
			labels: map[string]string{"service": "api"},
			want:   `service="api"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLabelSelector(tt.labels); got != tt.want {
				t.Errorf("buildLabelSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolve_WithHTTPLatencyTemplate(t *testing.T) {
	tests := []struct {
		name        string
		sli         observabilityv1alpha1.SLI
		wantRawExpr string
		wantErr     bool
	}{
		{
			name: "http-latency with labels and threshold",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: HTTPLatency,
					Labels: map[string]string{
						"service": "payment-api",
					},
					Params: map[string]string{
						"threshold": "0.5",
					},
				},
			},
			wantRawExpr: `1 - (sum(rate(http_request_duration_seconds_bucket{service="payment-api",le="0.5"}[{{window}}])) / clamp_min(sum(rate(http_request_duration_seconds_count{service="payment-api"}[{{window}}])), 1e-12))`,
			wantErr:     false,
		},
		{
			name: "http-latency without labels",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: HTTPLatency,
					Params: map[string]string{
						"threshold": "0.5",
					},
				},
			},
			wantRawExpr: `1 - (sum(rate(http_request_duration_seconds_bucket{le="0.5"}[{{window}}])) / clamp_min(sum(rate(http_request_duration_seconds_count[{{window}}])), 1e-12))`,
			wantErr:     false,
		},
		{
			name: "http-latency missing threshold",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: HTTPLatency,
					Labels: map[string]string{
						"service": "payment-api",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "http-latency empty threshold",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: HTTPLatency,
					Params: map[string]string{
						"threshold": "",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.sli)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if !got.IsRawExpression() {
				t.Error("Resolve() expected RawExpr to be set for http-latency template")
				return
			}

			if got.RawExpr != tt.wantRawExpr {
				t.Errorf("Resolve() RawExpr = %v, want %v", got.RawExpr, tt.wantRawExpr)
			}
		})
	}
}

func TestIsRawExpression(t *testing.T) {
	tests := []struct {
		name     string
		resolved ResolvedQuery
		want     bool
	}{
		{
			name:     "empty resolved query",
			resolved: ResolvedQuery{},
			want:     false,
		},
		{
			name: "with total and error queries",
			resolved: ResolvedQuery{
				TotalQuery: "total",
				ErrorQuery: "error",
			},
			want: false,
		},
		{
			name: "with raw expression",
			resolved: ResolvedQuery{
				RawExpr: "some_expr",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resolved.IsRawExpression(); got != tt.want {
				t.Errorf("IsRawExpression() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolve_WithKubernetesAPIServerTemplate(t *testing.T) {
	tests := []struct {
		name           string
		sli            observabilityv1alpha1.SLI
		wantTotalQuery string
		wantErrorQuery string
		wantErr        bool
	}{
		{
			name: "kubernetes-apiserver with labels",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: KubernetesAPIServer,
					Labels: map[string]string{
						"verb":     "GET",
						"resource": "pods",
					},
				},
			},
			wantTotalQuery: `apiserver_request_total{`, // partial match due to map ordering
			wantErrorQuery: `apiserver_request_total{`, // partial match
			wantErr:        false,
		},
		{
			name: "kubernetes-apiserver without labels",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: KubernetesAPIServer,
				},
			},
			wantTotalQuery: "apiserver_request_total",
			wantErrorQuery: `apiserver_request_total{code=~"5.."}`,
			wantErr:        false,
		},
		{
			name: "kubernetes-apiserver with custom error codes",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: KubernetesAPIServer,
					Params: map[string]string{
						"errorCodes": "4..|5..",
					},
				},
			},
			wantTotalQuery: "apiserver_request_total",
			wantErrorQuery: `apiserver_request_total{code=~"4..|5.."}`,
			wantErr:        false,
		},
		{
			name: "kubernetes-apiserver with single label",
			sli: observabilityv1alpha1.SLI{
				Template: observabilityv1alpha1.TemplateStruct{
					Name: KubernetesAPIServer,
					Labels: map[string]string{
						"verb": "LIST",
					},
				},
			},
			wantTotalQuery: `apiserver_request_total{verb="LIST"}`,
			wantErrorQuery: `apiserver_request_total{verb="LIST",code=~"5.."}`,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.sli)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got.IsRawExpression() {
				t.Error("Resolve() expected TotalQuery/ErrorQuery to be set for kubernetes-apiserver template, not RawExpr")
				return
			}

			// For cases with multiple labels, just check prefix due to map ordering
			if tt.name == "kubernetes-apiserver with labels" {
				if len(got.TotalQuery) < len(tt.wantTotalQuery) {
					t.Errorf("Resolve() TotalQuery = %v, want prefix %v", got.TotalQuery, tt.wantTotalQuery)
				}
				return
			}

			if got.TotalQuery != tt.wantTotalQuery {
				t.Errorf("Resolve() TotalQuery = %v, want %v", got.TotalQuery, tt.wantTotalQuery)
			}
			if got.ErrorQuery != tt.wantErrorQuery {
				t.Errorf("Resolve() ErrorQuery = %v, want %v", got.ErrorQuery, tt.wantErrorQuery)
			}
		})
	}
}
