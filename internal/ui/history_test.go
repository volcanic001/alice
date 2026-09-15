package ui

import (
	"path/filepath"
	"slices"
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

func testHistoryModel(t *testing.T, titles ...string) (Model, *store.Store) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	conversations := make([]chat.Conversation, 0, len(titles))
	for _, title := range titles {
		conversation, err := database.CreateConversation()
		if err != nil {
			t.Fatal(err)
		}
		conversation.Title = title
		conversations = append(conversations, conversation)
	}
	model, err := New(database, provider.DeepSeek{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	model.conversations = conversations
	model.conversation = conversations[0]
	model.messages = nil
	model.screen = historyScreen
	next, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model), database
}

func confirmDelete(t *testing.T, model Model) Model {
	t.Helper()
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = next.(Model)
	if !model.deleteConfirm {
		t.Fatal("d no inició la confirmación de borrado")
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	return next.(Model)
}

func containsConversation(conversations []chat.Conversation, id int64) bool {
	for _, conversation := range conversations {
		if conversation.ID == id {
			return true
		}
	}
	return false
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
	model.messages = append(model.messages, chat.Message{Role: "user", Content: strings.Repeat("palabra ", 600)})
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

func TestHistoryDeleteMiddleConversation(t *testing.T) {
	model, database := testHistoryModel(t, "Primera", "Intermedia", "Última")
	model.historyIndex = 1
	deleted := model.conversations[1]
	nextID := model.conversations[2].ID

	model = confirmDelete(t, model)
	if len(model.conversations) != 2 || model.historyIndex != 1 || model.conversations[model.historyIndex].ID != nextID {
		t.Fatalf("selección tras borrar intermedia: índice=%d conversaciones=%#v", model.historyIndex, model.conversations)
	}
	conversations, err := database.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	if containsConversation(conversations, deleted.ID) {
		t.Fatalf("la conversación %d sigue en almacenamiento", deleted.ID)
	}
}

func TestHistoryDeleteLastConversationSelectsPrevious(t *testing.T) {
	model, _ := testHistoryModel(t, "Primera", "Intermedia", "Última")
	model.historyIndex = 2
	previousID := model.conversations[1].ID

	model = confirmDelete(t, model)
	if len(model.conversations) != 2 || model.historyIndex != 1 || model.conversations[model.historyIndex].ID != previousID {
		t.Fatalf("índice inválido tras borrar la última: índice=%d conversaciones=%#v", model.historyIndex, model.conversations)
	}
}

func TestHistoryDeleteActiveConversationSelectsValidReplacement(t *testing.T) {
	model, _ := testHistoryModel(t, "Activa", "Siguiente")
	deletedID := model.conversation.ID

	model = confirmDelete(t, model)
	if model.conversation.ID == deletedID || model.conversation.ID != model.conversations[model.historyIndex].ID {
		t.Fatalf("conversación activa inválida: activa=%d selección=%d", model.conversation.ID, model.conversations[model.historyIndex].ID)
	}
}

func TestHistoryDeleteOnlyConversationCreatesEmptyReplacement(t *testing.T) {
	model, database := testHistoryModel(t, "Única")

	model = confirmDelete(t, model)
	if len(model.conversations) != 1 || model.historyIndex != 0 || model.conversation.ID != model.conversations[0].ID || model.conversation.Title != "Nuevo chat" {
		t.Fatalf("estado tras borrar la única conversación: índice=%d activa=%d conversaciones=%#v", model.historyIndex, model.conversation.ID, model.conversations)
	}
	conversations, err := database.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].Title != "Nuevo chat" {
		t.Fatalf("el reemplazo persistente no quedó vacío: %#v", conversations)
	}
}

func TestHistoryDeleteConfirmationCancelsWithN(t *testing.T) {
	model, _ := testHistoryModel(t, "Primera", "Segunda")
	selectedID := model.conversations[0].ID
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = next.(Model)
	if view := xansi.Strip(model.View()); !strings.Contains(view, `Borrar "Primera"? y/n`) || !strings.Contains(view, "↑/↓ seleccionar · Enter abrir · / buscar · r renombrar · d borrar · Esc volver") {
		t.Fatalf("confirmación o ayuda inesperada: %q", view)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(Model)
	if model.historyIndex != 0 {
		t.Fatalf("la selección cambió durante la confirmación: %d", model.historyIndex)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)
	if model.deleteConfirm || len(model.conversations) != 2 || model.conversations[0].ID != selectedID {
		t.Fatalf("n no canceló el borrado: confirmación=%v conversaciones=%#v", model.deleteConfirm, model.conversations)
	}
}

func TestHistoryDeleteConfirmationCancelsWithEsc(t *testing.T) {
	model, _ := testHistoryModel(t, "Única")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)
	if model.deleteConfirm || model.screen != historyScreen || len(model.conversations) != 1 {
		t.Fatalf("Esc no canceló el borrado: confirmación=%v pantalla=%d conversaciones=%d", model.deleteConfirm, model.screen, len(model.conversations))
	}
}

func TestHistoryShowsEmptyState(t *testing.T) {
	model := testScreenModel(t)
	model.screen = historyScreen
	model.conversations = nil
	if !strings.Contains(xansi.Strip(model.View()), "Sin conversaciones") {
		t.Fatal("el historial vacío no muestra el estado esperado")
	}
}

func pressKey(t *testing.T, model Model, key tea.KeyMsg) Model {
	t.Helper()
	next, _ := model.Update(key)
	return next.(Model)
}

func TestHistoryRenameStartsWithSelectedTitle(t *testing.T) {
	model, _ := testHistoryModel(t, "Título actual", "Otro")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !model.renameActive || !model.renameInput.Focused() || model.renameInput.Value() != "Título actual" {
		t.Fatalf("estado de renombrado inesperado: activo=%v foco=%v valor=%q", model.renameActive, model.renameInput.Focused(), model.renameInput.Value())
	}
	view := xansi.Strip(model.View())
	if !strings.Contains(view, "Renombrar:") || !strings.Contains(view, "> Título actual") || !strings.Contains(view, "r renombrar") {
		t.Fatalf("el input o la ayuda no aparecen: %q", view)
	}
}

func TestHistoryRenameConfirmPersistsTrimsAndKeepsSelection(t *testing.T) {
	model, database := testHistoryModel(t, "Original", "Otra")
	model.historyIndex = 0
	selectedID := model.conversations[0].ID
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model.renameInput.SetValue("  Título elegido  ")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.renameActive || model.historyIndex != 0 || model.conversations[0].ID != selectedID || model.conversations[0].Title != "Título elegido" {
		t.Fatalf("confirmación inesperada: índice=%d conversaciones=%#v", model.historyIndex, model.conversations)
	}
	conversations, err := database.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	if conversations[0].ID != selectedID || conversations[0].Title != "Título elegido" {
		t.Fatalf("el nombre no persistió: %#v", conversations)
	}
}

func TestHistoryRenameUpdatesActiveConversationAndCancelDoesNotPersist(t *testing.T) {
	model, database := testHistoryModel(t, "Activa", "Otra")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model.renameInput.SetValue("Renombrada")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.conversation.Title != "Renombrada" {
		t.Fatalf("la conversación activa no se actualizó: %#v", model.conversation)
	}
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model.renameInput.SetValue("No guardar")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.renameActive || model.conversation.Title != "Renombrada" || model.conversations[0].Title != "Renombrada" {
		t.Fatalf("Esc cambió el estado: activa=%#v lista=%#v", model.conversation, model.conversations)
	}
	conversations, err := database.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	if conversations[0].Title != "Renombrada" {
		t.Fatalf("Esc escribió en almacenamiento: %#v", conversations)
	}
}

func TestHistoryRenameEmptyTitleDoesNotChangeConversation(t *testing.T) {
	model, _ := testHistoryModel(t, "Original")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model.renameInput.SetValue(" \t ")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.renameActive || model.conversations[0].Title != "Original" || model.status != "El título no puede estar vacío" {
		t.Fatalf("título vacío cambió el estado: %#v", model)
	}
}

func TestHistoryRenameInputConsumesHistoryShortcuts(t *testing.T) {
	model, _ := testHistoryModel(t, "Original", "Otra")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dr")})
	if model.deleteConfirm || !model.renameActive || model.renameInput.Value() != "Originaldr" {
		t.Fatalf("d/r no quedaron en el input: borrar=%v activo=%v valor=%q", model.deleteConfirm, model.renameActive, model.renameInput.Value())
	}
}

func TestHistoryRenameDoesNothingWhenEmpty(t *testing.T) {
	model := testScreenModel(t)
	model.screen = historyScreen
	model.conversations = nil
	model.historyIndex = 0
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if model.renameActive || model.deleteConfirm {
		t.Fatalf("r activó una acción con el historial vacío: %#v", model)
	}
}

func startHistorySearch(t *testing.T, model Model) Model {
	t.Helper()
	return pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
}

func typeHistorySearch(t *testing.T, model Model, query string) Model {
	t.Helper()
	return pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(query)})
}

