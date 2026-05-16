package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// Focus indicates which panel has keyboard focus.
type Focus int

const (
	FocusFeed Focus = iota
	FocusFilter
)

// EventMsg carries a new event from the Store into the Bubble Tea update loop.
type EventMsg store.Event

// Model is the top-level Bubble Tea model.
type Model struct {
	// All events received (unfiltered).
	allEvents []store.Event
	// Tables seen so far and their active state (true = visible).
	tables []string
	active map[string]bool

	// Feed state.
	cursor   int
	expanded bool
	paused   bool
	pending  []store.Event // events received while paused

	// Filter sidebar state.
	focus        Focus
	filterCursor int

	// Confirm-clear state.
	confirmClear bool

	// Show help overlay.
	showHelp bool

	// Layout.
	width  int
	height int

	// Connection info for header.
	dbTarget string
}

// New creates the initial model.
func New(dbTarget string) Model {
	return NewWithEvents(dbTarget, nil)
}

// NewWithEvents creates a model preloaded with initial events.
func NewWithEvents(dbTarget string, events []store.Event) Model {
	active := make(map[string]bool)
	tables := make([]string, 0)
	all := make([]store.Event, 0, len(events))

	for _, e := range events {
		all = append(all, e)
		if e.IsDBEvent() && e.Table != "" {
			tables = addTable(tables, e.Table)
			active[e.Table] = true
		}
	}

	cursor := 0
	if len(all) > 0 {
		cursor = len(all) - 1
	}

	return Model{
		allEvents: all,
		tables:    tables,
		active:    active,
		cursor:    cursor,
		dbTarget:  dbTarget,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case EventMsg:
		e := store.Event(msg)
		if e.IsDBEvent() && e.Table != "" {
			m.tables = addTable(m.tables, e.Table)
			if _, seen := m.active[e.Table]; !seen {
				m.active[e.Table] = true
			}
		}

		if m.paused {
			m.pending = append(m.pending, e)
		} else {
			m.allEvents = append(m.allEvents, e)
			if !m.paused {
				// Auto-scroll: keep cursor at bottom if it was already there.
				visible := m.visibleEvents()
				if m.cursor >= len(visible)-2 {
					m.cursor = max(0, len(visible)-1)
				}
			}
		}

	case tea.KeyMsg:
		// Confirm-clear mode: second 'c' confirms, anything else cancels.
		if m.confirmClear {
			if msg.String() == "c" {
				m.allEvents = nil
				m.pending = nil
				m.cursor = 0
				m.expanded = false
			}
			m.confirmClear = false
			return m, nil
		}

		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		switch m.focus {
		case FocusFeed:
			return m.updateFeed(msg)
		case FocusFilter:
			return m.updateFilter(msg)
		}
	}

	return m, nil
}

