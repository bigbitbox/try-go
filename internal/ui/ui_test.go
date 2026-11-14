package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bigbitbox/try-go/internal/workspace"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func sampleEntries() []workspace.Entry {
	return []workspace.Entry{
		{Basename: "old", EntryPath: "/tries/old", TargetPath: "/tries/old", ModTime: time.Unix(1, 0), BaseScore: 1},
		{Basename: "新项目", EntryPath: "/tries/新项目", TargetPath: "/real/新项目", IsSymlink: true, ModTime: time.Unix(3, 0), BaseScore: 3},
		{Basename: "other", EntryPath: "/tries/other", TargetPath: "/tries/other", ModTime: time.Unix(2, 0), BaseScore: 2},
	}
}
func key(m *model, typ tea.KeyType) tea.Cmd { _, cmd := m.Update(tea.KeyMsg{Type: typ}); return cmd }
func typeText(m *model, s string)           { m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }

func TestSearchSelectRecencyAndUnicodeEditing(t *testing.T) {
	m := newModel(workspace.Config{}, sampleEntries(), "")
	if got := m.results[0].Text; got != "新项目" {
		t.Fatalf("most recent first: %q", got)
	}
	typeText(m, "新目")
	key(m, tea.KeyLeft)
	typeText(m, "项")
	if m.search.Value() != "新项目" || len(m.results) != 1 {
		t.Fatalf("unicode editing/filter: %q %#v", m.search.Value(), m.results)
	}
	if cmd := key(m, tea.KeyEnter); cmd == nil {
		t.Fatal("selection must quit")
	}
	if m.action.Kind != "cd" || m.action.Entry.TargetPath != "/real/新项目" {
		t.Fatalf("action: %+v", m.action)
	}
}

func TestCreateAndRenameForms(t *testing.T) {
	for _, tc := range []struct {
		name        string
		trigger     tea.KeyType
		query, want string
	}{
		{"enter creates unmatched", tea.KeyEnter, "想法", "想法"},
		{"ctrl-t always creates", tea.KeyCtrlT, "old", "old"},
		{"rename", tea.KeyCtrlR, "新", "新名字"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(workspace.Config{}, sampleEntries(), tc.query)
			key(m, tc.trigger)
			if tc.trigger == tea.KeyEnter {
				if m.action.Kind != "new" || m.action.Name != tc.want {
					t.Fatalf("create: %+v", m.action)
				}
				return
			}
			if tc.trigger == tea.KeyCtrlR {
				key(m, tea.KeyCtrlU)
				typeText(m, tc.want)
			}
			if key(m, tea.KeyEnter) == nil {
				t.Fatal("valid form should quit")
			}
			wantKind := "new"
			if tc.trigger == tea.KeyCtrlR {
				wantKind = "rename"
			}
			if m.action.Kind != wantKind || m.action.Name != tc.want {
				t.Fatalf("action: %+v", m.action)
			}
			if wantKind == "rename" && m.action.Entry.Basename != "新项目" {
				t.Fatalf("rename lost selected entry: %+v", m.action)
			}
		})
	}
}

func TestInvalidNamesStayInFormWithVisibleError(t *testing.T) {
	for _, name := range []string{"", " ", ".", "..", "a/b", ".hidden", `a\b`} {
		t.Run(name, func(t *testing.T) {
			m := newModel(workspace.Config{}, sampleEntries(), "")
			key(m, tea.KeyCtrlT)
			m.input.SetValue(name)
			key(m, tea.KeyEnter)
			if m.action.Kind != "" || !strings.Contains(m.View(), "Error:") {
				t.Fatalf("invalid %q accepted or invisible error: %s %+v", name, m.View(), m.action)
			}
		})
	}
}

func TestNameValidationRejectsControlCharacters(t *testing.T) {
	for _, name := range []string{"x\x1by", "a\nb", "x\x7fy"} {
		if validateName(name) == nil {
			t.Fatalf("accepted control in %q", name)
		}
	}
}

func TestGraduateDefaultAndEscapeForm(t *testing.T) {
	entries := sampleEntries()
	entries[1].Basename = "2026-11-14-新项目"
	cfg := workspace.Config{ProjectsPath: "/projects"}
	m := newModel(cfg, entries, "")
	key(m, tea.KeyCtrlG)
	want := filepath.Join(cfg.ProjectsPath, "新项目")
	if !strings.Contains(m.View(), "Graduate") || m.input.Value() != want {
		t.Fatalf("default destination: %s", m.View())
	}
	key(m, tea.KeyEsc)
	if m.action.Kind != "" || m.search.Value() != "" {
		t.Fatal("form escape should return to search")
	}
	key(m, tea.KeyCtrlG)
	key(m, tea.KeyEnter)
	if m.action.Kind != "graduate" || m.action.Destination != want || m.action.Entry != entries[1] {
		t.Fatalf("graduate: %+v", m.action)
	}
}

