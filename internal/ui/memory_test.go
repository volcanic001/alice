package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/memory"
	"github.com/volcanic001/alice/internal/store"
)

// scriptedProvider emite los eventos dados y cierra el canal, para probar el
// flujo de memoria sin depender de un provider HTTP real.
type scriptedProvider struct {
	events   []chat.Event
	requests []chat.Request
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) Stream(_ context.Context, request chat.Request) <-chan chat.Event {
	p.requests = append(p.requests, request)
	out := make(chan chat.Event, len(p.events))
	for _, event := range p.events {
		out <- event
	}
	close(out)
	return out
}

func newMemoryTestModel(t *testing.T, provider chat.Provider) Model {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	model, err := New(database, provider, "deepseek-flash", 0.7)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 84, Height: 28})
	return next.(Model)
}

func TestMemoryFactsAreInjectedAsSystemContextBeforeStreaming(t *testing.T) {
	mem0 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"results":[{"memory":"vive en Caracas"}]}`)
	}))
	defer mem0.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := memory.New("clave", "usuario-1")
	client.BaseURL = mem0.URL
	model.SetMemory(client)

	model.input.SetValue("¿dónde vivo?")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	if len(provider.requests) != 1 {
		t.Fatalf("se esperaba una request al provider, hubo %d", len(provider.requests))
	}
	messages := provider.requests[0].Messages
	if len(messages) != 2 || messages[0].Role != "system" || !strings.Contains(messages[0].Content, "vive en Caracas") {
		t.Fatalf("no se inyectó el contexto de mem0: %#v", messages)
	}
	if messages[1].Role != "user" || messages[1].Content != "¿dónde vivo?" {
		t.Fatalf("el mensaje del usuario no sigue al contexto: %#v", messages[1])
	}
}

// Un fallo de búsqueda no debe tumbar el turno: Alice sigue sin los hechos de
// esta consulta, pero como la memoria sigue activa, el aviso de capacidad se
// mantiene para que el modelo no diga que no tiene memoria persistente.
func TestMemorySearchFailureKeepsCapabilityNoticeWithoutFacts(t *testing.T) {
	mem0 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer mem0.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := memory.New("clave", "usuario-1")
	client.BaseURL = mem0.URL
	model.SetMemory(client)

	model.input.SetValue("hola")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	messages := provider.requests[0].Messages
	if len(provider.requests) != 1 || len(messages) != 2 || messages[0].Role != "system" {
		t.Fatalf("un fallo de búsqueda debió seguir avisando que hay memoria activa: %#v", provider.requests)
	}
	if strings.Contains(messages[0].Content, "Esto es lo que recuerdas") {
		t.Fatalf("no debería listar hechos cuando la búsqueda falló: %q", messages[0].Content)
	}
}

func TestWithoutMemoryConfiguredMessagesAreUnchanged(t *testing.T) {
	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider) // sin SetMemory: la función queda desactivada

	model.input.SetValue("hola")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	if len(provider.requests) != 1 || len(provider.requests[0].Messages) != 1 || provider.requests[0].Messages[0].Role != "user" {
		t.Fatalf("sin mem0 configurado no debería inyectarse contexto: %#v", provider.requests)
	}
}

func TestExchangeIsStoredInMemoryAfterCompletion(t *testing.T) {
	type addCall struct {
		Messages []map[string]string `json:"messages"`
		UserID   string              `json:"user_id"`
	}
	var received []addCall
	mem0 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v3/memories/search/":
			fmt.Fprint(response, `{"results":[]}`)
		case "/v3/memories/add/":
			var call addCall
			_ = json.NewDecoder(request.Body).Decode(&call)
			received = append(received, call)
			fmt.Fprint(response, `{"event_id":"e1","status":"PENDING"}`)
		}
	}))
	defer mem0.Close()

	provider := &scriptedProvider{events: []chat.Event{
		{Text: "hola, soy Alice"},
		{Done: true},
	}}
	model := newMemoryTestModel(t, provider)
	client := memory.New("clave", "usuario-1")
	client.BaseURL = mem0.URL
	model.SetMemory(client)

	model.input.SetValue("hola")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	drive(t, next.(Model), command, nil)

	if len(received) != 1 {
		t.Fatalf("se esperaba una llamada a mem0 Add, hubo %d", len(received))
	}
	call := received[0]
	if call.UserID != "usuario-1" {
		t.Fatalf("user_id inesperado: %q", call.UserID)
	}
	if len(call.Messages) != 2 || call.Messages[0]["content"] != "hola" || call.Messages[1]["content"] != "hola, soy Alice" {
		t.Fatalf("intercambio guardado inesperado: %#v", call.Messages)
	}
}

// El comando /memory es el "apartado" para activar, cambiar o quitar la
// memoria desde dentro de Alice, sin tocar variables de entorno ni archivos
// a mano. t.Setenv("XDG_CONFIG_HOME", ...) aísla cada test de la config real.

func TestMemoryCommandShowsStatusWhenDisabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})

	model.input.SetValue("/memory")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.memory != nil {
		t.Fatal("la memoria no debería activarse solo por consultar el estado")
	}
	if !strings.Contains(model.status, "desactivada") {
		t.Fatalf("estado inesperado: %q", model.status)
	}
}

func TestMemoryCommandActivatesAndVerifiesKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mem0 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"results":[]}`)
	}))
	defer mem0.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	model.memoryBaseURL = mem0.URL

	model.input.SetValue("/memory clave-nueva")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if model.memory == nil || model.memory.APIKey != "clave-nueva" {
		t.Fatalf("la clave no quedó activa: %+v", model.memory)
	}
	if !strings.Contains(model.status, "verificada") {
		t.Fatalf("no se confirmó la verificación: %q", model.status)
	}
	if len(provider.requests) != 0 || len(model.messages) != 0 {
		t.Fatalf("/memory no debe llegar nunca al provider ni al historial: requests=%d mensajes=%d", len(provider.requests), len(model.messages))
	}

	// Debe sobrevivir a un reinicio: Load() tiene que recoger la misma clave.
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mem0APIKey != "clave-nueva" || reloaded.Mem0UserID != model.memory.UserID {
		t.Fatalf("la clave no quedó persistida para el siguiente arranque: %+v", reloaded)
	}
}

