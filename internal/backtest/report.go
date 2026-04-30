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
	fmt.Fprintf(w, "SLO:    %s/%s\n", r.SLOName, r.ObjectiveName)
	fmt.Fprintf(w, "NS:     %s\n", r.Namespace)
	fmt.Fprintf(w, "Window: %s\n", r.Range)
	fmt.Fprintf(w, "Target: %.2f%%\n\n", tr.Target)
	fmt.Fprintf(w, "Historical result:\n")
	fmt.Fprintf(w, "  - Availability:        %.4f%%\n", tr.Availability)
	fmt.Fprintf(w, "  - Error budget burned: %.2f%%\n", tr.BudgetBurned)
	fmt.Fprintf(w, "  - Budget remaining:    %.2f%%\n", tr.BudgetRemaining)
	fmt.Fprintf(w, "  - Status:              %s\n", tr.Status)
}

func printTable(w io.Writer, r *Result) {
	fmt.Fprintf(w, "SLO: %s/%s  |  Namespace: %s  |  Window: %s\n\n",
		r.SLOName, r.ObjectiveName, r.Namespace, r.Range)

	const colFmt = "%-10s  %-14s  %-18s  %s\n"
	header := fmt.Sprintf(colFmt, "Target", "Availability", "Budget remaining", "Result")
	fmt.Fprint(w, header)
	fmt.Fprintln(w, strings.Repeat("-", len(header)-1))
	for _, tr := range r.Targets {
		fmt.Fprintf(w, colFmt,
			fmt.Sprintf("%.2f%%", tr.Target),
			fmt.Sprintf("%.4f%%", tr.Availability),
			fmt.Sprintf("%.2f%%", tr.BudgetRemaining),
			tr.Status,
		)
	}
}
