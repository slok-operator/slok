package validation

import "regexp"

// windowRegex matches Prometheus range vectors like [5m], [7d], [1h], [30s]
var windowRegex = regexp.MustCompile(`\[(\d+[smhdwy])\]`)

// QueryWindowMismatch represents a mismatch between query window and objective window
type QueryWindowMismatch struct {
	QueryWindow     string
	ObjectiveWindow string
}

// ValidateQueryWindow checks if the range windows in the PromQL query match the objective window.
// Returns a list of mismatched windows found in the query.
// This is a warning-level validation - mismatches don't prevent reconciliation.
func ValidateQueryWindow(sliQuery string, objectiveWindow string) []QueryWindowMismatch {
	matches := windowRegex.FindAllStringSubmatch(sliQuery, -1)
	if len(matches) == 0 {
		return nil
	}

	var mismatches []QueryWindowMismatch
	for _, match := range matches {
		if len(match) >= 2 {
			queryWindow := match[1]
			if queryWindow != objectiveWindow {
				mismatches = append(mismatches, QueryWindowMismatch{
					QueryWindow:     queryWindow,
					ObjectiveWindow: objectiveWindow,
				})
			}
		}
	}

	return mismatches
}
