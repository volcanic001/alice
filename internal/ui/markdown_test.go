package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/provider"
	"github.com/volcanic001/alice/internal/store"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderUserMessageUsesMinimalTerminalLayout(t *testing.T) {
	rendered := renderUserMessage("dame una ventaja de linux corto", 40)
	plain := strings.TrimPrefix(xansi.Strip(rendered), "\n")
	if plain != "┊ Tú\n  dame una ventaja de linux corto" {
		t.Fatalf("presentación inesperada del usuario: %q", plain)
	}
	for name, style := range map[string]lipgloss.Style{
		"marcador":  userMarkerStyle,
		"etiqueta":  userLabelStyle,
		"contenido": userContentStyle,
	} {
		if _, ok := style.GetBackground().(lipgloss.NoColor); !ok {
			t.Fatalf("el estilo %s todavía tiene fondo: %v", name, style.GetBackground())
		}
	}
}

func TestRenderUserMessageWrapsWithContentIndent(t *testing.T) {
	const width = 24
	content := "explícame qué diferencia hay entre el kernel de Linux y el sistema operativo completo"
	plain := strings.TrimPrefix(xansi.Strip(renderUserMessage(content, width)), "\n")
	lines := strings.Split(plain, "\n")
	if len(lines) < 4 || lines[0] != "┊ Tú" {
		t.Fatalf("el mensaje largo no hizo wrap bajo su encabezado: %q", plain)
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("línea sin sangría de contenido: %q", line)
		}
		if lineWidth := xansi.StringWidth(line); lineWidth > width {
			t.Fatalf("línea de ancho %d excede el ancho %d: %q", lineWidth, width, line)
		}
	}
}

func TestRenderUserMessagePreservesMultilineLayout(t *testing.T) {
	content := "primera línea\nsegunda línea que también pertenece al usuario\n\núltima línea"
	plain := strings.TrimPrefix(xansi.Strip(renderUserMessage(content, 60)), "\n")
	want := "┊ Tú\n  primera línea\n  segunda línea que también pertenece al usuario\n  \n  última línea"
	if plain != want {
		t.Fatalf("presentación multilínea inesperada:\nwant: %q\n got: %q", want, plain)
	}
}

func TestUserMessageKeepsHeaderInputAndContextMeter(t *testing.T) {
	model := testScreenModel(t)
	headerBefore := xansi.Strip(model.chatHeader(model.viewport.Width))
	promptBefore := model.input.Prompt
	model.messages = []chat.Message{{Role: "user", Content: "hola"}}
	model.refresh()
	headerAfter := xansi.Strip(model.chatHeader(model.viewport.Width))

	if headerAfter != headerBefore || !strings.Contains(headerAfter, "◆ ALICE") || !strings.Contains(headerAfter, "Context") {
		t.Fatalf("el header o Context Meter cambió: antes=%q después=%q", headerBefore, headerAfter)
	}
	if model.input.Prompt != promptBefore || model.input.Prompt != "› " {
		t.Fatalf("el indicador del input cambió: antes=%q después=%q", promptBefore, model.input.Prompt)
	}
	if view := xansi.Strip(model.View()); !strings.Contains(xansi.Strip(model.pages[0]), "┊ Tú\n  hola") || !strings.Contains(view, "◆ ALICE") || !strings.Contains(view, "› ") {
		t.Fatalf("faltan símbolos semánticos en la vista: %q", view)
	}
}

func TestRenderMarkdownForNarrowViewport(t *testing.T) {
	model := Model{markdownCache: make(map[string]string)}
	source := "# Título\n\n**Negrita** y `código`.\n\n- uno\n- dos\n\n1. primero\n2. segundo\n\n> una cita\n\n```sh\npkg install git\n```\n\n[Termux](https://termux.dev)"

	rendered := model.renderMarkdown(source, 28)
	plain := xansi.Strip(rendered)

	for _, marker := range []string{"**", "```", "`código`", "# Título"} {
		if strings.Contains(plain, marker) {
			t.Fatalf("el marcador Markdown %q sigue visible en %q", marker, plain)
		}
	}
	for _, expected := range []string{"Título", "Negrita", "código", "• uno", "1. primero", "2. segundo", "│ una cita", "pkg install git", "Termux", "https://termux.dev"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("falta %q en el renderizado %q", expected, plain)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if width := xansi.StringWidth(line); width > 28 {
			t.Fatalf("línea de ancho %d excede el viewport: %q", width, xansi.Strip(line))
		}
	}
}