func TestDeleteMarksSurviveFiltersAndRequireExactYES(t *testing.T) {
	m := newModel(workspace.Config{}, sampleEntries(), "新")
	key(m, tea.KeyCtrlD)
	key(m, tea.KeyCtrlU)
	typeText(m, "old")
	key(m, tea.KeyCtrlD)
	// Mark/unmark toggles only the focused workspace.
	key(m, tea.KeyCtrlD)
	key(m, tea.KeyCtrlD)
	key(m, tea.KeyCtrlU)
	typeText(m, "no-match")
	key(m, tea.KeyEnter)
	if m.action.Kind != "" {
		t.Fatalf("delete must await confirmation: %+v", m.action)
	}
	view := m.View()
	for _, name := range []string{"old", "新项目", "YES"} {
		if !strings.Contains(view, name) {
			t.Fatalf("confirmation omits %q: %s", name, view)
		}
	}
	typeText(m, "yes")
	key(m, tea.KeyEnter)
	if m.action.Kind != "" || !strings.Contains(m.View(), "Error:") {
		t.Fatal("lowercase confirmation must be rejected visibly")
	}
	key(m, tea.KeyCtrlU)
	typeText(m, "YES")
	key(m, tea.KeyEnter)
	if m.action.Kind != "delete" || strings.Join(m.action.Names, ",") != "old,新项目" {
		t.Fatalf("all marked basenames: %+v", m.action)
	}
}

func TestEscapeCancelsDeleteDialogThenMarksThenApp(t *testing.T) {
	m := newModel(workspace.Config{}, sampleEntries(), "")
	key(m, tea.KeyCtrlD)
	key(m, tea.KeyEnter)
	key(m, tea.KeyEsc)
	if m.mode != "" || m.action.Kind != "" {
		t.Fatal("escape dialog must return to search")
	}
	key(m, tea.KeyEnter)
	if !strings.Contains(m.View(), "YES") {
		t.Fatal("escape dialog should preserve marks")
	}
	key(m, tea.KeyEsc)
	key(m, tea.KeyEsc)
	if len(m.marks) != 0 || m.action.Kind != "" {
		t.Fatal("escape marking mode must clear marks")
	}
	if key(m, tea.KeyEsc) == nil {
		t.Fatal("third escape should quit")
	}
}

func TestNavigationScrollingAndResize(t *testing.T) {
	entries := make([]workspace.Entry, 30)
	for i := range entries {
		name := fmt.Sprintf("project-%02d", i)
		entries[i] = workspace.Entry{Basename: name, EntryPath: "/tries/" + name, BaseScore: float64(30 - i)}
	}
	m := newModel(workspace.Config{}, entries, "")
	m.Update(tea.WindowSizeMsg{Width: 42, Height: 12})
	for i := 0; i < 20; i++ {
		key(m, tea.KeyDown)
	}
	if m.cursor != 20 || !strings.Contains(m.View(), "project-20") || strings.Contains(m.View(), "project-00") {
		t.Fatalf("scroll did not follow cursor: %d %s", m.cursor, m.View())
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > 42 {
			t.Fatalf("line exceeds terminal width: %q", line)
		}
	}
	if len(strings.Split(m.View(), "\n")) > 12 {
		t.Fatalf("view exceeds terminal height: %s", m.View())
	}
	key(m, tea.KeyPgDown)
	if m.cursor <= 20 {
		t.Fatal("page down did not advance")
	}
	m.Update(tea.WindowSizeMsg{Width: 18, Height: 8})
	if !strings.Contains(m.View(), "project-") {
		t.Fatalf("resize lost results: %s", m.View())
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > 18 {
			t.Fatalf("resize width: %q", line)
		}
	}
	key(m, tea.KeyCtrlHome)
	if m.cursor != 0 {
		t.Fatal("ctrl-home should select first")
	}
	key(m, tea.KeyUp)
	if m.cursor != 0 {
		t.Fatal("up should clamp")
	}
	key(m, tea.KeyCtrlEnd)
	if m.cursor != 29 {
		t.Fatal("ctrl-end should select last")
	}
	key(m, tea.KeyDown)
	if m.cursor != 29 {
		t.Fatal("down should clamp")
	}
	typeText(m, "project-00")
	if m.cursor != 0 || !strings.Contains(m.View(), "project-00") {
		t.Fatal("filter must reset scroll")
	}
}

func TestDeleteConfirmationScrollsEveryMarkedName(t *testing.T) {
	entries := make([]workspace.Entry, 18)
	for i := range entries {
		entries[i] = workspace.Entry{Basename: fmt.Sprintf("marked-%02d", i), BaseScore: float64(18 - i)}
	}
	m := newModel(workspace.Config{}, entries, "")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	for range entries {
		key(m, tea.KeyCtrlD)
		key(m, tea.KeyDown)
	}
	key(m, tea.KeyEnter)
	seen := m.View()
	for i := 0; i < 18; i++ {
		key(m, tea.KeyDown)
		seen += m.View()
	}
	for _, e := range entries {
		if !strings.Contains(seen, e.Basename) {
			t.Fatalf("marked entry inaccessible: %s", e.Basename)
		}
	}
	if len(strings.Split(m.View(), "\n")) > 12 {
		t.Fatal("confirmation exceeds viewport")
	}
}

