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
	"sync"
	"time"
)

const (
	// BurnRateCriticalThreshold is the threshold for critical severity
	BurnRateCriticalThreshold = 14.0
	// BurnRateDegradedThreshold is the threshold for degraded severity
	BurnRateDegradedThreshold = 6.0
	// BurnRateWarningThreshold is the threshold for warning severity
	BurnRateWarningThreshold = 1.0
	// DefaultCooldown is the minimum time between correlations for the same SLO
	DefaultCooldown = 2 * time.Hour
)

// BurnRateState tracks the burn rate state for an SLO
type BurnRateState struct {
	PreviousBurnRate float64
	LastCorrelation  time.Time
}

// AnomalyDetector detects burn rate spikes and manages cooldowns
type AnomalyDetector struct {
	states   map[string]*BurnRateState // key: namespace/name
	cooldown time.Duration
	mu       sync.RWMutex
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{
		states:   make(map[string]*BurnRateState),
		cooldown: DefaultCooldown,
	}
}

// NewAnomalyDetectorWithCooldown creates a detector with custom cooldown
func NewAnomalyDetectorWithCooldown(cooldown time.Duration) *AnomalyDetector {
	return &AnomalyDetector{
		states:   make(map[string]*BurnRateState),
		cooldown: cooldown,
	}
}

// SpikeResult contains the result of a spike detection
type SpikeResult struct {
	Detected         bool
	Severity         string
	CurrentBurnRate  float64
	PreviousBurnRate float64
	CrossedThreshold float64
}

// DetectSpike checks if a burn rate spike occurred and should trigger correlation
func (d *AnomalyDetector) DetectSpike(sloNamespace, sloName string, currentBurnRate float64) *SpikeResult {
	key := sloNamespace + "/" + sloName

	d.mu.Lock()
	defer d.mu.Unlock()

	state, exists := d.states[key]
	if !exists {
		// First time seeing this SLO, just record the state
		d.states[key] = &BurnRateState{
			PreviousBurnRate: currentBurnRate,
		}
		return &SpikeResult{Detected: false, CurrentBurnRate: currentBurnRate}
	}

	previousBurnRate := state.PreviousBurnRate

	// Update state with current burn rate
	state.PreviousBurnRate = currentBurnRate

	// Check cooldown
	if time.Since(state.LastCorrelation) < d.cooldown {
		return &SpikeResult{
			Detected:         false,
			CurrentBurnRate:  currentBurnRate,
			PreviousBurnRate: previousBurnRate,
		}
	}

	// Detect threshold crossings
	result := &SpikeResult{
		CurrentBurnRate:  currentBurnRate,
		PreviousBurnRate: previousBurnRate,
	}

	// Check critical threshold (>14x)
	if currentBurnRate > BurnRateCriticalThreshold && previousBurnRate <= BurnRateCriticalThreshold {
		result.Detected = true
		result.Severity = "critical"
		result.CrossedThreshold = BurnRateCriticalThreshold
		state.LastCorrelation = time.Now()
		return result
	}

	// Check degraded threshold (>6x)
	if currentBurnRate > BurnRateDegradedThreshold && previousBurnRate <= BurnRateDegradedThreshold {
		result.Detected = true
		result.Severity = "degraded"
		result.CrossedThreshold = BurnRateDegradedThreshold
		state.LastCorrelation = time.Now()
		return result
	}

	// Check warning threshold (>1x)
	if currentBurnRate > BurnRateWarningThreshold && previousBurnRate <= BurnRateWarningThreshold {
		result.Detected = true
		result.Severity = "warning"
		result.CrossedThreshold = BurnRateWarningThreshold
		state.LastCorrelation = time.Now()
		return result
	}

	return result
}

// GetState returns the current state for an SLO (for testing)
func (d *AnomalyDetector) GetState(sloNamespace, sloName string) *BurnRateState {
	key := sloNamespace + "/" + sloName

	d.mu.RLock()
	defer d.mu.RUnlock()

	if state, exists := d.states[key]; exists {
		// Return a copy
		return &BurnRateState{
			PreviousBurnRate: state.PreviousBurnRate,
			LastCorrelation:  state.LastCorrelation,
		}
	}
	return nil
}

// ResetCooldown resets the cooldown for an SLO (mainly for testing)
func (d *AnomalyDetector) ResetCooldown(sloNamespace, sloName string) {
	key := sloNamespace + "/" + sloName

	d.mu.Lock()
	defer d.mu.Unlock()

	if state, exists := d.states[key]; exists {
		state.LastCorrelation = time.Time{}
	}
}

// Clear removes all state (mainly for testing)
func (d *AnomalyDetector) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.states = make(map[string]*BurnRateState)
}
