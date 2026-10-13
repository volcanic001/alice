package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/volcanic001/alice/internal/config"
)

// El prompt de sistema personalizado (/prompt) vive en disco igual que las
// claves de mem0 y Brave, así que estos tests aíslan cada uno con su propio
// XDG_CONFIG_HOME, siguiendo el mismo patrón que memory_test.go y
// weather_test.go.

func TestPromptCommandOpensDedicatedScreen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, provider := testCommandModel(t)
	model.systemPrompt = "Eres concisa"

	model = submitInput(t, model, "/prompt")

	if model.screen != promptScreen || len(provider.requests) != 0 || len(model.messages) != 0 {
		t.Fatalf("/prompt no abrió la pantalla dedicada: pantalla=%d requests=%d mensajes=%d", model.screen, len(provider.requests), len(model.messages))
	}
	if model.promptInput.Value() != "Eres concisa" {
		t.Fatalf("el textarea no se precargó con el prompt actual: %q", model.promptInput.Value())
	}
	view := xansi.Strip(model.View())
	for _, expected := range []string{"PROMPT DE SISTEMA", "Eres concisa", "Ctrl+S guardar", "Ctrl+L limpiar", "Esc cancelar"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("la pantalla de prompt no muestra %q: %q", expected, view)
		}
	}
}

func TestPromptScreenCtrlLClearsTextWhenNotEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, _ := testCommandModel(t)
	model = submitInput(t, model, "/prompt")
	model.promptInput.SetValue("texto que se debe borrar")

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = next.(Model)

	if model.promptInput.Value() != "" {
		t.Fatalf("Ctrl+L no vació el textarea: %q", model.promptInput.Value())
	}
	if model.screen != promptScreen {
		t.Fatalf("Ctrl+L no debería salir de la pantalla de edición: pantalla=%d", model.screen)
	}
}

func TestPromptScreenCtrlLIsNoOpWhenAlreadyEmptyAndHidesHint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, _ := testCommandModel(t)
	model = submitInput(t, model, "/prompt")

	view := xansi.Strip(model.View())
	if strings.Contains(view, "Ctrl+L limpiar") {
		t.Fatalf("la pista de Ctrl+L no debería mostrarse sin texto que limpiar: %q", view)
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = next.(Model)
	if model.promptInput.Value() != "" {
		t.Fatalf("Ctrl+L sobre un textarea vacío no debería insertar nada: %q", model.promptInput.Value())
	}
}

func TestPromptScreenSavesWithCtrlSAndPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, _ := testCommandModel(t)

	model = submitInput(t, model, "/prompt")
	model.promptInput.SetValue("Eres un asistente pirata")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = next.(Model)

	if model.screen != chatScreen {
		t.Fatalf("Ctrl+S debería volver al chat: pantalla=%d", model.screen)
	}
	if model.systemPrompt != "Eres un asistente pirata" {
		t.Fatalf("systemPrompt no se actualizó: %q", model.systemPrompt)
	}
	if !strings.Contains(model.status, "guardado") {
		t.Fatalf("no se confirmó el guardado: %q", model.status)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SystemPrompt != "Eres un asistente pirata" {
		t.Fatalf("el prompt no quedó persistido para el siguiente arranque: %+v", reloaded)
	}
}

func TestPromptScreenEscDiscardsChanges(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveSystemPrompt("original"); err != nil {
		t.Fatal(err)
	}
	model, _ := testCommandModel(t)
	model.SetSystemPrompt("original")

	model = submitInput(t, model, "/prompt")
	model.promptInput.SetValue("cambio que no debe guardarse")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)

	if model.screen != chatScreen {
		t.Fatalf("Esc debería volver al chat: pantalla=%d", model.screen)
	}
	if model.systemPrompt != "original" {
		t.Fatalf("Esc no debería modificar el prompt activo: %q", model.systemPrompt)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SystemPrompt != "original" {
		t.Fatalf("Esc no debería tocar lo guardado en disco: %+v", reloaded)
	}
}

func TestPromptCommandSetsDirectlyWithoutOpeningScreen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, provider := testCommandModel(t)

	model = submitInput(t, model, "/prompt Eres un asistente pirata")

	if model.screen != chatScreen || len(provider.requests) != 0 || len(model.messages) != 0 {
		t.Fatalf("/prompt <texto> no fue local: pantalla=%d requests=%d mensajes=%d", model.screen, len(provider.requests), len(model.messages))
	}
	if model.systemPrompt != "Eres un asistente pirata" {
		t.Fatalf("systemPrompt no se actualizó: %q", model.systemPrompt)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SystemPrompt != "Eres un asistente pirata" {
		t.Fatalf("el prompt no quedó persistido: %+v", reloaded)
	}
}

func TestPromptCommandClearRemovesPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, _ := testCommandModel(t)
	model = submitInput(t, model, "/prompt Eres un asistente pirata")

	model = submitInput(t, model, "/prompt clear")

	if model.systemPrompt != "" {
		t.Fatalf("/prompt clear no vació el prompt activo: %q", model.systemPrompt)
	}
	if !strings.Contains(model.status, "eliminado") {
		t.Fatalf("no se confirmó la eliminación: %q", model.status)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SystemPrompt != "" {
		t.Fatalf("/prompt clear debería quedar persistido: %+v", reloaded)
	}
}

// El textarea de bubbles trae por defecto MaxHeight: 99, que descarta en
// silencio cualquier Enter más allá de la fila 99 sin avisar — un prompt de
// sistema largo se queda sin saltos de línea desde ahí en vez de fallar
// visiblemente. Este test cubre la regresión: newPromptInput debe desactivar
// ese límite.
func TestPromptScreenAllowsMoreThan99Lines(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, _ := testCommandModel(t)
	model = submitInput(t, model, "/prompt")

	const lines = 150
	for i := 0; i < lines; i++ {
		model.promptInput.SetValue(model.promptInput.Value() + "línea")
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = next.(Model)
	}

	if got := strings.Count(model.promptInput.Value(), "\n"); got < lines {
		t.Fatalf("se perdieron saltos de línea más allá de la fila 99: %d saltos, se esperaban al menos %d", got, lines)
	}
}

func TestSystemPromptIsSentAsFirstMessageToProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model, provider := testCommandModel(t)
	model.SetSystemPrompt("Eres un asistente pirata")

	model.input.SetValue("hola")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	if len(provider.requests) != 1 {
		t.Fatalf("se esperaba una sola request: %d", len(provider.requests))
	}
	messages := provider.requests[0].Messages
	if len(messages) == 0 || messages[0].Role != "system" || messages[0].Content != "Eres un asistente pirata" {
		t.Fatalf("el prompt de sistema no fue el primer mensaje enviado: %#v", messages)
	}
}
