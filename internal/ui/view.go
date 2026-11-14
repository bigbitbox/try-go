package ui

import (
	"fmt"
	"os"
	"path/filepath"
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
	ruleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	keyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

func (m *model) View() string {
	var lines []string
	width := max(1, m.width-4)
	rule := ruleStyle.Render(strings.Repeat("─", width))
	switch m.mode {
	case "delete":
		names := m.markedNames()
		lines = []string{
			errorStyle.Render(fmt.Sprintf("Delete %d workspaces", len(names))), rule,
			m.input.View(), mutedStyle.Render("Permanent deletion · symlink targets are kept"),
		}
		end := min(len(names), m.confirmOffset+m.rows())
		for _, name := range names[m.confirmOffset:end] {
			lines = append(lines, errorStyle.Render("× ")+displayText(name))
		}
		for len(lines) < 4+m.rows() {
			lines = append(lines, "")
		}
		lines = append(lines, rule,
			mutedStyle.Render(fmt.Sprintf("%d–%d of %d marked · ↑/↓ or PgUp/PgDn to review", m.confirmOffset+1, end, len(names))),
			m.errorLine(), shortcut("enter", "delete")+"    "+shortcut("esc", "back"))
	case "new", "rename", "graduate":
		title, detail := "New workspace", shortPath(m.cfg.TriesPath)
		hint := "A clean name, without a date prefix"
		if m.cfg.UseDate {
			hint = "A date prefix will be added"
		}
		if m.mode == "rename" {
			title = "Rename workspace"
			detail = m.selected.Basename
			hint = "Enter the new name"
		}
		if m.mode == "graduate" {
			title = "Graduate workspace"
			detail = m.selected.Basename
			hint = "Move to this path · leave a link in the original location"
		}
		lines = []string{titleStyle.Render(title), rule, mutedStyle.Render(displayText(detail)), "",
			m.input.View(), mutedStyle.Render(hint), "", m.errorLine(), rule,
			shortcut("enter", "confirm") + "    " + shortcut("esc", "back")}
		if m.height < len(lines) {
			// On short terminals, input and validation outrank decorative spacing.
			lines = []string{m.input.View(), m.errorLine()}
			if m.height >= 3 {
				lines = append(lines, shortcut("enter", "confirm")+"  "+shortcut("esc", "back"))
			}
			if m.height >= 4 {
				lines = append([]string{titleStyle.Render(title)}, lines...)
			}
			if m.height == 1 && m.err != "" {
				lines = []string{m.errorLine()}
			}
		}
	default:
		path := mutedStyle.Render(displayText(shortPath(m.cfg.TriesPath)))
		lines = []string{joinSides(titleStyle.Render("try")+"  "+path,
			mutedStyle.Render(fmt.Sprintf("%d workspaces", len(m.entries))), width, lipgloss.NewStyle()), rule,
			m.search.View(), rule}
		end := min(len(m.results), m.offset+m.rows())
		now := time.Now()
		for i := m.offset; i < end; i++ {
			result := m.results[i]
			entry := m.entries[result.ID]
			style := lipgloss.NewStyle()
			pointer := "  "
			if i == m.cursor {
				pointer, style = "› ", selectedStyle
			}
			mark := ""
			if len(m.marks) > 0 {
				mark = style.Render("  ")
				if m.marks[entry.Basename] {
					mark = style.Foreground(lipgloss.Color("203")).Render("× ")
				}
			}
			name := highlightedName(result, style)
			if entry.IsSymlink {
				name += style.Foreground(lipgloss.Color("245")).Render(" ↗")
			}
			age := workspace.FormatRelativeTime(entry.ModTime, now)
			ageStyle := style
			if i != m.cursor {
				ageStyle = mutedStyle
			}
			row := joinSides(style.Render(pointer)+mark+name, ageStyle.Render(age+" "), width, style)
			lines = append(lines, row)
		}
		if len(m.results) == 0 {
			message := "No workspaces yet · Ctrl-T to create one"
			if m.search.Value() != "" {
				message = "No matches · Enter to create " + displayText(m.search.Value())
			}
			lines = append(lines, mutedStyle.Render(message))
		}
		for len(lines) < 4+m.rows() {
			lines = append(lines, "")
		}
		position := 0
		if len(m.results) > 0 {
			position = m.cursor + 1
		}
		status := fmt.Sprintf("%d / %d", position, len(m.results))
		if len(m.marks) > 0 {
			status += fmt.Sprintf(" · %d marked · Enter review · Esc clear marks", len(m.marks))
		} else if m.err == "" {
			status += " · recent first"
		}
		if m.err != "" {
			status += "  " + m.errorLine()
		}
		primaryHelp, secondaryHelp := listHelp(width)
		lines = append(lines, rule, mutedStyle.Render(status), primaryHelp, secondaryHelp)
	}
	for i, line := range lines {
		// Cell-aware clipping keeps wide Unicode names and ANSI styles intact.
		if m.width > 4 {
			line = "  " + ansi.Truncate(line, width, "…")
		}
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

// Keep metadata aligned; drop it on small terminals to preserve the name.
func joinSides(left, right string, width int, fill lipgloss.Style) string {
	if width < 36 || lipgloss.Width(right)+4 >= width {
		left = ansi.Truncate(left, width, fill.Render("…"))
		return left + fill.Render(strings.Repeat(" ", max(0, width-lipgloss.Width(left))))
	}
	room := width - lipgloss.Width(right) - 2
	left = ansi.Truncate(left, room, fill.Render("…"))
	return left + fill.Render(strings.Repeat(" ", max(2, width-lipgloss.Width(left)-lipgloss.Width(right)))) + right
}

func listHelp(width int) (string, string) {
	if width < 20 {
		return keyStyle.Render("enter / esc"), mutedStyle.Render("^T ^R ^G ^D")
	}
	if width < 32 {
		return shortcut("↑↓", "move") + "  " + keyStyle.Render("enter / esc"), shortcut("^T", "new") + "  " + keyStyle.Render("^R  ^G  ^D")
	}
	if width < 46 {
		return shortcut("↑↓", "move") + "  " + shortcut("enter", "open") + "  " + shortcut("esc", "quit"),
			shortcut("^T", "new") + " " + shortcut("^R", "rename") + " " + shortcut("^G", "move") + " " + shortcut("^D", "del")
	}
	gap := "    "
	deleteLabel := "mark delete"
	if width < 70 {
		gap, deleteLabel = "  ", "delete"
	}
	return shortcut("↑↓", "move") + gap + shortcut("enter", "open/create") + gap + shortcut("^T", "new"),
		shortcut("^R", "rename") + gap + shortcut("^G", "graduate") + gap + shortcut("^D", deleteLabel) + gap + shortcut("esc", "cancel")
}

func shortcut(key, label string) string {
	return keyStyle.Render(key) + " " + mutedStyle.Render(label)
}

func shortPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(filepath.Separator)) {
			return "~" + strings.TrimPrefix(path, home)
		}
	}
	return path
}

func (m *model) errorLine() string {
	if m.err == "" {
		return ""
	}
	return errorStyle.Render("Error: " + displayText(m.err))
}

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

// Match positions refer to runes, not bytes; slicing UTF-8 would corrupt names.
func highlightedName(result fuzzy.Result, base lipgloss.Style) string {
	var b strings.Builder
	positions := result.HighlightPositions
	next := 0
	for i, r := range []rune(result.Text) {
		style := base
		if next < len(positions) && positions[next] == i {
			style = base.Bold(true).Underline(true).Foreground(lipgloss.Color("117"))
			next++
		}
		b.WriteString(style.Render(displayText(string(r))))
	}
	return b.String()
}
