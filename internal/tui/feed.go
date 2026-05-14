package tui

import (
	"fmt"
	"strings"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

func renderFeed(events []store.Event, cursor int, expanded bool, width int) string {
	if len(events) == 0 {
		return styleDim.Render("  Waiting for events...")
	}

	var b strings.Builder
	for i, e := range events {
		line := formatFeedLine(e, width)
		if i == cursor {
			b.WriteString(styleCursor.Render(fmt.Sprintf("%-*s", width, line)))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")

		if i == cursor && expanded {
			b.WriteString(renderDetail(e, width))
		}
	}
	return b.String()
}

func formatFeedLine(e store.Event, width int) string {
	ts := styleTimestamp.Render(e.Timestamp.Format("15:04:05.000"))
	typStr := eventTypeStyle(e.Type).Render(fmt.Sprintf("%-6s", string(e.Type)))
	table := styleTableName.Render(e.Table)
	summary := formatSummary(e)
	return fmt.Sprintf("  %s  %s  %s  %s", ts, typStr, table, summary)
}

func formatSummary(e store.Event) string {
	switch e.Type {
	case store.EventInsert:
		if id := primaryKeyValue(e); id != "?" {
			return styleDim.Render(fmt.Sprintf("id=%v", id))
		}
	case store.EventUpdate:
		// Show first changed field: old → new
		// When OldValues is absent (no REPLICA IDENTITY FULL) only show new value.
		for _, col := range e.Columns {
			newV := formatValue(e.NewValues[col.Name])
			if col.IsKey {
				continue
			}
			if len(e.OldValues) == 0 {
				// No old-value info — just show the new value for the first non-key column.
				return fmt.Sprintf("%s %s",
					col.Name,
					styleDiffNew.Render(truncate(newV, 16)),
				)
			}
			oldV := formatValue(e.OldValues[col.Name])
			if oldV != newV {
				return fmt.Sprintf("%s %s → %s",
					col.Name,
					styleDiffOld.Render(truncate(oldV, 12)),
					styleDiffNew.Render(truncate(newV, 12)),
				)
			}
		}
	case store.EventDelete:
		if id := primaryKeyValue(e); id != "?" {
			return styleDim.Render(fmt.Sprintf("id=%v", id))
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
