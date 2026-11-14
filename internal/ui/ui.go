// Package ui presents workspace actions without executing them.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/bigbitbox/try-go/internal/fuzzy"
	"github.com/bigbitbox/try-go/internal/workspace"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Action describes work for the caller to execute after the terminal is restored.
// An empty Kind means the user canceled.
type Action struct {
	Kind        string
	Entry       workspace.Entry
	Name        string
	Destination string
	Names       []string
}

type model struct {
	cfg            workspace.Config
	entries        []workspace.Entry
	matcher        *fuzzy.Matcher
	results        []fuzzy.Result
	search         textinput.Model
	input          textinput.Model
	mode           string
	selected       workspace.Entry
	err            string
	cursor, offset int
	width, height  int
	confirmOffset  int
	marks          map[string]bool
	action         Action
}

// Run loads the workspace list and returns a selected action. Terminal output is
// restricted to stderr so the caller can reserve stdout for shell integration.
func Run(cfg workspace.Config, query string) (Action, error) {
	// Shell integration captures stdout; color capability must follow the TUI on stderr.
	renderer := lipgloss.NewRenderer(os.Stderr)
	lipgloss.DefaultRenderer().SetOutput(renderer.Output())
	lipgloss.SetColorProfile(renderer.ColorProfile())
	entries, err := workspace.LoadEntries(cfg.TriesPath, time.Now())
	if err != nil {
		return Action{}, fmt.Errorf("load workspaces: %w", err)
	}
	m := newModel(cfg, entries, query)
	_, err = tea.NewProgram(m, tea.WithOutput(os.Stderr), tea.WithAltScreen()).Run()
	if err != nil {
		return Action{}, err
	}
	return m.action, nil
}

func newModel(cfg workspace.Config, entries []workspace.Entry, query string) *model {
	candidates := make([]fuzzy.Candidate, len(entries))
	for i, e := range entries {
		candidates[i] = fuzzy.Candidate{ID: i, Text: e.Basename, BaseScore: e.BaseScore}
	}
	search := textinput.New()
	search.Prompt = "Search > "
	search.SetValue(query)
	search.CursorEnd()
	search.Focus()
	m := &model{cfg: cfg, entries: entries, matcher: fuzzy.New(candidates), search: search, marks: make(map[string]bool), width: 80, height: 24}
	m.results = m.matcher.Match(search.Value(), 0)
	m.resizeInputs()
	return m
}

func (m *model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = max(1, size.Width)
		m.height = max(1, size.Height)
		m.resizeInputs()
		m.keepVisible()
		m.confirmOffset = min(m.confirmOffset, max(0, len(m.marks)-m.rows()))
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		if k.Type == tea.KeyRunes && strings.ContainsFunc(string(k.Runes), unicode.IsControl) {
			m.err = "Input must contain no control characters"
			return m, nil
		}
		if k.Type == tea.KeyCtrlC {
			m.action = Action{}
			return m, tea.Quit
		}
		if m.mode != "" {
			switch k.Type {
			case tea.KeyEsc:
				m.mode = ""
				m.err = ""
				m.input.Blur()
				return m, m.search.Focus()
			case tea.KeyEnter:
				return m, m.submit()
			}
			if m.mode == "delete" && m.navigate(k, true) {
				return m, nil
			}
		} else {
			if m.navigate(k, false) {
				return m, nil
			}
			switch k.Type {
			case tea.KeyEsc:
				if len(m.marks) > 0 {
					clear(m.marks)
					m.err = ""
					return m, nil
				}
				return m, tea.Quit
			case tea.KeyCtrlD:
				if len(m.results) == 0 {
					m.err = "No workspace selected"
					return m, nil
				}
				name := m.results[m.cursor].Text
				if m.marks[name] {
					delete(m.marks, name)
				} else {
					m.marks[name] = true
				}
				m.err = ""
				return m, nil
			case tea.KeyCtrlT:
				return m, m.openForm("new", m.search.Value())
			case tea.KeyCtrlR, tea.KeyCtrlG:
				if len(m.results) == 0 {
					m.err = "No workspace selected"
					return m, nil
				}
				m.selected = m.entries[m.results[m.cursor].ID]
				if k.Type == tea.KeyCtrlR {
					return m, m.openForm("rename", workspace.StripDatePrefix(m.selected.Basename))
				}
				return m, m.openForm("graduate", filepath.Join(m.cfg.ProjectsPath, workspace.StripDatePrefix(m.selected.Basename)))
			case tea.KeyEnter:
				if len(m.marks) > 0 {
					return m, m.openForm("delete", "")
				}
				if len(m.results) > 0 {
					m.action = Action{Kind: "cd", Entry: m.entries[m.results[m.cursor].ID]}
					return m, tea.Quit
				}
				m.openForm("new", m.search.Value())
				return m, m.submit()
			}
		}
	}
	var cmd tea.Cmd
	if m.mode != "" {
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	before := m.search.Value()
	m.search, cmd = m.search.Update(msg)
	if m.search.Value() != before {
		m.results = m.matcher.Match(m.search.Value(), 0)
		m.cursor = 0
		m.offset = 0
		m.err = ""
	}
	return m, cmd
}

