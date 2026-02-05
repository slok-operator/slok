package burnrate

import (
	"math"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

type BurnRate struct {
	LongBurnRate float64

	ShortBurnRate float64

	LongWindow string

	ShortWindow string

	Status string
}

func Calculate(obj observabilityv1alpha1.Objective, sliShortWindow float64, sliLongWindow float64) (BurnRate, error) {
	burnShortRate := math.Round(((1-sliShortWindow)/(1-obj.Target/100))*100) / 100
	burnLongRate := math.Round(((1-sliLongWindow)/(1-obj.Target/100))*100) / 100
	// sloWindowduration, err := parseWindow(obj.Window)
	// if err != nil {
	// 	return BurnRate{}, err
	// }
	// //burnRateWindow, err := parseWindow(obj.Alerting.BurnRateAlerts[0].Window)
	// if err != nil {
	// 	return BurnRate{}, err
	// }
	// burnRateThreshold := (obj.Alerting.BurnRateAlerts[0].ConsumePercent / 100) * float64(sloWindowduration.Hours()) / float64(burnRateWindow.Hours())
	return BurnRate{
		LongBurnRate:  burnLongRate,
		ShortBurnRate: burnShortRate,
		Status:        "true",
	}, nil
}

// parseWindow converts window string to duration
// Supports: "30d", "7d", "90d", "24h", "60m"
// func parseWindow(window string) (time.Duration, error) {
// 	if len(window) < 2 {
// 		return 0, fmt.Errorf("window too short")
// 	}

// 	// Extract number and unit
// 	unit := window[len(window)-1:]
// 	numberStr := window[:len(window)-1]

// 	var number int
// 	_, err := fmt.Sscanf(numberStr, "%d", &number)
// 	if err != nil {
// 		return 0, fmt.Errorf("invalid number in window: %w", err)
// 	}

// 	switch unit {
// 	case "d":
// 		return time.Duration(number) * 24 * time.Hour, nil
// 	case "h":
// 		return time.Duration(number) * time.Hour, nil
// 	case "m":
// 		return time.Duration(number) * time.Minute, nil
// 	default:
// 		return 0, fmt.Errorf("unsupported time unit: %s (use d, h, or m)", unit)
// 	}
// }

func DetermineStatus(target float64, actual float64, budgetPercente float64) string {
	if actual < target {
		return "violated"
	}

	if budgetPercente < 10.0 && budgetPercente >= 0.0 {
		return "at-risk"
	}

	return "met"
}
