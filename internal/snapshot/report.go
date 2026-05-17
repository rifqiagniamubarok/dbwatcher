package snapshot

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Report output formats.
const (
	FormatText     = "text"
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
)

// FormatReport renders a CompareReport in the requested format.
// An unknown format falls back to text.
func FormatReport(r *CompareReport, format string) (string, error) {
	switch format {
	case FormatJSON:
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal report: %w", err)
		}
		return string(b), nil
	case FormatMarkdown:
		return renderMarkdown(r), nil
	default:
		return renderText(r), nil
	}
}

func renderText(r *CompareReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== Snapshot %q vs %q ===\n\n", r.FromLabel, r.ToLabel)

	b.WriteString("SCHEMA CHANGES\n")
	if len(r.SchemaChanges) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, c := range r.SchemaChanges {
		marker := "  "
		if c.Breaking {
			marker = "⚠ "
		}
		target := c.Table
		if c.Column != "" {
			target += "." + c.Column
		}
		fmt.Fprintf(&b, "%s%-7s %-28s %s\n", marker, c.Kind, target, c.Detail)
	}

	b.WriteString("\nDATA CHANGES\n")
	if len(r.DataChanges) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, dc := range r.DataChanges {
		fmt.Fprintf(&b, "  %s:\n", dc.Table)
		fmt.Fprintf(&b, "    rows:           %d → %d\n", dc.RowCountFrom, dc.RowCountTo)
		if dc.Note != "" {
			fmt.Fprintf(&b, "    (%s)\n", dc.Note)
		}
		for _, cc := range dc.Columns {
			flag := ""
			if cc.Notable {
				flag = "  ⚠"
			}
			fmt.Fprintf(&b, "    %s non-null:  %d → %d%s\n", cc.Column, cc.NonNullFrom, cc.NonNullTo, flag)
			if cc.NullFrom != cc.NullTo {
				fmt.Fprintf(&b, "    %s null:      %d → %d\n", cc.Column, cc.NullFrom, cc.NullTo)
			}
		}
	}

	fmt.Fprintf(&b, "\nSUMMARY\n")
	fmt.Fprintf(&b, "  %d schema change(s), %d breaking\n", len(r.SchemaChanges), r.BreakingCount())
	fmt.Fprintf(&b, "  %d table(s) with data drift, %d notable\n", len(r.DataChanges), r.NotableDataCount())
	return b.String()
}

func renderMarkdown(r *CompareReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Snapshot compare: `%s` → `%s`\n\n", r.FromLabel, r.ToLabel)

	b.WriteString("### Schema changes\n\n")
	if len(r.SchemaChanges) == 0 {
		b.WriteString("_No schema changes._\n\n")
	} else {
		b.WriteString("| Change | Target | Detail | Breaking |\n")
		b.WriteString("| --- | --- | --- | --- |\n")
		for _, c := range r.SchemaChanges {
			target := c.Table
			if c.Column != "" {
				target += "." + c.Column
			}
			breaking := ""
			if c.Breaking {
				breaking = "⚠ yes"
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n", c.Kind, target, c.Detail, breaking)
		}
		b.WriteString("\n")
	}

	b.WriteString("### Data changes\n\n")
	if len(r.DataChanges) == 0 {
		b.WriteString("_No data drift._\n\n")
	} else {
		for _, dc := range r.DataChanges {
			fmt.Fprintf(&b, "- **`%s`** — rows %d → %d", dc.Table, dc.RowCountFrom, dc.RowCountTo)
			if dc.Note != "" {
				fmt.Fprintf(&b, " _(%s)_", dc.Note)
			}
			b.WriteString("\n")
			for _, cc := range dc.Columns {
				flag := ""
				if cc.Notable {
					flag = " ⚠"
				}
				fmt.Fprintf(&b, "  - `%s`: non-null %d → %d, null %d → %d%s\n",
					cc.Column, cc.NonNullFrom, cc.NonNullTo, cc.NullFrom, cc.NullTo, flag)
			}
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "### Summary\n\n")
	fmt.Fprintf(&b, "- %d schema change(s), **%d breaking**\n", len(r.SchemaChanges), r.BreakingCount())
	fmt.Fprintf(&b, "- %d table(s) with data drift, **%d notable**\n", len(r.DataChanges), r.NotableDataCount())
	return b.String()
}
