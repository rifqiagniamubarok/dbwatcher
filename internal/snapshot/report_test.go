package snapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleReport() *CompareReport {
	return &CompareReport{
		FromLabel: "pre-migration",
		ToLabel:   "now",
		SchemaChanges: []SchemaChange{
			{Kind: ChangeModified, Table: "public.users", Column: "phone", Detail: "NULL → NOT NULL", Breaking: true},
			{Kind: ChangeAdded, Table: "public.orders", Column: "note", Detail: "column added (text, nullable)"},
		},
		DataChanges: []DataChange{
			{
				Table: "public.users", RowCountFrom: 100, RowCountTo: 100,
				Columns: []ColumnDataChange{
					{Column: "phone", NonNullFrom: 97, NonNullTo: 100, NullFrom: 3, NullTo: 0, Notable: true},
				},
			},
		},
	}
}

func TestFormatReport_Text(t *testing.T) {
	out, err := FormatReport(sampleReport(), FormatText)
	require.NoError(t, err)
	assert.Contains(t, out, "SCHEMA CHANGES")
	assert.Contains(t, out, "DATA CHANGES")
	assert.Contains(t, out, "SUMMARY")
	assert.Contains(t, out, "NULL → NOT NULL")
	assert.Contains(t, out, "⚠") // breaking + notable markers
	assert.Contains(t, out, "1 breaking")
}

func TestFormatReport_JSON(t *testing.T) {
	out, err := FormatReport(sampleReport(), FormatJSON)
	require.NoError(t, err)

	var parsed CompareReport
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, "pre-migration", parsed.FromLabel)
	assert.Len(t, parsed.SchemaChanges, 2)
	assert.Equal(t, 1, parsed.BreakingCount())
}

func TestFormatReport_Markdown(t *testing.T) {
	out, err := FormatReport(sampleReport(), FormatMarkdown)
	require.NoError(t, err)
	assert.Contains(t, out, "## Snapshot compare")
	assert.Contains(t, out, "| Change | Target | Detail | Breaking |")
	assert.Contains(t, out, "**1 breaking**")
}

func TestFormatReport_UnknownFormatFallsBackToText(t *testing.T) {
	out, err := FormatReport(sampleReport(), "yaml-please")
	require.NoError(t, err)
	assert.Contains(t, out, "SCHEMA CHANGES") // text marker
}

func TestFormatReport_EmptyReport(t *testing.T) {
	empty := &CompareReport{FromLabel: "a", ToLabel: "b"}
	for _, format := range []string{FormatText, FormatJSON, FormatMarkdown} {
		out, err := FormatReport(empty, format)
		require.NoError(t, err)
		assert.NotEmpty(t, out)
		if format != FormatJSON {
			assert.True(t,
				strings.Contains(out, "none") || strings.Contains(out, "No "),
				"format %s should indicate emptiness", format)
		}
	}
}
