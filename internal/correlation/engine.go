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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultWindowBefore is how far back to look for changes before the spike
	DefaultWindowBefore = 30 * time.Minute
	// DefaultWindowAfter is how far forward to look for changes after the spike
	DefaultWindowAfter = 10 * time.Minute
	// HighConfidenceWindow is the window for high confidence correlation
	HighConfidenceWindow = 15 * time.Minute
	// MediumConfidenceWindow is the window for medium confidence correlation
	MediumConfidenceWindow = 60 * time.Minute
)

// CorrelationEngine analyzes changes and determines correlations
type CorrelationEngine struct {
	collector *ChangeCollector
}

// NewCorrelationEngine creates a new correlation engine
func NewCorrelationEngine(collector *ChangeCollector) *CorrelationEngine {
	return &CorrelationEngine{
		collector: collector,
	}
}

// ScoredChange is a ChangeRecord with a confidence score
type ScoredChange struct {
	Record     ChangeRecord
	Confidence string
	Score      int // internal score for sorting
}

// Analyze performs correlation analysis for a burn rate spike
func (e *CorrelationEngine) Analyze(
	sloName, sloNamespace string,
	triggerTime time.Time,
	burnRate float64,
	previousBurnRate float64,
	severity string,
	workloadSelector *observabilityv1alpha1.WorkloadSelector,
) *observabilityv1alpha1.SLOCorrelation {
	// Define analysis window
	windowStart := triggerTime.Add(-DefaultWindowBefore)
	windowEnd := triggerTime.Add(DefaultWindowAfter)

	// Get all changes in window
	allChanges := e.collector.GetChangesInWindow(windowStart, windowEnd)

	// Filter changes based on workload selector
	filteredChanges := e.filterBySelector(allChanges, sloNamespace, workloadSelector)

	// Filter and score changes
	var workloadLabels map[string]string
	if workloadSelector != nil {
		workloadLabels = workloadSelector.LabelSelector
	}
	scored := e.scoreChanges(filteredChanges, triggerTime, sloNamespace, workloadLabels)

	// Sort by score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Convert to CorrelatedEvents
	correlatedEvents := make([]observabilityv1alpha1.CorrelatedEvent, 0, len(scored))
	for _, sc := range scored {
		correlatedEvents = append(correlatedEvents, observabilityv1alpha1.CorrelatedEvent{
			Kind:       sc.Record.Kind,
			Name:       sc.Record.Name,
			Namespace:  sc.Record.Namespace,
			Timestamp:  metav1.NewTime(sc.Record.Timestamp),
			ChangeType: sc.Record.ChangeType,
			Change:     sc.Record.Diff,
			Actor:      sc.Record.Actor,
			Confidence: sc.Confidence,
		})
	}

	// Generate summary
	summary := e.generateSummary(scored, severity)

	// Create correlation name
	correlationName := fmt.Sprintf("%s-%s", sloName, triggerTime.Format("2006-01-02-1504"))

	// If GROQ_API_KEY is set, use LLM to refine the summary
	if apiKey := os.Getenv("GROQ_API_KEY"); apiKey != "" && len(filteredChanges) > 0 {
		llmSummary := e.queryLLM(
			apiKey, sloName, sloNamespace,
			triggerTime, burnRate, previousBurnRate, severity,
			windowStart, windowEnd, filteredChanges,
		)
		if llmSummary != "" {
			summary = llmSummary
		}
	}

	return &observabilityv1alpha1.SLOCorrelation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      correlationName,
			Namespace: sloNamespace,
		},
		Spec: observabilityv1alpha1.SLOCorrelationSpec{
			SLORef: observabilityv1alpha1.SLOReference{
				Name:      sloName,
				Namespace: sloNamespace,
			},
		},
		Status: observabilityv1alpha1.SLOCorrelationStatus{
			DetectedAt:          metav1.NewTime(triggerTime),
			BurnRateAtDetection: burnRate,
			PreviousBurnRate:    previousBurnRate,
			Severity:            severity,
			Window: observabilityv1alpha1.TimeWindow{
				Start: metav1.NewTime(windowStart),
				End:   metav1.NewTime(windowEnd),
			},
			CorrelatedEvents: correlatedEvents,
			Summary:          summary,
			EventCount:       len(correlatedEvents),
		},
	}
}

// scoreChanges assigns confidence scores to changes
func (e *CorrelationEngine) scoreChanges(
	changes []ChangeRecord,
	triggerTime time.Time,
	sloNamespace string,
	workloadLabels map[string]string,
) []ScoredChange {
	scored := make([]ScoredChange, 0, len(changes))

	for _, change := range changes {
		score := 0
		var confidence string

		// Time-based scoring
		timeDiff := triggerTime.Sub(change.Timestamp)
		if timeDiff < 0 {
			timeDiff = -timeDiff // Change happened after spike
		}

		if timeDiff <= HighConfidenceWindow {
			score += 30
		} else if timeDiff <= MediumConfidenceWindow {
			score += 15
		} else {
			score += 5
		}

		// Namespace scoring
		if change.Namespace == sloNamespace {
			score += 20
		}

		// Kind scoring - Deployments and ConfigMaps are more likely causes
		switch change.Kind {
		case "Deployment":
			score += 25
		case "ConfigMap":
			score += 20
		case "Secret":
			score += 15
		case "Event":
			// Events are consequences, not causes (usually)
			if strings.Contains(change.Diff, "OOMKilled") || strings.Contains(change.Diff, "CrashLoopBackOff") {
				score += 10 // But these indicate problems
			} else {
				score -= 5
			}
		}

		// Label matching scoring
		if len(workloadLabels) > 0 && matchLabels(change.Labels, workloadLabels) {
			score += 30
		}

		// Determine confidence level based on score
		if score >= 50 {
			confidence = observabilityv1alpha1.ConfidenceHigh
		} else if score >= 25 {
			confidence = observabilityv1alpha1.ConfidenceMedium
		} else {
			confidence = observabilityv1alpha1.ConfidenceLow
		}

		scored = append(scored, ScoredChange{
			Record:     change,
			Confidence: confidence,
			Score:      score,
		})
	}

	return scored
}