func TestRenderMarkdownCachesFinalMessagesByWidth(t *testing.T) {
	model := Model{markdownCache: make(map[string]string)}
	source := "**respuesta final**"

	first := model.renderMarkdown(source, 30)
	second := model.renderMarkdown(source, 30)
	if first != second {
		t.Fatal("el resultado cacheado cambió")
	}
	if len(model.markdownCache) != 1 {
		t.Fatalf("se esperó una entrada de caché, hay %d", len(model.markdownCache))
	}

	model.renderMarkdown(source, 20)
	if model.markdownWidth != 20 || len(model.markdownCache) != 1 {
		t.Fatalf("la caché no se reconstruyó para el nuevo ancho: width=%d entries=%d", model.markdownWidth, len(model.markdownCache))
	}
}

func TestCompleteStreamingFlow(t *testing.T) {
	requestReached := false
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestReached = true
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		flusher := response.(http.Flusher)
		for _, chunk := range []string{"**Hola", " mundo**"} {
			fmt.Fprintf(response, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(response, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	model, err := New(database, provider.DeepSeek{APIKey: "test", BaseURL: server.URL}, provider.DeepSeekFlashModel, 0.7)
	if err != nil {
		t.Fatal(err)
	}
	model.viewport.Width = 32
	model.input.SetValue("hola!")

	next, command := model.send()
	current := next.(Model)
	for step := 1; current.busy; step++ {
		if command == nil {
			t.Fatalf("paso %d: no se programó la siguiente lectura SSE", step)
		}
		message := command()
		next, command = current.Update(message)
		current = next.(Model)
		t.Logf("paso %d: Bubble Tea recibió %T, draft=%q busy=%v siguiente=%v", step, message, current.draft, current.busy, command != nil)
	}
	if !requestReached {
		t.Fatal("la request HTTP nunca llegó al servidor")
	}
	if current.status != "Listo" || len(current.messages) != 2 || current.messages[1].Content != "**Hola mundo**" {
		t.Fatalf("finalización incorrecta: status=%q messages=%+v", current.status, current.messages)
	}
	if len(current.markdownCache) != 1 || strings.Contains(xansi.Strip(current.viewport.View()), "**") {
		t.Fatalf("Glamour no renderizó la respuesta final: cache=%d view=%q", len(current.markdownCache), xansi.Strip(current.viewport.View()))
	}
}

func TestStreamErrorIsFriendlyAndLeavesChatUsable(t *testing.T) {
	const exposedKey = "sk-secret-817e"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(response, `{"error":{"message":"Authentication Fails, Your api key: %s is invalid"}}`, exposedKey)
	}))
	defer server.Close()

	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	model, err := New(database, provider.DeepSeek{APIKey: "test", BaseURL: server.URL}, provider.DeepSeekFlashModel, 0.7)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = next.(Model)
	model.input.SetValue("hola")

	next, command := model.send()
	current := next.(Model)
	next, _ = current.Update(command())
	current = next.(Model)
	const expected = "⚠ Error de autenticación\nLa API key de DeepSeek no es válida."
	if current.busy || current.cancel != nil || current.status != expected {
		t.Fatalf("estado después del error: busy=%v cancel=%v status=%q", current.busy, current.cancel != nil, current.status)
	}
	if len(current.messages) != 1 || current.messages[0].Content != "hola" {
		t.Fatalf("el error se guardó como respuesta: %#v", current.messages)
	}
	if view := xansi.Strip(current.View()); strings.Contains(view, exposedKey) || strings.Contains(view, "Authentication Fails") {
		t.Fatalf("la TUI expuso detalles de la API: %q", view)
	}

	current.input.SetValue("reintentar")
	next, command = current.send()
	current = next.(Model)
	if !current.busy || command == nil {
		t.Fatalf("el input no quedó utilizable: busy=%v command=%v", current.busy, command != nil)
	}
	next, _ = current.Update(command())
	current = next.(Model)
	if current.busy {
		t.Fatal("la reintento dejó la interfaz generando")
	}
}
