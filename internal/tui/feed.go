package tui

import (
	"fmt"
	"strings"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// tsLayout is the timestamp format used across all feed line renderers.
const tsLayout = "15:04:05.000"

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
	switch {
	case e.IsMarker():
		return formatMarkerLine(e, width)
	case e.IsLog():
		return formatLogLine(e)
	case e.IsDDL():
		return formatDDLLine(e)
	}
	ts := styleTimestamp.Render(e.Timestamp.Format(tsLayout))
	typStr := eventTypeStyle(e.Type).Render(fmt.Sprintf("%-6s", string(e.Type)))
	table := styleTableName.Render(e.Table)
	summary := formatSummary(e)
	return fmt.Sprintf("  %s  %s  %s  %s", ts, typStr, table, summary)
}

// formatMarkerLine renders a separator line:
//
//	──────────────── TEST: create order ────────────────
//
// The line stretches to fill `width` so it visually divides the feed.
func formatMarkerLine(e store.Event, width int) string {
	style := markerColorStyle(e.Color)
	label := e.Label
	if label == "" {
		label = "marker"
	}
	// Leading "  " (the same gutter regular feed lines use) + label + spaces.
	// Total chrome around the label: 2 (gutter) + 4 ("── " on each side) + len(label).
	const minDashes = 4
	chrome := 2 + (minDashes+1)*2 + len(label)
	dashCount := width - chrome
	if dashCount < minDashes {
		dashCount = minDashes
	}
	dashes := strings.Repeat("─", dashCount/2)
	body := fmt.Sprintf("%s %s %s", dashes, label, dashes)
	return "  " + style.Render(body)
}

// formatLogLine renders an inline log entry without a separator:
//
//	14:32:05.123  [log]  Starting test suite
func formatLogLine(e store.Event) string {
	ts := styleTimestamp.Render(e.Timestamp.Format(tsLayout))
	tag := styleLog.Render("[log]")
	return fmt.Sprintf("  %s  %s  %s", ts, tag, e.Message)
}

// formatDDLLine renders a schema-change event in a distinct (magenta) color:
//
//	14:33:10.456  ⚡ DDL   ALTER TABLE   public.users
func formatDDLLine(e store.Event) string {
	ts := styleTimestamp.Render(e.Timestamp.Format(tsLayout))
	tag := styleDDL.Render(fmt.Sprintf("%-6s", "⚡ DDL"))
	cmd := styleDDL.Render(fmt.Sprintf("%-13s", e.CommandTag))
	return fmt.Sprintf("  %s  %s  %s  %s", ts, tag, cmd, styleTableName.Render(e.ObjectIdentity))
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
