/*
Copyright 2026 Federico Le Pera.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package correlation

import (
	"context"
	"fmt"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// DefaultBufferSize is the default number of changes to keep in memory
	DefaultBufferSize = 1000
	// DefaultRetentionDuration is how long to keep changes
	DefaultRetentionDuration = 4 * time.Hour
)

// ChangeRecord represents a single change event in the cluster
type ChangeRecord struct {
	Timestamp  time.Time
	Kind       string
	Name       string
	Namespace  string
	ChangeType string // "create", "update", "delete"
	Diff       string // human-readable diff (e.g., "image: v1.4.2 → v1.4.3")
	Actor      string // who triggered the change
	Labels     map[string]string
}

// ChangeCollector watches Kubernetes resources and records changes in a ring buffer
type ChangeCollector struct {
	client            client.Client
	buffer            []ChangeRecord
	bufferSize        int
	retentionDuration time.Duration
	mu                sync.RWMutex
	head              int // next write position
	count             int // number of valid entries
}

// NewChangeCollector creates a new ChangeCollector with default settings
func NewChangeCollector() *ChangeCollector {
	return &ChangeCollector{
		buffer:            make([]ChangeRecord, DefaultBufferSize),
		bufferSize:        DefaultBufferSize,
		retentionDuration: DefaultRetentionDuration,
	}
}

// NewChangeCollectorWithOptions creates a ChangeCollector with custom settings
func NewChangeCollectorWithOptions(bufferSize int, retention time.Duration) *ChangeCollector {
	return &ChangeCollector{
		buffer:            make([]ChangeRecord, bufferSize),
		bufferSize:        bufferSize,
		retentionDuration: retention,
	}
}

// SetupWithManager registers the collector to watch resources
func (c *ChangeCollector) SetupWithManager(mgr ctrl.Manager) error {
	c.client = mgr.GetClient()

	// Create a dummy reconciler that just records changes
	return ctrl.NewControllerManagedBy(mgr).
		Named("change-collector").
		// Watch Deployments
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(c.handleDeployment)).
		// Watch ConfigMaps (excluding system ones)
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(c.handleConfigMap)).
		// Watch Secrets (excluding system ones)
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(c.handleSecret)).
		// Watch Events for CrashLoopBackOff, OOMKilled, etc.
		Watches(&corev1.Event{}, handler.EnqueueRequestsFromMapFunc(c.handleEvent)).
		Complete(&noopReconciler{})
}

// noopReconciler is a reconciler that does nothing - we only use the watches
type noopReconciler struct{}

func (r *noopReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// handleDeployment processes Deployment changes
func (c *ChangeCollector) handleDeployment(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	deployment, ok := obj.(*appsv1.Deployment)
	if !ok {
		return nil
	}

	// Skip system namespaces
	if isSystemNamespace(deployment.Namespace) {
		return nil
	}

	diff := ""
	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		diff = fmt.Sprintf("image: %s", deployment.Spec.Template.Spec.Containers[0].Image)
	}

	actor := extractActor(deployment.Annotations)

	record := ChangeRecord{
		Timestamp:  time.Now(),
		Kind:       "Deployment",
		Name:       deployment.Name,
		Namespace:  deployment.Namespace,
		ChangeType: "update",
		Diff:       diff,
		Actor:      actor,
		Labels:     deployment.Labels,
	}

	c.addRecord(record)
	logger.V(1).Info("Recorded deployment change", "name", deployment.Name, "namespace", deployment.Namespace)

	return nil
}

// handleConfigMap processes ConfigMap changes
func (c *ChangeCollector) handleConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}

	// Skip system namespaces and system configmaps
	if isSystemNamespace(cm.Namespace) || isSystemConfigMap(cm.Name) {
		return nil
	}

	diff := fmt.Sprintf("keys: %d", len(cm.Data))
	actor := extractActor(cm.Annotations)

	record := ChangeRecord{
		Timestamp:  time.Now(),
		Kind:       "ConfigMap",
		Name:       cm.Name,
		Namespace:  cm.Namespace,
		ChangeType: "update",
		Diff:       diff,
		Actor:      actor,
		Labels:     cm.Labels,
	}

	c.addRecord(record)
	logger.V(1).Info("Recorded configmap change", "name", cm.Name, "namespace", cm.Namespace)

	return nil
}

// handleSecret processes Secret changes (without exposing content)
func (c *ChangeCollector) handleSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	// Skip system namespaces and service account tokens
	if isSystemNamespace(secret.Namespace) || secret.Type == corev1.SecretTypeServiceAccountToken {
		return nil
	}

	// Don't expose secret content, just note that it changed
	diff := fmt.Sprintf("type: %s, keys: %d", secret.Type, len(secret.Data))
	actor := extractActor(secret.Annotations)

	record := ChangeRecord{
		Timestamp:  time.Now(),
		Kind:       "Secret",
		Name:       secret.Name,
		Namespace:  secret.Namespace,
		ChangeType: "update",
		Diff:       diff,
		Actor:      actor,
		Labels:     secret.Labels,
	}

	c.addRecord(record)
	logger.V(1).Info("Recorded secret change", "name", secret.Name, "namespace", secret.Namespace)

	return nil
}

// handleEvent processes Kubernetes Events (CrashLoopBackOff, OOMKilled, etc.)
func (c *ChangeCollector) handleEvent(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	event, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}

	// Only track significant events
	if !isSignificantEvent(event) {
		return nil
	}

	record := ChangeRecord{
		Timestamp:  time.Now(),
		Kind:       "Event",
		Name:       event.InvolvedObject.Name,
		Namespace:  event.Namespace,
		ChangeType: "create",
		Diff:       fmt.Sprintf("%s: %s", event.Reason, event.Message),
		Actor:      event.Source.Component,
		Labels:     nil,
	}

	c.addRecord(record)
	logger.V(1).Info("Recorded event", "reason", event.Reason, "name", event.InvolvedObject.Name)

	return nil
}

// addRecord adds a change record to the ring buffer
func (c *ChangeCollector) addRecord(record ChangeRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.buffer[c.head] = record
	c.head = (c.head + 1) % c.bufferSize
	if c.count < c.bufferSize {
		c.count++
	}
}

// GetChangesInWindow returns all changes within the specified time window
func (c *ChangeCollector) GetChangesInWindow(start, end time.Time) []ChangeRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []ChangeRecord
	cutoff := time.Now().Add(-c.retentionDuration)

	for i := 0; i < c.count; i++ {
		idx := (c.head - 1 - i + c.bufferSize) % c.bufferSize
		record := c.buffer[idx]

		// Skip records older than retention
		if record.Timestamp.Before(cutoff) {
			continue
		}

		// Check if within window
		if record.Timestamp.After(start) && record.Timestamp.Before(end) {
			results = append(results, record)
		}
	}

	return results
}

// GetChangesForNamespace returns changes filtered by namespace
func (c *ChangeCollector) GetChangesForNamespace(namespace string, start, end time.Time) []ChangeRecord {
	changes := c.GetChangesInWindow(start, end)
	var filtered []ChangeRecord
	for _, change := range changes {
		if change.Namespace == namespace {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

// GetChangesForLabels returns changes filtered by label selector
func (c *ChangeCollector) GetChangesForLabels(labels map[string]string, start, end time.Time) []ChangeRecord {
	changes := c.GetChangesInWindow(start, end)
	var filtered []ChangeRecord
	for _, change := range changes {
		if matchLabels(change.Labels, labels) {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

// Stats returns statistics about the collector
func (c *ChangeCollector) Stats() (count int, oldest time.Time, newest time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.count == 0 {
		return 0, time.Time{}, time.Time{}
	}

	oldest = time.Now()
	newest = time.Time{}

	for i := 0; i < c.count; i++ {
		idx := (c.head - 1 - i + c.bufferSize) % c.bufferSize
		record := c.buffer[idx]
		if record.Timestamp.Before(oldest) {
			oldest = record.Timestamp
		}
		if record.Timestamp.After(newest) {
			newest = record.Timestamp
		}
	}

	return c.count, oldest, newest
}

// Helper functions

func isSystemNamespace(ns string) bool {
	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
		"default":         false, // default is not system
	}
	return systemNamespaces[ns]
}

func isSystemConfigMap(name string) bool {
	// Skip leader election and other system configmaps
	systemPrefixes := []string{"kube-", "extension-apiserver-"}
	for _, prefix := range systemPrefixes {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func isSignificantEvent(event *corev1.Event) bool {
	significantReasons := map[string]bool{
		"CrashLoopBackOff":    true,
		"OOMKilled":           true,
		"FailedScheduling":    true,
		"Unhealthy":           true,
		"BackOff":             true,
		"Failed":              true,
		"FailedCreate":        true,
		"FailedMount":         true,
		"NodeNotReady":        true,
		"Evicted":             true,
		"ContainerKilled":     true,
		"DeadlineExceeded":    true,
		"ImagePullBackOff":    true,
		"ErrImagePull":        true,
		"ScalingReplicaSet":   true,
		"SuccessfulRescale":   true,
		"SuccessfulCreate":    true,
		"SuccessfulDelete":    true,
	}
	return significantReasons[event.Reason]
}

func extractActor(annotations map[string]string) string {
	// Try common annotations that indicate who made the change
	actorAnnotations := []string{
		"kubectl.kubernetes.io/last-applied-configuration",
		"kubernetes.io/change-cause",
		"argocd.argoproj.io/sync-logged",
		"meta.helm.sh/release-name",
	}

	for _, key := range actorAnnotations {
		if val, ok := annotations[key]; ok && val != "" {
			// For last-applied-configuration, just note it was kubectl
			if key == "kubectl.kubernetes.io/last-applied-configuration" {
				return "kubectl"
			}
			return val
		}
	}
	return "unknown"
}

func matchLabels(recordLabels, selectorLabels map[string]string) bool {
	if len(selectorLabels) == 0 {
		return true
	}
	for key, value := range selectorLabels {
		if recordLabels[key] != value {
			return false
		}
	}
	return true
}

// Ensure HPA is imported for future use
var _ = &autoscalingv1.HorizontalPodAutoscaler{}
var _ = types.NamespacedName{}
