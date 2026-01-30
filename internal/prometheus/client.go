package prometheus

import (
	"context"
	"fmt"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// PrometheusClient defines the interface for Prometheus operations
type PrometheusClient interface {
	QuerySLI(ctx context.Context, query string) (float64, error)
	CheckConnection(ctx context.Context) error
	QuerySLINotNormalized(ctx context.Context, query string) (float64, error)
}

type Client struct {
	api promv1.API
}

// Ensure Client implements PrometheusClient
var _ PrometheusClient = (*Client)(nil)

func NewClient(prometheusURL string) (*Client, error) {
	client, err := promapi.NewClient(promapi.Config{
		Address: prometheusURL,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		api: promv1.NewAPI(client),
	}, nil
}

func (c *Client) QuerySLI(ctx context.Context, query string) (float64, error) {
	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Prometheus query warnings: %v\n", warnings)
	}
	vectorResult, ok := result.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}
	if len(vectorResult) == 0 {
		return 0, fmt.Errorf("no data returned for query: %s", query)
	}
	value := float64(vectorResult[0].Value)
	// Normalize value to 0-100 scale
	if value >= 0 && value <= 1 {
		value = value * 100
	}
	// Ensure value is in valid range
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return value, nil
}

func (c *Client) QuerySLINotNormalized(ctx context.Context, query string) (float64, error) {
	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Prometheus query warnings: %v\n", warnings)
	}
	vectorResult, ok := result.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}
	if len(vectorResult) == 0 {
		return 0, fmt.Errorf("no data returned for query: %s", query)
	}
	value := float64(vectorResult[0].Value)
	return value, nil
}

// CheckConnection verifies Prometheus is reachable
func (c *Client) CheckConnection(ctx context.Context) error {
	// Simple query to check if Prometheus is up
	_, _, err := c.api.Query(ctx, "up", time.Now())
	return err
}