func (m Model) updateFeed(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visible := m.visibleEvents()

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.cursor < len(visible)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "g":
		m.cursor = 0
		m.expanded = false

	case "G":
		m.cursor = max(0, len(visible)-1)

	case "enter":
		if len(visible) > 0 {
			m.expanded = !m.expanded
		}

	case " ":
		m.paused = !m.paused
		if !m.paused && len(m.pending) > 0 {
			m.allEvents = append(m.allEvents, m.pending...)
			m.pending = nil
			m.cursor = max(0, len(m.visibleEvents())-1)
		}

	case "f":
		m.focus = FocusFilter
		m.filterCursor = 0

	case "c":
		m.confirmClear = true

	case "?":
		m.showHelp = true

	case "[":
		// Jump cursor to the previous marker in the visible feed.
		for i := m.cursor - 1; i >= 0; i-- {
			if visible[i].IsMarker() {
				m.cursor = i
				m.expanded = false
				break
			}
		}

	case "]":
		// Jump cursor to the next marker in the visible feed.
		for i := m.cursor + 1; i < len(visible); i++ {
			if visible[i].IsMarker() {
				m.cursor = i
				m.expanded = false
				break
			}
		}

	case "M":
		// Drop every item that arrived before the most-recent marker —
		// useful for cleaning out noise between two test runs.
		lastMarkerIdx := -1
		for i := len(m.allEvents) - 1; i >= 0; i-- {
			if m.allEvents[i].IsMarker() {
				lastMarkerIdx = i
				break
			}
		}
		if lastMarkerIdx > 0 {
			m.allEvents = append([]store.Event{}, m.allEvents[lastMarkerIdx:]...)
			m.cursor = 0
			m.expanded = false
		}
	}

	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "f", "esc":
		m.focus = FocusFeed

	case "j", "down":
		if m.filterCursor < len(m.tables)-1 {
			m.filterCursor++
		}

	case "k", "up":
		if m.filterCursor > 0 {
			m.filterCursor--
		}

	case " ":
		if len(m.tables) > 0 {
			t := m.tables[m.filterCursor]
			m.active[t] = !m.active[t]
			// Clamp cursor to visible events after filter change.
			visible := m.visibleEvents()
			if m.cursor >= len(visible) {
				m.cursor = max(0, len(visible)-1)
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	if m.showHelp {
		return m.helpView()
	}

	header := m.headerView()
	footer := m.footerView()

	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	sidebar := ""
	feedWidth := m.width
	if len(m.tables) > 0 {
		sw := sidebarWidth(m.tables)
		sidebar = styleSidebar.
			Width(sw).
			Height(bodyHeight).
			Render(renderSidebar(m.tables, m.active, m.filterCursor, m.focus == FocusFilter, bodyHeight))
		feedWidth = m.width - sw - 2
	}

	visible := m.visibleEvents()
	feedContent := renderFeed(visible, m.cursor, m.expanded, feedWidth)

	// Trim to bodyHeight lines.
	feedLines := strings.Split(feedContent, "\n")
	// Show last bodyHeight lines (most recent events visible).
	start := 0
	if !m.expanded && len(feedLines) > bodyHeight {
		start = len(feedLines) - bodyHeight
	}
	if start < 0 {
		start = 0
	}
	feedTrimmed := strings.Join(feedLines[start:], "\n")

	feedView := lipgloss.NewStyle().
		Width(feedWidth).
		Height(bodyHeight).
		Render(feedTrimmed)

	var body string
	if len(m.tables) > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, feedView)
	} else {
		body = feedView
	}

	if m.confirmClear {
		body = lipgloss.Place(m.width, bodyHeight, lipgloss.Center, lipgloss.Center,
			styleUpdate.Render("Press 'c' again to clear, any other key to cancel"),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) headerView() string {
	status := styleInsert.Render("▶ live")
	if m.paused {
		pendingStr := ""
		if len(m.pending) > 0 {
			pendingStr = fmt.Sprintf(" (%d new)", len(m.pending))
		}
		status = styleUpdate.Render("⏸ paused" + pendingStr)
	}

	total := fmt.Sprintf("%d events", len(m.allEvents)+len(m.pending))
	db := styleTableName.Render(m.dbTarget)

	left := fmt.Sprintf(" dbwatch — %s  •  %s  •  %s", db, status, total)
	right := styleTimestamp.Render("press ? for help ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	line := left + strings.Repeat(" ", gap) + right
	return styleHeader.Copy().
		Width(m.width).
		Background(lipgloss.AdaptiveColor{Light: "#e8e8e8", Dark: "#1a1a2e"}).
		Render(line)
}

func (m Model) footerView() string {
	hints := "space:pause  j/k:nav  [/]:marker  enter:expand  f:filter  c:clear  q:quit"
	if m.focus == FocusFilter {
		hints = "j/k:nav  space:toggle  f/esc:back  q:quit"
	}
	return styleFooter.Copy().Width(m.width).Render(hints)
}

func (m Model) helpView() string {
	help := `
  Keybindings

  Feed
    j / ↓       move down
    k / ↑       move up
    g           jump to oldest
    G           jump to newest
    enter       expand/collapse detail
    space       pause / resume
    [           jump to previous marker
    ]           jump to next marker
    M           clear feed up to last marker

  Filter
    f           toggle filter sidebar focus
    j/k         navigate tables
    space       toggle table visibility
    esc / f     back to feed

  General
    c           clear feed (press c again to confirm)
    ?           toggle this help
    q / Ctrl+C  quit

  Press any key to close.
`
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			Render(help),
	)
}

func (m Model) visibleEvents() []store.Event {
	return filterEvents(m.allEvents, m.active)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
