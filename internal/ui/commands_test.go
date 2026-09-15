package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/provider"
	"github.com/volcanic001/alice/internal/store"
	"github.com/volcanic001/alice/internal/usage"
)

type recordingProvider struct {
	requests []chat.Request
}

func (p *recordingProvider) Name() string { return "recording" }

func (p *recordingProvider) Stream(_ context.Context, request chat.Request) <-chan chat.Event {
	p.requests = append(p.requests, request)
	return make(chan chat.Event)
}

func testCommandModel(t *testing.T, usageStores ...*usage.Store) (Model, *recordingProvider) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	provider := &recordingProvider{}
	model, err := New(database, provider, "deepseek-flash", 0.7, usageStores...)
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

func TestUsageIsKeptOnlyAfterSuccessfulStream(t *testing.T) {
	usageStore := usage.New(filepath.Join(t.TempDir(), "usage.json"))
	model, _ := testCommandModel(t, usageStore)
	usage := &chat.UsageRecord{Model: "deepseek-chat", TotalTokens: 3, RequestedAt: time.Now()}
	next, _ := model.Update(streamMsg(chat.Event{Usage: usage}))
	model = next.(Model)
	if len(model.usageRecords) != 0 || model.pendingUsage == nil {
		t.Fatalf("el uso se guardó antes de finalizar: records=%#v pending=%#v", model.usageRecords, model.pendingUsage)
	}
	next, _ = model.Update(streamMsg(chat.Event{Done: true}))
	model = next.(Model)
	if len(model.usageRecords) != 1 || model.usageRecords[0] != *usage || model.pendingUsage != nil {
		t.Fatalf("uso final inesperado: records=%#v pending=%#v", model.usageRecords, model.pendingUsage)
	}
	persisted, err := usageStore.Load()
	if err != nil || len(persisted) != 1 || persisted[0].Model != usage.Model || !persisted[0].RequestedAt.Equal(usage.RequestedAt) {
		t.Fatalf("uso no persistido: records=%#v err=%v", persisted, err)
	}
}

func TestNewLoadsPersistedUsage(t *testing.T) {
	usageStore := usage.New(filepath.Join(t.TempDir(), "usage.json"))
	want := chat.UsageRecord{Model: "deepseek-chat", TotalTokens: 3, RequestedAt: time.Now()}
	if err := usageStore.Append(want); err != nil {
		t.Fatal(err)
	}
	model, _ := testCommandModel(t, usageStore)
	if len(model.usageRecords) != 1 || model.usageRecords[0].Model != want.Model || model.usageRecords[0].TotalTokens != want.TotalTokens || !model.usageRecords[0].RequestedAt.Equal(want.RequestedAt) {
		t.Fatalf("uso no restaurado: %#v", model.usageRecords)
	}
}

func TestFinalUsageChunkIsPropagatedAndPersisted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(response, `data: {"model":"deepseek-chat","choices":[{"delta":{"content":"Hola"}}],"usage":null}`)
		fmt.Fprintln(response)
		fmt.Fprintln(response, `data: {"model":"deepseek-chat","choices":[{"delta":{"content":" mundo"}}],"usage":null}`)
		fmt.Fprintln(response)
		fmt.Fprintln(response, `data:{"model":"deepseek-flash","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":17,"completion_tokens":9,"total_tokens":26,"prompt_cache_hit_tokens":12,"prompt_cache_miss_tokens":5}}`)
		fmt.Fprintln(response)
		fmt.Fprintln(response, "data:[DONE]")
	}))
	defer server.Close()

	usageStore := usage.New(filepath.Join(t.TempDir(), "usage.json"))
	model, _ := testCommandModel(t, usageStore)
	model.provider = provider.DeepSeek{APIKey: "secreto", BaseURL: server.URL}
	model.input.SetValue("saluda")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	for command != nil {
		next, command = model.Update(command())
		model = next.(Model)
	}
	if model.busy {
		t.Fatal("el stream no terminó")
	}
	if len(model.usageRecords) != 1 {
		t.Fatalf("UsageRecord no propagado: %v", model.usageRecords)
	}
	record := model.usageRecords[0]
	if record.ConversationID != model.conversation.ID || record.Model != "deepseek-flash" || record.PromptTokens != 17 || record.CompletionTokens != 9 || record.TotalTokens != 26 || record.PromptCacheHitTokens != 12 || record.PromptCacheMissTokens != 5 {
		t.Fatalf("UsageRecord inesperado: %v", record)
	}
	persisted, err := usageStore.Load()
	if err != nil || len(persisted) != 1 || persisted[0].ConversationID != model.conversation.ID || persisted[0].Model != "deepseek-flash" || persisted[0].TotalTokens != 26 {
		t.Fatalf("UsageRecord no persistido: records=%v err=%v", persisted, err)
	}
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
		for _, expected := range []string{"AYUDA", "COMANDOS", "/help", "/stats", "/new", "/history", "/copy", "NAVEGACIÓN", "Shift+Enter"} {
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
	if len(provider.requests) != 1 || provider.requests[0].Model != "deepseek-flash" || provider.requests[0].Temperature != 0.7 || !model.busy || len(model.messages) != 1 || model.messages[0].Content != "¿Qué es /etc en Linux?" {
		t.Fatalf("el texto normal no siguió el flujo del provider: requests=%d busy=%v mensajes=%#v", len(provider.requests), model.busy, model.messages)
	}
}