func TestMemoryCommandReportsVerificationFailureButStaysActive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mem0 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
	}))
	defer mem0.Close()

	model := newMemoryTestModel(t, &scriptedProvider{})
	model.memoryBaseURL = mem0.URL

	model.input.SetValue("/memory clave-mala")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if model.memory == nil {
		t.Fatal("la clave se guarda aunque la verificación falle; un 401 también puede ser un corte de red pasajero")
	}
	if !strings.Contains(model.status, "no respondió") {
		t.Fatalf("no se avisó del fallo de verificación: %q", model.status)
	}
}

func TestMemoryCommandClearDisablesMemoryAndPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})
	client := memory.New("clave", "usuario-1")
	model.SetMemory(client)

	model.input.SetValue("/memory clear")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.memory != nil {
		t.Fatal("la memoria debería quedar desactivada")
	}
	if !strings.Contains(model.status, "desactivada") {
		t.Fatalf("estado inesperado: %q", model.status)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mem0APIKey != "" {
		t.Fatalf("/memory clear debería quedar persistido: %+v", reloaded)
	}
}

// Reproduce el caso real: alguien copia "/memory <api_key>" de la ayuda sin
// notar que los <> eran solo un marcador de posición. mem0 rechazaría esa
// clave con un 401 silencioso y guardaría hechos que nunca se recuperan;
// Alice debe limpiarla antes de guardarla.
func TestMemoryCommandStripsPlaceholderBrackets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})
	model.memoryBaseURL = "http://127.0.0.1:0" // no debe llegar a usarse en esta prueba

	model.input.SetValue("/memory <m0-abc123>")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.memory == nil || model.memory.APIKey != "m0-abc123" {
		t.Fatalf("la clave debió limpiarse de los <>: %+v", model.memory)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mem0APIKey != "m0-abc123" {
		t.Fatalf("la clave persistida sigue con los <>: %q", reloaded.Mem0APIKey)
	}
}
