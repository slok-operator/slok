package errorbudget

import (
	"fmt"
	"math"
	"time"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

type Budget struct {
	// Total error budget for the window (e.g., "43.2m" for 43.2 minutes)
	Total string

	// Consumed error budget so far (e.g., "10.5m")
	Consumed string

	// Remaining error budget (e.g., "32.7m")
	Remaining string

	// PercentRemaining is the percentage of budget left (e.g., 75.69)
	PercentRemaining float64
}

func calculatePercentage(target float64, actual float64, window string) (*Budget, error) {
	duration, err := parseWindow(window)
	if err != nil {
		return &Budget{}, err
	}
	totalSeconds := duration.Seconds()
	errorBudgetPercent := 100.0 - target
	errorBudgetSeconds := (errorBudgetPercent / 100.0) * totalSeconds

	actualErrorPercent := 100.0 - actual
	consumedErrorSeconds := (actualErrorPercent / 100.0) * totalSeconds

	remainingErrorSeconds := errorBudgetSeconds - consumedErrorSeconds

	percentRemaining := 0.0

	// Normalize remaining error seconds
	if remainingErrorSeconds < 0 {
		remainingErrorSeconds = 0
	}

	if remainingErrorSeconds > 0 {
		percentRemaining = (remainingErrorSeconds / errorBudgetSeconds) * 100.0
	}

	return &Budget{
		Total:            fmt.Sprintf("%.1fm", errorBudgetSeconds/60.0),
		Consumed:         fmt.Sprintf("%.1fm", consumedErrorSeconds/60.0),
		Remaining:        fmt.Sprintf("%.1fm", remainingErrorSeconds/60.0),
		PercentRemaining: math.Round(percentRemaining*100) / 100,
	}, nil
}

func calculateThreshold(target float64, actual float64, operator string) (*Budget, error) {
	switch operator {
	case "<", "<=":
		if actual >= target {
			percentRemaining := 0.0
			return &Budget{
				Total:            fmt.Sprintf("%.1f", target),
				Consumed:         fmt.Sprintf("%.1f", actual),
				Remaining:        fmt.Sprintf("%.1f", target-actual),
				PercentRemaining: percentRemaining,
			}, nil
		} else {
			percentRemaining := (100.0 * actual) / target
			return &Budget{
				Total:            fmt.Sprintf("%.1f", target),
				Consumed:         fmt.Sprintf("%.1f", actual),
				Remaining:        fmt.Sprintf("%.1f", target-actual),
				PercentRemaining: percentRemaining,
			}, nil
		}
	case ">", ">=":
		if actual <= target {
			percentRemaining := 0.0
			return &Budget{
				Total:            fmt.Sprintf("%.1f", target),
				Consumed:         fmt.Sprintf("%.1f", actual),
				Remaining:        fmt.Sprintf("%.1f", actual-target),
				PercentRemaining: percentRemaining,
			}, nil
		} else {
			percentRemaining := ((actual / target) * 100.0) - 100.0
			return &Budget{
				Total:            fmt.Sprintf("%.1f", target),
				Consumed:         fmt.Sprintf("%.1f", actual),
				Remaining:        fmt.Sprintf("%.1f", actual-target),
				PercentRemaining: percentRemaining,
			}, nil
		}
	}
	return nil, nil
}
func Calculate(obj observabilityv1alpha1.Objective, sliValue float64) (*Budget, error) {
	switch obj.Sli.Type {
	case "threshold":
		return calculateThreshold(obj.Target, sliValue, obj.Sli.Operator)
	default:
		return calculatePercentage(obj.Target, sliValue, obj.Window)
	}
}

// parseWindow converts window string to duration
// Supports: "30d", "7d", "90d", "24h", "60m"
func parseWindow(window string) (time.Duration, error) {
	if len(window) < 2 {
		return 0, fmt.Errorf("window too short")
	}

	// Extract number and unit
	unit := window[len(window)-1:]
	numberStr := window[:len(window)-1]

	var number int
	_, err := fmt.Sscanf(numberStr, "%d", &number)
	if err != nil {
		return 0, fmt.Errorf("invalid number in window: %w", err)
	}

	switch unit {
	case "d":
		return time.Duration(number) * 24 * time.Hour, nil
	case "h":
		return time.Duration(number) * time.Hour, nil
	case "m":
		return time.Duration(number) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unsupported time unit: %s (use d, h, or m)", unit)
	}
}

func DetermineStatus(target float64, actual float64, budgetPercente float64) string {
	if actual < target {
		return "violated"
	}

	if budgetPercente < 10.0 && budgetPercente >= 0.0 {
		return "at-risk"
	}

	return "met"
}
