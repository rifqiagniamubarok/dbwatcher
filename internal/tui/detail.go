package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

func renderDetail(e store.Event, width int) string {
	var b strings.Builder

	title := fmt.Sprintf("  %s %s (id=%v)",
		eventTypeStyle(e.Type).Render(string(e.Type)),
		styleTableName.Render(e.Table),
		primaryKeyValue(e),
	)
	b.WriteString(title + "\n")

	switch e.Type {
	case store.EventInsert:
		renderValues(&b, e.NewValues, e.Columns, width)
	case store.EventDelete:
		renderValues(&b, e.OldValues, e.Columns, width)
	case store.EventUpdate:
		renderDiff(&b, e.OldValues, e.NewValues, e.Columns, width)
	}

	return b.String()
}

func renderValues(b *strings.Builder, vals map[string]any, cols []store.Column, width int) {
	for _, col := range cols {
		v := formatValue(vals[col.Name])
		line := fmt.Sprintf("    %-20s %s", col.Name+":", v)
		b.WriteString(line + "\n")
	}
}

func renderDiff(b *strings.Builder, oldVals, newVals map[string]any, cols []store.Column, width int) {
	colNames := columnNames(cols)
	if len(colNames) == 0 {
		// Fallback when no column metadata.
		keys := allKeys(oldVals, newVals)
		sort.Strings(keys)
		colNames = keys
	}

	// When OldValues is nil the table has no REPLICA IDENTITY FULL — we can
	// only show the new (post-update) state.
	if len(oldVals) == 0 {
		b.WriteString(styleDim.Render("    ⚠  old values unavailable (set REPLICA IDENTITY FULL on this table)") + "\n")
		renderValues(b, newVals, cols, width)
		return
	}

	for _, name := range colNames {
		newV := formatValue(newVals[name])

		// Use "?" when the old value is absent for this column (partial replica
		// identity or TOAST column that wasn't updated).
		var oldV string
		if v, ok := oldVals[name]; ok {
			oldV = formatValue(v)
		} else {
			oldV = "?"
		}

		if oldV == newV {
			line := fmt.Sprintf("    %-20s %s  %s",
				name+":",
				styleDim.Render(oldV),
				styleDim.Render("[unchanged]"),
			)
			b.WriteString(line + "\n")
		} else {
			line := fmt.Sprintf("    %-20s %s  →  %s",
				name+":",
				styleDiffOld.Render(oldV),
				styleDiffNew.Render(newV),
			)
			b.WriteString(line + "\n")
		}
	}
}

func primaryKeyValue(e store.Event) any {
	vals := e.NewValues
	if vals == nil {
		vals = e.OldValues
	}
	for _, col := range e.Columns {
		if col.IsKey {
			if v, ok := vals[col.Name]; ok {
				return v
			}
		}
	}
	return "?"
}

func columnNames(cols []store.Column) []string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return names
}

func allKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}

func formatValue(v any) string {
	if v == nil {
		return styleDim.Render("NULL")
	}
	return fmt.Sprintf("%v", v)
}

func eventTypeStyle(t store.EventType) lipgloss.Style {
	switch t {
	case store.EventInsert:
		return styleInsert
	case store.EventUpdate:
		return styleUpdate
	case store.EventDelete:
		return styleDelete
	}
	return lipgloss.NewStyle()
}
