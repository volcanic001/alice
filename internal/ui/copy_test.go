package ui

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/chat"
)

func TestCopyCopiesLatestAssistantResponse(t *testing.T) {
	model, provider := testCommandModel(t)
	model.messages = []chat.Message{
		{Role: "assistant", Content: "respuesta anterior"},
		{Role: "user", Content: "otra pregunta"},
		{Role: "assistant", Content: "respuesta más reciente"},
	}
	var copied string
	model.clipboardWrite = func(_ context.Context, content string) error {
		copied = content
		return nil
	}

	model.input.SetValue("/copy")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command == nil {
		t.Fatal("/copy no produjo el comando de portapapeles")
	}
	next, _ = model.Update(command())
	model = next.(Model)

	if copied != "respuesta más reciente" {
		t.Fatalf("se copió %q, se esperaba la última respuesta", copied)
	}
	if model.status != "✓ Respuesta copiada" || len(provider.requests) != 0 || model.input.Value() != "" {
		t.Fatalf("resultado inesperado: status=%q requests=%d input=%q", model.status, len(provider.requests), model.input.Value())
	}
}

func TestCopyWithoutAssistantResponse(t *testing.T) {
	model, provider := testCommandModel(t)
	model.messages = []chat.Message{{Role: "user", Content: "hola"}}
	called := false
	model.clipboardWrite = func(context.Context, string) error {
		called = true
		return nil
	}

	model = submitInput(t, model, "/copy")

	if called || model.status != "No hay ninguna respuesta para copiar." || len(provider.requests) != 0 {
		t.Fatalf("resultado inesperado: called=%v status=%q requests=%d", called, model.status, len(provider.requests))
	}
}

func TestCopyUsesOriginalMarkdownInsteadOfRenderedContent(t *testing.T) {
	model, _ := testCommandModel(t)
	const markdown = "# Título\n\n**negrita** y `código`"
	model.messages = []chat.Message{{Role: "assistant", Content: markdown}}
	model.refresh()
	if rendered := model.renderMarkdown(markdown, 80); rendered == markdown {
		t.Fatal("la prueba necesita que Glamour transforme el Markdown")
	}
	var copied string
	model.clipboardWrite = func(_ context.Context, content string) error {
		copied = content
		return nil
	}

	model.input.SetValue("/copy")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command == nil {
		t.Fatal("/copy no produjo el comando de portapapeles")
	}
	model.Update(command())

	if copied != markdown {
		t.Fatalf("se alteró el Markdown original: obtuve %q, esperaba %q", copied, markdown)
	}
}

func TestCopyReportsClipboardBackendFailure(t *testing.T) {
	model, _ := testCommandModel(t)
	model.messages = []chat.Message{{Role: "assistant", Content: "respuesta"}}
	model.clipboardWrite = func(context.Context, string) error {
		return errors.New("backend unavailable")
	}

	model.input.SetValue("/copy")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command == nil {
		t.Fatal("/copy no produjo el comando de portapapeles")
	}
	next, _ = model.Update(command())
	model = next.(Model)

	want := "✗ Portapapeles no disponible"
	if runtime.GOOS == "android" {
		want = "✗ Portapapeles no disponible · /copy requiere Termux:API en Android"
	}
	if model.status != want {
		t.Fatalf("mensaje de error inesperado: obtuve %q, esperaba %q", model.status, want)
	}
}

func TestCopyTimesOutWithoutBlockingIndefinitely(t *testing.T) {
	model, _ := testCommandModel(t)
	model.messages = []chat.Message{{Role: "assistant", Content: "respuesta"}}
	model.clipboardTimeout = 20 * time.Millisecond
	model.clipboardWrite = func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}

	model.input.SetValue("/copy")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command == nil {
		t.Fatal("/copy no produjo el comando de portapapeles")
	}
	started := time.Now()
	message := command()
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("el timeout tardó demasiado: %s", elapsed)
	}
	result, ok := message.(copyMsg)
	if !ok || !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("resultado de timeout inesperado: %#v", message)
	}
	next, _ = model.Update(message)
	model = next.(Model)
	if model.status != clipboardUnavailableStatus() {
		t.Fatalf("mensaje de timeout inesperado: %q", model.status)
	}
}