func resultTitles(model Model) []string {
	results := model.historyResults()
	titles := make([]string, len(results))
	for i, index := range results {
		titles[i] = model.conversations[index].Title
	}
	return titles
}

func TestHistorySearchFiltersTitlesInRealTime(t *testing.T) {
	model, _ := testHistoryModel(t, "Proyecto Linux", "Atlas", "Aprendiendo Go", "Servidor Linux")
	model = startHistorySearch(t, model)
	if !model.searchActive || !model.searchInput.Focused() {
		t.Fatal("/ no activó el input de búsqueda")
	}
	if view := xansi.Strip(model.View()); !strings.Contains(view, "Buscar:") || !strings.Contains(view, "Esc limpiar") {
		t.Fatalf("la vista de búsqueda es inesperada: %q", view)
	}

	model = typeHistorySearch(t, model, "lin")
	if got, want := resultTitles(model), []string{"Proyecto Linux", "Servidor Linux"}; !slices.Equal(got, want) {
		t.Fatalf("coincidencia parcial: obtuve %q, esperaba %q", got, want)
	}
	model = typeHistorySearch(t, model, "ux")
	if got, want := resultTitles(model), []string{"Proyecto Linux", "Servidor Linux"}; !slices.Equal(got, want) {
		t.Fatalf("la lista no se actualizó en tiempo real: obtuve %q, esperaba %q", got, want)
	}
}

