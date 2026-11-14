package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/bigbitbox/try-go/internal/fuzzy"
	"github.com/bigbitbox/try-go/internal/workspace"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	matchStyle    = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("212"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

func (m *model) View() string {
	var lines []string
	switch m.mode {
	case "delete":
		names := m.markedNames()
		lines = []string{titleStyle.Render(fmt.Sprintf("Delete %d marked workspaces", len(names))), m.input.View(), "All marked names (including hidden matches):"}
		end := min(len(names), m.confirmOffset+m.rows())
		for _, name := range names[m.confirmOffset:end] {
			lines = append(lines, "  "+displayText(name))
		}
		for len(lines) < 3+m.rows() {
			lines = append(lines, "")
		}
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("%d–%d of %d • ↑/↓ or PgUp/PgDn to review", m.confirmOffset+1, end, len(names))), m.errorLine(), "Enter confirm • Esc back • Ctrl-C cancel")
	case "new", "rename", "graduate":
		title := "New workspace"
		detail := "Enter a workspace name"
		if m.mode == "rename" {
			title = "Rename " + displayText(m.selected.Basename)
			detail = "Enter the new workspace name"
		}
		if m.mode == "graduate" {
			title = "Graduate " + displayText(m.selected.Basename)
			detail = "Enter the destination path"
		}
		lines = []string{titleStyle.Render(title), mutedStyle.Render(detail), m.input.View(), "", m.errorLine(), "Enter accept • Esc back • Ctrl-C cancel"}
	default:
		lines = []string{titleStyle.Render("try · workspaces"), m.search.View(), ""}
		end := min(len(m.results), m.offset+m.rows())
		now := time.Now()
		for i := m.offset; i < end; i++ {
			result := m.results[i]
			entry := m.entries[result.ID]
			pointer := "  "
			if i == m.cursor {
				pointer = selectedStyle.Render("› ")
			}
			mark := "[ ] "
			if m.marks[entry.Basename] {
				mark = selectedStyle.Render("[×] ")
			}
			name := highlightedName(result)
			suffix := ""
			if entry.IsSymlink {
				suffix = " ↗"
			}
			age := "  " + workspace.FormatRelativeTime(entry.ModTime, now)
			lines = append(lines, pointer+mark+name+mutedStyle.Render(suffix+age))
		}
		if len(m.results) == 0 {
			lines = append(lines, mutedStyle.Render("No matches. Enter to create a workspace."))
		}
		for len(lines) < 3+m.rows() {
			lines = append(lines, "")
		}
		position := 0
		if len(m.results) > 0 {
			position = m.cursor + 1
		}
		status := fmt.Sprintf("%d/%d matches • %d marked", position, len(m.results), len(m.marks))
		if len(m.marks) > 0 {
			status += " • Enter review deletion • Esc clear marks"
		}
		lines = append(lines, mutedStyle.Render(status), m.errorLine(), "↑/↓ move • Enter open/create • Ctrl-T new", "Ctrl-R rename • Ctrl-G graduate • Ctrl-D mark • Esc cancel")
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

func (m *model) errorLine() string {
	if m.err == "" {
		return ""
	}
	return errorStyle.Render("Error: " + m.err)
}

// Match positions refer to runes, not bytes; slicing UTF-8 would corrupt names.
func displayText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			q := strconv.QuoteRuneToGraphic(r)
			b.WriteString(q[1 : len(q)-1])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func highlightedName(result fuzzy.Result) string {
	var b strings.Builder
	positions := result.HighlightPositions
	next := 0
	for i, r := range []rune(result.Text) {
		text := displayText(string(r))
		if next < len(positions) && positions[next] == i {
			text = matchStyle.Render(text)
			next++
		}
		b.WriteString(text)
	}
	return b.String()
}