func (m *model) rows() int { return max(1, m.height-8) }

func (m *model) resizeInputs() {
	m.search.Width = max(1, m.width-4-len(m.search.Prompt)-1)
	m.input.Width = max(1, m.width-4-len(m.input.Prompt)-1)
}

func (m *model) keepVisible() {
	m.cursor = max(0, min(m.cursor, len(m.results)-1))
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.rows() {
		m.offset = m.cursor - m.rows() + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.results)-m.rows())))
}

// Navigation leaves Home/End and word movement available to textinput.
func (m *model) navigate(k tea.KeyMsg, confirmation bool) bool {
	position := m.cursor
	last := len(m.results) - 1
	if confirmation {
		position = m.confirmOffset
		last = max(0, len(m.marks)-m.rows())
	}
	switch k.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		position--
	case tea.KeyDown, tea.KeyCtrlN:
		position++
	case tea.KeyPgUp:
		position -= m.rows()
	case tea.KeyPgDown:
		position += m.rows()
	case tea.KeyCtrlHome:
		position = 0
	case tea.KeyCtrlEnd:
		position = last
	default:
		return false
	}
	position = max(0, min(position, last))
	if confirmation {
		m.confirmOffset = position
	} else {
		m.cursor = position
		m.keepVisible()
	}
	return true
}

func (m *model) openForm(mode, value string) tea.Cmd {
	m.mode = mode
	m.err = ""
	m.search.Blur()
	m.confirmOffset = 0
	m.input = textinput.New()
	m.input.Prompt = "> "
	if mode == "delete" {
		m.input.Prompt = "Type YES > "
	}
	m.input.SetValue(displayText(value))
	m.input.CursorEnd()
	m.resizeInputs()
	return m.input.Focus()
}

func validateName(name string) error {
	return workspace.ValidateName(strings.TrimSpace(name))
}

func (m *model) submit() tea.Cmd {
	value := m.input.Value()
	switch m.mode {
	case "delete":
		if value != "YES" {
			m.err = "Type YES exactly to confirm deletion"
			return nil
		}
		m.action = Action{Kind: "delete", Names: m.markedNames()}
	case "graduate":
		if strings.TrimSpace(value) == "" || strings.ContainsFunc(value, unicode.IsControl) || value == "." || value == ".." {
			m.err = "Enter a destination path"
			return nil
		}
		m.action = Action{Kind: "graduate", Entry: m.selected, Destination: value}
	default:
		if err := validateName(value); err != nil {
			m.err = err.Error()
			return nil
		}
		m.action = Action{Kind: m.mode, Name: value}
		if m.mode == "rename" {
			m.action.Entry = m.selected
		}
	}
	return tea.Quit
}

func (m *model) markedNames() []string {
	names := make([]string, 0, len(m.marks))
	for name := range m.marks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