func TestEscapeCancelsApplication(t *testing.T) {
	m := newModel(workspace.Config{}, sampleEntries(), "old")
	if key(m, tea.KeyEsc) == nil || m.action.Kind != "" {
		t.Fatalf("cancel: %+v", m.action)
	}
}

func TestControlInputIsRejectedWithoutChangingName(t *testing.T) {
	m := newModel(workspace.Config{}, nil, "")
	key(m, tea.KeyCtrlT)
	typeText(m, "bad\nname")
	if m.input.Value() != "" || !strings.Contains(m.View(), "Error:") {
		t.Fatalf("control input silently changed the name: %q %s", m.input.Value(), m.View())
	}
}

func TestMissingSelectionAndEmptyCreationShowErrors(t *testing.T) {
	for _, trigger := range []tea.KeyType{tea.KeyCtrlR, tea.KeyCtrlG, tea.KeyCtrlD, tea.KeyEnter} {
		m := newModel(workspace.Config{}, nil, "")
		key(m, trigger)
		if m.action.Kind != "" || !strings.Contains(m.View(), "Error:") {
			t.Fatalf("missing selection/empty name: key=%v action=%+v view=%s", trigger, m.action, m.View())
		}
	}
}

func TestGraduateEditedDestinationValidationAndCancel(t *testing.T) {
	m := newModel(workspace.Config{ProjectsPath: "/projects"}, sampleEntries(), "old")
	key(m, tea.KeyCtrlG)
	key(m, tea.KeyCtrlU)
	key(m, tea.KeyEnter)
	if m.action.Kind != "" || !strings.Contains(m.View(), "Error:") {
		t.Fatal("empty destination must be rejected")
	}
	typeText(m, "/different/我的项目")
	key(m, tea.KeyEnter)
	if m.action.Destination != "/different/我的项目" {
		t.Fatalf("edited destination: %+v", m.action)
	}
	for _, trigger := range []tea.KeyType{tea.KeyCtrlT, tea.KeyCtrlR, tea.KeyCtrlG, tea.KeyCtrlD} {
		m = newModel(workspace.Config{}, sampleEntries(), "old")
		key(m, trigger)
		if trigger == tea.KeyCtrlD {
			key(m, tea.KeyEnter)
		}
		if key(m, tea.KeyCtrlC) == nil || m.action.Kind != "" {
			t.Fatalf("ctrl-c must cancel %v", trigger)
		}
	}
}

func TestFilesystemNamesCannotInjectTerminalControls(t *testing.T) {
	hostile := "workspace\x1b]52;c;dHJ5\x07\n\r\t\u202e"
	entries := []workspace.Entry{{Basename: hostile, EntryPath: "/tries/" + hostile, TargetPath: "/tries/" + hostile}}
	m := newModel(workspace.Config{ProjectsPath: "/projects"}, entries, "")
	// Normal list, deletion review, and form titles/input all display existing names.
	views := []string{m.View()}
	m.marks[hostile] = true
	m.openForm("delete", "")
	views = append(views, m.View())
	m.selected = entries[0]
	m.openForm("rename", hostile)
	views = append(views, m.View())
	m.openForm("graduate", "/projects/"+hostile)
	views = append(views, m.View())
	for i, v := range views {
		if strings.Contains(v, "\x1b]52;") || strings.ContainsAny(v, "\x07\r\t\u202e") {
			t.Fatalf("view %d emits filesystem controls: %q", i, v)
		}
	}
	m.mode = ""
	clear(m.marks)
	key(m, tea.KeyEnter)
	if m.action.Entry.Basename != hostile {
		t.Fatal("display sanitization changed action identifier")
	}
}

func TestRunReportsLoadErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	action, err := Run(workspace.Config{TriesPath: path}, "")
	if err == nil || action.Kind != "" {
		t.Fatalf("load error: %+v %v", action, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatalf("UI changed user data: %q %v", data, err)
	}
}

func TestRunDoesNotCreateMissingWorkspaceRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	action, err := Run(workspace.Config{TriesPath: path}, "")
	if err == nil || action.Kind != "" {
		t.Fatalf("missing root must return an error: %+v %v", action, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("UI created the workspace root: %v", err)
	}
}

func TestBubbleTeaProgramReturnsAction(t *testing.T) {
	m := newModel(workspace.Config{}, sampleEntries(), "old")
	var output bytes.Buffer
	program := tea.NewProgram(m, tea.WithInput(strings.NewReader("\r")), tea.WithOutput(&output), tea.WithAltScreen(), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	if _, err := program.Run(); err != nil {
		t.Fatal(err)
	}
	if m.action.Kind != "cd" || m.action.Entry.Basename != "old" {
		t.Fatalf("program action: %+v", m.action)
	}
}
