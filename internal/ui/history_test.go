package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/provider"
	"github.com/volcanic001/alice/internal/store"
)

func testScreenModel(t *testing.T) Model {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	model, err := New(database, provider.DeepSeek{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model)
}

func TestChatHasNoPermanentSidebar(t *testing.T) {
	model := testScreenModel(t)
	if strings.Contains(xansi.Strip(model.View()), "Conversaciones") {
		t.Fatal("el chat aún muestra el sidebar")
	}
}

func TestHistoryNavigationAndOpenConversation(t *testing.T) {
	model := testScreenModel(t)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	model = next.(Model)
	if model.screen != historyScreen || !strings.Contains(xansi.Strip(model.View()), "HISTORIAL") {
		t.Fatal("Ctrl+H no abrió el historial")
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	model = next.(Model)
	if model.screen != chatScreen || !model.input.Focused() || len(model.conversations) != 2 {
		t.Fatalf("Ctrl+N desde historial: pantalla=%d focused=%v conversaciones=%d", model.screen, model.input.Focused(), len(model.conversations))
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = next.(Model)
	if model.historyIndex != 0 {
		t.Fatalf("↑ no cambió la selección: %d", model.historyIndex)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(Model)
	if model.historyIndex != 1 {
		t.Fatalf("↓ no cambió la selección: %d", model.historyIndex)
	}
	expected := model.conversations[1].ID
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.screen != chatScreen || model.conversation.ID != expected {
		t.Fatalf("Enter no abrió la conversación: pantalla=%d id=%d", model.screen, model.conversation.ID)
	}
}

func TestHistoryEscReturnsToChat(t *testing.T) {
	model := testScreenModel(t)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)
	if model.screen != chatScreen {
		t.Fatal("Esc no volvió al chat")
	}
}

func TestColumnWidthUsesTerminalWidthAndCentersWideChat(t *testing.T) {
	model := testScreenModel(t)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	model = next.(Model)
	if model.viewport.Width != 36 {
		t.Fatalf("ancho estrecho = %d, se esperaban 36", model.viewport.Width)
	}
	next, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model = next.(Model)
	if model.viewport.Width != maxColumnWidth {
		t.Fatalf("ancho amplio = %d, se esperaban %d", model.viewport.Width, maxColumnWidth)
	}
	firstLine := strings.Split(xansi.Strip(model.View()), "\n")[0]
	if !strings.HasPrefix(firstLine, " ") {
		t.Fatalf("la columna amplia no está centrada: %q", firstLine)
	}
}

func TestResizeRecalculatesColumnWidthAndPages(t *testing.T) {
	model := testScreenModel(t)
	model.messages = append(model.messages, chat.Message{Role: "user", Content: strings.Repeat("palabra ", 240)})
	next, _ := model.Update(tea.WindowSizeMsg{Width: 48, Height: 30})
	model = next.(Model)
	narrowPages := model.totalPages
	next, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model = next.(Model)
	if model.totalPages >= narrowPages {
		t.Fatalf("resize no recalculó páginas: estrecho=%d ancho=%d", narrowPages, model.totalPages)
	}
}

func TestColumnLayoutMath(t *testing.T) {
	cases := []struct {
		terminalWidth, contentWidth, leftMargin, rightMargin int
	}{
		{40, 36, 2, 2},
		{60, 56, 2, 2},
		{80, 76, 2, 2},
		{100, 84, 8, 8},
		{120, 84, 18, 18},
		{160, 84, 38, 38},
	}
	for _, test := range cases {
		model := Model{width: test.terminalWidth}
		layout := model.columnLayout()
		if layout.contentWidth != test.contentWidth || layout.leftMargin != test.leftMargin || layout.rightMargin != test.rightMargin {
			t.Fatalf("%d columnas: layout=%+v", test.terminalWidth, layout)
		}
		if layout.leftMargin-layout.rightMargin > 1 || layout.rightMargin-layout.leftMargin > 1 {
			t.Fatalf("%d columnas: márgenes desbalanceados: %+v", test.terminalWidth, layout)
		}
	}
}
