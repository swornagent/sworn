package bench

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/verdict"
)

// Report is the full benchmark report.
type Report struct {
	Models []ModelEntry `json:"models"`
	Tasks  []string     `json:"-"`
	Cells  []CellResult `json:"cells"`
}

// Table returns a formatted plain-text table of model × task results with
// summary columns for pass-rate, total cost, and jurisdiction.
//
// Columns: model_id | jurisdiction | task1 ... taskN | pass-rate | total_cost
func Table(r *Report) string {
	if len(r.Models) == 0 {
		return "(no models in report)"
	}

	taskNames := r.Tasks

	// Build cell lookup: modelID × taskName → CellResult.
	cells := make(map[string]map[string]CellResult)
	for _, c := range r.Cells {
		if cells[c.ModelID] == nil {
			cells[c.ModelID] = make(map[string]CellResult)
		}
		cells[c.ModelID][c.TaskName] = c
	}

	// Sort models by pass-rate descending for readable output.
	type modelRow struct {
		entry        ModelEntry
		passRate     float64
		totalCost    float64
		jurisdiction string
	}
	var rows []modelRow
	for _, m := range r.Models {
		var passed, total int
		var cost float64
		for _, tn := range taskNames {
			c, ok := cells[m.ModelID][tn]
			if !ok {
				continue
			}
			total++
			if c.Verdict == verdict.Pass {
				passed++
			}
			cost += c.CostUSD
		}
		rate := 0.0
		if total > 0 {
			rate = float64(passed) / float64(total) * 100
		}
		rows = append(rows, modelRow{
			entry:        m,
			passRate:     rate,
			totalCost:    cost,
			jurisdiction: jurisdiction(m.Provider),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].passRate != rows[j].passRate {
			return rows[i].passRate > rows[j].passRate
		}
		return rows[i].totalCost < rows[j].totalCost
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%-24s %-14s", "model_id", "jurisdiction"))
	for _, tn := range taskNames {
		name := tn
		if len(name) > 22 {
			name = name[:19] + "..."
		}
		b.WriteString(fmt.Sprintf(" %-24s", name))
	}
	b.WriteString(fmt.Sprintf(" %-10s %-10s\n", "pass-rate", "total_cost"))

	// Separator line.
	sepLen := 24 + 1 + 14 + 1 + len(taskNames)*25 + 1 + 10 + 1 + 10
	b.WriteString(strings.Repeat("-", sepLen) + "\n")

	for _, row := range rows {
		b.WriteString(fmt.Sprintf("%-24s %-14s", row.entry.ModelID, row.jurisdiction))
		for _, tn := range taskNames {
			c, ok := cells[row.entry.ModelID][tn]
			if !ok {
				b.WriteString(fmt.Sprintf(" %-24s", "-"))
				continue
			}
			cell := string(c.Verdict)
			if c.Error != "" {
				cell = "ERR"
			}
			b.WriteString(fmt.Sprintf(" %-24s", cell))
		}
		b.WriteString(fmt.Sprintf(" %5.0f%%     ", row.passRate))
		b.WriteString(fmt.Sprintf("$%-9.4f", row.totalCost))
		b.WriteString("\n")
	}

	return b.String()
}

// JSONReport returns the report as an indented JSON object.
func JSONReport(r *Report) (string, error) {
	type jsonSummary struct {
		ModelID      string  `json:"model_id"`
		Jurisdiction string  `json:"jurisdiction"`
		PassRate     float64 `json:"pass_rate"`
		TotalCost    float64 `json:"total_cost_usd"`
	}

	cells := make(map[string]map[string]CellResult)
	for _, c := range r.Cells {
		if cells[c.ModelID] == nil {
			cells[c.ModelID] = make(map[string]CellResult)
		}
		cells[c.ModelID][c.TaskName] = c
	}

	var summaries []jsonSummary
	for _, m := range r.Models {
		var passed, total int
		var cost float64
		for _, tn := range r.Tasks {
			c, ok := cells[m.ModelID][tn]
			if !ok {
				continue
			}
			total++
			if c.Verdict == verdict.Pass {
				passed++
			}
			cost += c.CostUSD
		}
		rate := 0.0
		if total > 0 {
			rate = float64(passed) / float64(total) * 100
		}
		summaries = append(summaries, jsonSummary{
			ModelID:      m.ModelID,
			Jurisdiction: jurisdiction(m.Provider),
			PassRate:     rate,
			TotalCost:    cost,
		})
	}

	type jsonReport struct {
		Models    []ModelEntry  `json:"models"`
		TaskNames []string      `json:"task_names"`
		Cells     []CellResult  `json:"cells"`
		Summary   []jsonSummary `json:"summary"`
	}

	jr := jsonReport{
		Models:    r.Models,
		TaskNames: r.Tasks,
		Cells:     r.Cells,
		Summary:   summaries,
	}

	data, err := json.MarshalIndent(jr, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// jurisdiction returns the hosting jurisdiction for a provider. Currently
// only "openai" (US, trusted) is known.
func jurisdiction(provider string) string {
	switch provider {
	case "openai":
		return "US (trusted)"
	default:
		return "unknown"
	}
}
