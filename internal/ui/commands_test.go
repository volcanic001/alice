package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/store"
)

type recordingProvider struct {
	requests []chat.Request
}

func (p *recordingProvider) Name() string { return "recording" }

func (p *recordingProvider) Stream(_ context.Context, request chat.Request) <-chan chat.Event {
	p.requests = append(p.requests, request)
	return make(chan chat.Event)
}

func testCommandModel(t *testing.T) (Model, *recordingProvider) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	provider := &recordingProvider{}
	model, err := New(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 84, Height: 28})
	return next.(Model), provider
}

func submitInput(t *testing.T, model Model, value string) Model {
	t.Helper()
	model.input.SetValue(value)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model)
}

func TestFooterIsCompactSingleLine(t *testing.T) {
	model, _ := testCommandModel(t)
	view := xansi.Strip(model.View())
	var footer string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Enter enviar") {
			footer = line
		}
	}
	if !strings.Contains(footer, "Ctrl+N nuevo") || !strings.Contains(footer, "Ctrl+H historial") || !strings.Contains(footer, "/help ayuda") {
		t.Fatalf("footer compacto inesperado: %q", footer)
	}
	for _, removed := range []string{"Shift+Enter", "PgUp/PgDn", "Inicio/Fin", "Esc cancelar"} {
		if strings.Contains(footer, removed) {
			t.Fatalf("el footer todavía contiene %q: %q", removed, footer)
		}
	}
}

func TestFooterStaysOnOneLineInNarrowTerminal(t *testing.T) {
	model, _ := testCommandModel(t)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 28})
	model = next.(Model)
	view := xansi.Strip(model.View())
	lines := strings.Split(view, "\n")
	var footerLines int
	for _, line := range lines {
		if strings.Contains(line, "Enter enviar") {
			footerLines++
		}
	}
	if footerLines != 1 {
		t.Fatalf("el footer se partió en %d líneas: %q", footerLines, view)
	}
}

func TestLocalCommandsUseSharedScreenActionsWithoutProvider(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		model, provider := testCommandModel(t)
		model = submitInput(t, model, "/help")
		if model.screen != helpScreen || len(provider.requests) != 0 || len(model.messages) != 0 || model.input.Value() != "" {
			t.Fatalf("/help no fue local: pantalla=%d requests=%d mensajes=%d input=%q", model.screen, len(provider.requests), len(model.messages), model.input.Value())
		}
		view := xansi.Strip(model.View())
		for _, expected := range []string{"AYUDA", "COMANDOS", "/help", "/new", "/history", "NAVEGACIÓN", "Shift+Enter"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("la ayuda no muestra %q: %q", expected, view)
			}
		}
		model = pressKey(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.screen != chatScreen || !model.input.Focused() {
			t.Fatal("Esc desde ayuda no volvió al chat con el input enfocado")
		}
	})

	t.Run("new", func(t *testing.T) {
		model, provider := testCommandModel(t)
		model = submitInput(t, model, "/new")
		if model.screen != chatScreen || len(model.conversations) != 2 || len(provider.requests) != 0 || len(model.messages) != 0 {
			t.Fatalf("/new no usó la acción local: pantalla=%d conversaciones=%d requests=%d mensajes=%d", model.screen, len(model.conversations), len(provider.requests), len(model.messages))
		}
	})

	t.Run("history", func(t *testing.T) {
		model, provider := testCommandModel(t)
		model = submitInput(t, model, "/history")
		if model.screen != historyScreen || len(provider.requests) != 0 || len(model.messages) != 0 {
			t.Fatalf("/history no usó la acción local: pantalla=%d requests=%d mensajes=%d", model.screen, len(provider.requests), len(model.messages))
		}
	})
}

func TestUnknownSlashCommandStaysLocal(t *testing.T) {
	model, provider := testCommandModel(t)
	model = submitInput(t, model, "/inicio")
	if len(provider.requests) != 0 || model.busy || len(model.messages) != 0 || model.input.Value() != "" {
		t.Fatalf("el comando desconocido salió de la UI: requests=%d busy=%v mensajes=%#v input=%q", len(provider.requests), model.busy, model.messages, model.input.Value())
	}
	if got, want := model.status, "Comando desconocido: /inicio\nUsa /help para ver los comandos disponibles."; got != want {
		t.Fatalf("aviso inesperado: obtuve %q, esperaba %q", got, want)
	}
}

func TestSlashInsideNormalTextUsesProvider(t *testing.T) {
	model, provider := testCommandModel(t)
	model = submitInput(t, model, "¿Qué es /etc en Linux?")
	if len(provider.requests) != 1 || !model.busy || len(model.messages) != 1 || model.messages[0].Content != "¿Qué es /etc en Linux?" {
		t.Fatalf("el texto normal no siguió el flujo del provider: requests=%d busy=%v mensajes=%#v", len(provider.requests), model.busy, model.messages)
	}
}
