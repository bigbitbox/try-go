package ui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bigbitbox/try-go/internal/workspace"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestHelpRemainsVisibleInNarrowTerminals(t *testing.T) {
	for width := 12; width <= 120; width++ {
		first, second := listHelp(width)
		if !strings.Contains(first+second, "esc") {
			t.Fatalf("cancel hint missing at width %d", width)
		}
		for _, line := range []string{first, second} {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("help needs %d cells in a %d-cell area: %q", got, width, line)
			}
		}
	}
}

func TestSelectedBackgroundSurvivesNestedStyles(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(profile)
	for _, width := range []int{30, 80} {
		for _, name := range []string{"demo", "中文-demo", strings.Repeat("demo-", 30)} {
			for _, query := range []string{"", "d"} {
				m := newModel(workspace.Config{}, []workspace.Entry{{Basename: name, IsSymlink: true}}, query)
				m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
				m.marks[name] = true
				row := strings.Split(m.View(), "\n")[4]
				assertRowBackground(t, row, width-4)
			}
		}
	}
}

// Interpret emitted SGR transitions rather than checking for the presence of a color code.
func assertRowBackground(t *testing.T, row string, width int) {
	t.Helper()
	tokens := regexp.MustCompile(`\x1b\[[0-9;]*m|[^\x1b]+`).FindAllString(row, -1)
	column, background := 0, -1
	for _, token := range tokens {
		if strings.HasPrefix(token, "\x1b[") {
			parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(token, "\x1b["), "m"), ";")
			for i := 0; i < len(parts); i++ {
				code, _ := strconv.Atoi(parts[i])
				switch code {
				case 0, 49:
					background = -1
				case 38, 48:
					if i+2 < len(parts) && parts[i+1] == "5" {
						if code == 48 {
							background, _ = strconv.Atoi(parts[i+2])
						}
						i += 2
					}
				}
			}
			continue
		}
		for _, r := range token {
			if column >= 2 && column < 2+width && background != 24 {
				t.Fatalf("selected background lost at column %d (%q): %q", column, r, row)
			}
			column += lipgloss.Width(string(r))
		}
	}
	if column < 2+width {
		t.Fatalf("selected row ends at %d, want at least %d", column, 2+width)
	}
}

func TestShortFormsKeepInputAndErrorVisible(t *testing.T) {
	for _, height := range []int{4, 5, 6, 7, 8, 9} {
		for _, mode := range []string{"new", "rename", "graduate"} {
			m := newModel(workspace.Config{}, sampleEntries(), "")
			m.Update(tea.WindowSizeMsg{Width: 80, Height: height})
			m.openForm(mode, "")
			m.submit()
			view := m.View()
			if !strings.Contains(view, "Error:") || !strings.Contains(view, "> ") || !strings.Contains(view, "esc") {
				t.Fatalf("%s form at height %d hid input/error/cancel: %q", mode, height, view)
			}
			if len(strings.Split(view, "\n")) > height {
				t.Fatalf("form exceeds height %d", height)
			}
		}
	}
}
