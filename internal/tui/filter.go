package tui

import (
	"fmt"
	"strings"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

func renderSidebar(tables []string, active map[string]bool, cursor int, focused bool, height int) string {
	var b strings.Builder

	title := "Tables"
	if focused {
		title = styleSelected.Render(title)
	} else {
		title = styleHeader.Render(title)
	}
	b.WriteString(title + "\n")

	for i, t := range tables {
		check := "[ ]"
		if active[t] {
			check = "[x]"
		}

		line := fmt.Sprintf("%s %s", check, t)

		if focused && i == cursor {
			line = styleSelected.Render(line)
		} else if active[t] {
			line = styleInsert.Render(fmt.Sprintf("[x] %s", t))
		} else {
			line = styleDim.Render(fmt.Sprintf("[ ] %s", t))
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

func addTable(tables []string, name string) []string {
	for _, t := range tables {
		if t == name {
			return tables
		}
	}
	return append(tables, name)
}

// buildFilterMap returns the set of active (visible) tables.
func buildFilterMap(tables []string, active map[string]bool) map[string]bool {
	result := make(map[string]bool, len(tables))
	for _, t := range tables {
		if v, ok := active[t]; !ok || v {
			result[t] = true
		}
	}
	return result
}

func sidebarWidth(tables []string) int {
	w := 14
	for _, t := range tables {
		if len(t)+5 > w {
			w = len(t) + 5
		}
	}
	if w > 24 {
		w = 24
	}
	return w
}

// filterEvents returns only events whose table is enabled.
func filterEvents(events []store.Event, active map[string]bool) []store.Event {
	if len(active) == 0 {
		return events
	}
	out := make([]store.Event, 0, len(events))
	for _, e := range events {
		if active[e.Table] {
			out = append(out, e)
		}
	}
	return out
}