func TestHistorySearchIsCaseInsensitiveAndTrimsQuery(t *testing.T) {
	model, _ := testHistoryModel(t, "Proyecto Linux", "Atlas")
	model = startHistorySearch(t, model)
	model = typeHistorySearch(t, model, "  LINUX  ")
	if got, want := resultTitles(model), []string{"Proyecto Linux"}; !slices.Equal(got, want) {
		t.Fatalf("búsqueda con mayúsculas/espacios: obtuve %q, esperaba %q", got, want)
	}
}

func TestHistorySearchHandlesNoResultsAndKeepsSelectionValid(t *testing.T) {
	model, _ := testHistoryModel(t, "Proyecto Linux", "Atlas", "Servidor Linux")
	model = startHistorySearch(t, model)
	model.historyIndex = 2
	model = typeHistorySearch(t, model, "atlas")
	if model.historyIndex != 0 || model.selectedHistoryConversationIndex() < 0 {
		t.Fatalf("la selección no se ajustó al reducir resultados: índice=%d", model.historyIndex)
	}
	model = typeHistorySearch(t, model, " sin-coincidencias")
	if model.historyIndex != 0 || model.selectedHistoryConversationIndex() != -1 || !strings.Contains(xansi.Strip(model.View()), "Sin resultados") {
		t.Fatalf("estado inválido sin resultados: índice=%d vista=%q", model.historyIndex, xansi.Strip(model.View()))
	}
}

func TestHistorySearchNavigationOpensSelectedResultAndEscRestoresList(t *testing.T) {
	model, _ := testHistoryModel(t, "Proyecto Linux", "Atlas", "Servidor Linux")
	secondLinuxID := model.conversations[2].ID
	model = startHistorySearch(t, model)
	model = typeHistorySearch(t, model, "linux")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyDown})
	if model.historyIndex != 1 {
		t.Fatalf("↓ no navegó entre resultados: %d", model.historyIndex)
	}
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyUp})
	if model.historyIndex != 0 {
		t.Fatalf("↑ no navegó entre resultados: %d", model.historyIndex)
	}
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyDown})
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.screen != chatScreen || model.conversation.ID != secondLinuxID {
		t.Fatalf("Enter no abrió el resultado seleccionado: pantalla=%d id=%d", model.screen, model.conversation.ID)
	}

	model.screen = historyScreen
	model = startHistorySearch(t, model)
	model = typeHistorySearch(t, model, "linux")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.searchActive || model.searchInput.Value() != "" || len(model.historyResults()) != len(model.conversations) {
		t.Fatalf("Esc no limpió ni restauró historial: activa=%v query=%q resultados=%d", model.searchActive, model.searchInput.Value(), len(model.historyResults()))
	}
}

func TestHistorySearchInputConsumesHistoryShortcuts(t *testing.T) {
	model, _ := testHistoryModel(t, "Original", "Otra")
	model = startHistorySearch(t, model)
	model = typeHistorySearch(t, model, "dr/")
	if model.deleteConfirm || model.renameActive || model.searchInput.Value() != "dr/" {
		t.Fatalf("atajos se ejecutaron durante búsqueda: borrar=%v renombrar=%v query=%q", model.deleteConfirm, model.renameActive, model.searchInput.Value())
	}
}

func TestHistorySearchReflectsRenameAndDelete(t *testing.T) {
	model, _ := testHistoryModel(t, "Original", "Servidor Linux", "Atlas")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model.renameInput.SetValue("Proyecto Linux")
	model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	model = startHistorySearch(t, model)
	model = typeHistorySearch(t, model, "linux")
	if got, want := resultTitles(model), []string{"Proyecto Linux", "Servidor Linux"}; !slices.Equal(got, want) {
		t.Fatalf("el renombrado no apareció en resultados: obtuve %q, esperaba %q", got, want)
	}
	deletedID := model.conversations[model.selectedHistoryConversationIndex()].ID
	next, _ := model.deleteSelectedConversation()
	model = next.(Model)
	if containsConversation(model.conversations, deletedID) || len(model.historyResults()) != 1 || model.selectedHistoryConversationIndex() < 0 {
		t.Fatalf("el borrado no actualizó los resultados: conversaciones=%#v resultados=%v índice=%d", model.conversations, model.historyResults(), model.historyIndex)
	}
}

func TestHistorySearchEmptyHistoryDoesNotPanic(t *testing.T) {
	model := testScreenModel(t)
	model.screen = historyScreen
	model.conversations = nil
	model = startHistorySearch(t, model)
	if !model.searchActive || !strings.Contains(xansi.Strip(model.View()), "Sin conversaciones") {
		t.Fatalf("historial vacío quedó en estado inesperado: %#v", model)
	}
}
