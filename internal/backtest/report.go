package backtest

import (
	"fmt"
	"io"
	"strings"
)

// Print writes the backtest result to w.
// Single target → detailed view. Multiple targets → what-if comparison table.
func Print(w io.Writer, r *Result) {
	if len(r.Targets) == 1 {
		printSingle(w, r)
	} else {
		printTable(w, r)
	}
}

func printSingle(w io.Writer, r *Result) {
	tr := r.Targets[0]
	writef(w, "SLO:    %s/%s\n", r.SLOName, r.ObjectiveName)
	writef(w, "NS:     %s\n", r.Namespace)
	writef(w, "Window: %s\n", r.Range)
	if r.Source != "" {
		writef(w, "Source: %s\n", r.Source)
	}
	writef(w, "Target: %.2f%%\n\n", tr.Target)
	writef(w, "Historical result:\n")
	writef(w, "  - Availability:        %.4f%%\n", tr.Availability)
	writef(w, "  - Error budget burned: %.2f%%\n", tr.BudgetBurned)
	writef(w, "  - Budget remaining:    %.2f%%\n", tr.BudgetRemaining)
	writef(w, "  - Status:              %s\n", tr.Status)
}

func printTable(w io.Writer, r *Result) {
	writef(w, "SLO: %s/%s  |  Namespace: %s  |  Window: %s\n",
		r.SLOName, r.ObjectiveName, r.Namespace, r.Range)
	if r.Source != "" {
		writef(w, "Source: %s\n", r.Source)
	}
	writef(w, "\n")

	const colFmt = "%-10s  %-14s  %-18s  %s\n"
	header := fmt.Sprintf(colFmt, "Target", "Availability", "Budget remaining", "Result")
	writef(w, "%s", header)
	writef(w, "%s\n", strings.Repeat("-", len(header)-1))
	for _, tr := range r.Targets {
		writef(w, colFmt,
			fmt.Sprintf("%.2f%%", tr.Target),
			fmt.Sprintf("%.4f%%", tr.Availability),
			fmt.Sprintf("%.2f%%", tr.BudgetRemaining),
			tr.Status,
		)
	}
}

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