// generateSummary creates a human-readable summary of the correlation
func (e *CorrelationEngine) generateSummary(scored []ScoredChange, severity string) string {
	if len(scored) == 0 {
		return fmt.Sprintf("Burn rate spike detected (%s severity), but no cluster changes found in the analysis window.", severity)
	}

	// Count by confidence
	highCount := 0
	var highConfidence []string
	for _, sc := range scored {
		if sc.Confidence == observabilityv1alpha1.ConfidenceHigh {
			highCount++
			highConfidence = append(highConfidence, fmt.Sprintf("%s/%s", sc.Record.Kind, sc.Record.Name))
		}
	}

	if highCount == 0 {
		return fmt.Sprintf("Burn rate spike detected (%s severity). Found %d changes in the analysis window, but none with high correlation confidence.", severity, len(scored))
	}

	if highCount == 1 {
		top := scored[0]
		return fmt.Sprintf("Burn rate spike (%s) likely caused by %s %s/%s: %s",
			severity, top.Record.Kind, top.Record.Namespace, top.Record.Name, top.Record.Diff)
	}

	// Multiple high-confidence changes
	return fmt.Sprintf("Burn rate spike (%s) correlates with %d high-confidence changes: %s",
		severity, highCount, strings.Join(highConfidence[:min(3, len(highConfidence))], ", "))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// filterBySelector filters changes based on the workload selector
func (e *CorrelationEngine) filterBySelector(
	changes []ChangeRecord,
	sloNamespace string,
	selector *observabilityv1alpha1.WorkloadSelector,
) []ChangeRecord {
	// If no selector, only filter by SLO namespace
	if selector == nil {
		filtered := make([]ChangeRecord, 0, len(changes))
		for _, change := range changes {
			if change.Namespace == sloNamespace {
				filtered = append(filtered, change)
			}
		}
		return filtered
	}

	// Build allowed namespaces set
	allowedNamespaces := make(map[string]bool)
	if len(selector.Namespaces) > 0 {
		for _, ns := range selector.Namespaces {
			allowedNamespaces[ns] = true
		}
	} else {
		// Default to SLO namespace only
		allowedNamespaces[sloNamespace] = true
	}

	filtered := make([]ChangeRecord, 0, len(changes))
	for _, change := range changes {
		// Check namespace
		if !allowedNamespaces[change.Namespace] {
			continue
		}

		// If label selector is specified, check labels
		if len(selector.LabelSelector) > 0 {
			// For Events, we can't match labels directly (they don't have workload labels)
			// Include them if they're in the right namespace and mention a matching resource
			if change.Kind == "Event" {
				// Events are included if namespace matches - they'll be scored lower anyway
				filtered = append(filtered, change)
				continue
			}

			// For other resources, match labels
			if !matchLabels(change.Labels, selector.LabelSelector) {
				continue
			}
		}

		filtered = append(filtered, change)
	}

	return filtered
}

// queryLLM calls Groq API to get an LLM-refined summary of the correlation
func (e *CorrelationEngine) queryLLM(
	apiKey string,
	sloName, sloNamespace string,
	triggerTime time.Time,
	burnRate, previousBurnRate float64,
	severity string,
	windowStart, windowEnd time.Time,
	changes []ChangeRecord,
) string {
	systemPrompt := `You are a helpful assistant for correlating SLO burn rate spikes with Kubernetes cluster changes. ` +
		`Analyze the provided changes and determine which ones are most likely the root cause of the burn rate spike ` +
		`based on timing, resource type, and relevance. ` +
		`If there exists an event that intentionally brings capacity to zero, that one always wins over probe failures, pod churn, and secondary errors. ` +
		`Do NOT trust the confidence levels - they may not be accurate. ` +
		`Respond with ONLY a single summary sentence identifying the most likely root cause.`

	userPrompt := fmt.Sprintf(
		"An SLO burn rate spike was detected for SLO '%s' in namespace '%s' at %s with burn rate %.2f (previous: %.2f). "+
			"Severity: %s. Analysis window: %s to %s.\n\nCluster changes:\n%s",
		sloName, sloNamespace,
		triggerTime.Format(time.RFC3339), burnRate, previousBurnRate, severity,
		windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339),
		formatChanges(changes),
	)

	body, err := json.Marshal(map[string]interface{}{
		"model": "llama-3.3-70b-versatile",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.1,
	})
	if err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.groq.com/openai/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if len(result.Choices) > 0 {
		return strings.TrimSpace(result.Choices[0].Message.Content)
	}
	return ""
}

// formatChanges formats a list of changes for the LLM prompt
func formatChanges(changes []ChangeRecord) string {
	var sb strings.Builder
	for _, c := range changes {
		sb.WriteString(fmt.Sprintf("- [%s] %s %s/%s (%s) by %s: %s\n",
			c.Timestamp.Format(time.RFC3339),
			c.ChangeType, c.Namespace, c.Name,
			c.Kind, c.Actor, c.Diff,
		))
	}
	return sb.String()
}
