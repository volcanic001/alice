package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/memory"
	"github.com/volcanic001/alice/internal/search"
)

// newSearchServer responde /web/search con los resultados dados y devuelve
// la URL del servidor. Usa newMemoryTestModel y scriptedProvider, definidos
// en memory_test.go — ambos son genéricos, no específicos de mem0.

func TestSearchKeyCommandShowsStatusWhenDisabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})

	model.input.SetValue("/search-key")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.webSearch != nil {
		t.Fatal("la búsqueda web no debería activarse solo por consultar el estado")
	}
	if !strings.Contains(model.status, "desactivada") {
		t.Fatalf("estado inesperado: %q", model.status)
	}
}

func TestSearchKeyCommandActivatesAndVerifiesKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	brave := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"grounding":{"generic":[]},"sources":{}}`)
	}))
	defer brave.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	model.webSearchBaseURL = brave.URL

	model.input.SetValue("/search-key clave-nueva")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if model.webSearch == nil || model.webSearch.APIKey != "clave-nueva" {
		t.Fatalf("la clave no quedó activa: %+v", model.webSearch)
	}
	if !strings.Contains(model.status, "verificada") {
		t.Fatalf("no se confirmó la verificación: %q", model.status)
	}
	if len(provider.requests) != 0 || len(model.messages) != 0 {
		t.Fatalf("/search-key no debe llegar nunca al provider ni al historial: requests=%d mensajes=%d", len(provider.requests), len(model.messages))
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.BraveAPIKey != "clave-nueva" {
		t.Fatalf("la clave no quedó persistida para el siguiente arranque: %+v", reloaded)
	}
}

func TestSearchKeyCommandClearDisablesAndPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})
	model.SetWebSearch(search.New("clave"))

	model.input.SetValue("/search-key clear")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.webSearch != nil {
		t.Fatal("la búsqueda web debería quedar desactivada")
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.BraveAPIKey != "" {
		t.Fatalf("/search-key clear debería quedar persistido: %+v", reloaded)
	}
}

func TestSearchWithoutQueryShowsUsage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	model.SetWebSearch(search.New("clave"))

	model.input.SetValue("/search")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "Uso: /search") {
		t.Fatalf("faltó el mensaje de uso: %q", model.status)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("sin pregunta no debe llamarse al provider: %d", len(provider.requests))
	}
}

func TestSearchWithoutKeyConfiguredShowsStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider) // sin SetWebSearch

	model.input.SetValue("/search quién ganó el partido")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "desactivada") {
		t.Fatalf("estado inesperado: %q", model.status)
	}
	if len(provider.requests) != 0 || len(model.messages) != 0 {
		t.Fatalf("sin clave no debe guardarse ni mandarse nada: requests=%d mensajes=%d", len(provider.requests), len(model.messages))
	}
}

func TestWebSearchInjectsResultsBeforeStreaming(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	brave := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/llm/context" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		if request.URL.Query().Get("q") != "quién ganó el partido" {
			t.Errorf("query inesperada: %q", request.URL.Query().Get("q"))
		}
		fmt.Fprint(response, `{"grounding":{"generic":[{"url":"https://example.com/partido","title":"Resultado del partido","snippets":["El equipo local ganó 3-1"]}]},"sources":{"https://example.com/partido":{"age":["hace 1 hora"]}}}`)
	}))
	defer brave.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := search.New("clave")
	client.BaseURL = brave.URL
	model.SetWebSearch(client)

	model.input.SetValue("/search quién ganó el partido")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	if len(provider.requests) != 1 {
		t.Fatalf("se esperaba una request al provider, hubo %d", len(provider.requests))
	}
	messages := provider.requests[0].Messages
	if len(messages) != 2 || messages[0].Role != "system" || !strings.Contains(messages[0].Content, "El equipo local ganó 3-1") {
		t.Fatalf("no se inyectaron los resultados de Brave: %#v", messages)
	}
	if messages[1].Role != "user" || messages[1].Content != "quién ganó el partido" {
		t.Fatalf("el mensaje del usuario no sigue al contexto: %#v", messages[1])
	}
	// La pregunta guardada en el historial debe ser la pregunta tal cual,
	// no los resultados crudos de Brave.
	if len(model.messages) != 1 || model.messages[0].Content != "quién ganó el partido" {
		t.Fatalf("el historial debería guardar solo la pregunta: %#v", model.messages)
	}
}

func TestWebSearchEmptyResultsStillAnswers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	brave := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"grounding":{"generic":[]},"sources":{}}`)
	}))
	defer brave.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := search.New("clave")
	client.BaseURL = brave.URL
	model.SetWebSearch(client)

	model.input.SetValue("/search algo muy raro que no existe")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	if len(provider.requests) != 1 {
		t.Fatalf("sin resultados igual debe seguir hasta el provider: %d", len(provider.requests))
	}
	messages := provider.requests[0].Messages
	if len(messages) != 2 || messages[0].Role != "system" || !strings.Contains(messages[0].Content, "no hubo resultados") {
		t.Fatalf("debió avisar que no hubo resultados: %#v", messages)
	}
}

func TestWebSearchFailureReportsErrorAndSkipsProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	brave := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
	}))
	defer brave.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := search.New("clave-mala")
	client.BaseURL = brave.URL
	model.SetWebSearch(client)

	model.input.SetValue("/search algo")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if len(provider.requests) != 0 {
		t.Fatalf("un fallo de búsqueda no debe llegar al provider: %d", len(provider.requests))
	}
	if model.busy {
		t.Fatal("un fallo de búsqueda debe dejar la interfaz utilizable")
	}
	if !strings.Contains(model.status, "No se pudo buscar en internet") {
		t.Fatalf("el fallo de búsqueda debe avisarse, a diferencia de mem0: %q", model.status)
	}
}

func TestWebSearchCombinesWithMemoryContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	brave := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"grounding":{"generic":[{"url":"https://example.com","title":"T","snippets":["resultado de brave"]}]},"sources":{}}`)
	}))
	defer brave.Close()
	mem0 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"results":[{"memory":"vive en El Salvador"}]}`)
	}))
	defer mem0.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	webClient := search.New("clave")
	webClient.BaseURL = brave.URL
	model.SetWebSearch(webClient)
	memClient := memory.New("clave", "usuario-1")
	memClient.BaseURL = mem0.URL
	model.SetMemory(memClient)

	model.input.SetValue("/search algo")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	messages := provider.requests[0].Messages
	if len(messages) != 3 {
		t.Fatalf("se esperaban 3 mensajes (brave + mem0 + usuario): %#v", messages)
	}
	if messages[0].Role != "system" || !strings.Contains(messages[0].Content, "resultado de brave") {
		t.Fatalf("el contexto de brave debe ir primero: %#v", messages[0])
	}
	if messages[1].Role != "system" || !strings.Contains(messages[1].Content, "vive en El Salvador") {
		t.Fatalf("el contexto de mem0 debe ir segundo: %#v", messages[1])
	}
	if messages[2].Role != "user" {
		t.Fatalf("el mensaje del usuario debe ir al final: %#v", messages[2])
	}
}

func TestSearchKeyStripsPlaceholderBrackets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})
	model.webSearchBaseURL = "http://127.0.0.1:0" // no debe llegar a usarse

	model.input.SetValue("/search-key <brave-abc123>")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.webSearch == nil || model.webSearch.APIKey != "brave-abc123" {
		t.Fatalf("la clave debió limpiarse de los <>: %+v", model.webSearch)
	}
}

func TestFormatSearchResultsIncludesURL(t *testing.T) {
	sources := []search.ContextSource{{Title: "T", URL: "https://example.com", Snippets: []string{"D"}}}
	text := formatSearchResults(sources)
	if !strings.Contains(text, "https://example.com") || !strings.Contains(text, "T") || !strings.Contains(text, "D") {
		t.Fatalf("faltan datos en el contexto formateado: %q", text)
	}
}

// El aviso sobre la antigüedad es lo que evita que el modelo trate un
// "reloj" capturado hace meses como la hora actual real (ver sesión: Brave
// devolvió horas distintas de cuatro páginas, todas con pinta de "ahora").
func TestFormatSearchResultsWarnsAboutStaleAge(t *testing.T) {
	sources := []search.ContextSource{{Title: "T", URL: "https://example.com", Snippets: []string{"04:24:14 CST"}, Age: []string{"288 days ago"}}}
	text := formatSearchResults(sources)
	if !strings.Contains(text, "288 days ago") {
		t.Fatalf("faltó mostrar la antigüedad real de la fuente: %q", text)
	}
	if !strings.Contains(text, "nunca lo trates como el valor actual") {
		t.Fatalf("faltó el aviso de desconfiar de valores tipo reloj: %q", text)
	}
}

func TestFormatSearchResultsUsesUnknownAgeFallback(t *testing.T) {
	sources := []search.ContextSource{{Title: "T", URL: "https://example.com", Snippets: []string{"D"}}}
	text := formatSearchResults(sources)
	if !strings.Contains(text, "antigüedad desconocida") {
		t.Fatalf("faltó el fallback de antigüedad: %q", text)
	}
}

func TestNewsWithoutQueryShowsUsage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	model.SetWebSearch(search.New("clave"))

	model.input.SetValue("/news")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "Uso: /news") {
		t.Fatalf("faltó el mensaje de uso: %q", model.status)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("sin tema no debe llamarse al provider: %d", len(provider.requests))
	}
}

func TestNewsWithoutKeyConfiguredShowsStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider) // sin SetWebSearch

	model.input.SetValue("/news elecciones")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "desactivada") {
		t.Fatalf("estado inesperado: %q", model.status)
	}
	if len(provider.requests) != 0 || len(model.messages) != 0 {
		t.Fatalf("sin clave no debe guardarse ni mandarse nada: requests=%d mensajes=%d", len(provider.requests), len(model.messages))
	}
}

// "/new" coincide con un case exacto del switch, evaluado antes que el
// prefijo "/news" del default — así que en teoría no hay forma de que choquen.
// Este test es el guardia que lo mantiene así si alguien cambia esa
// estructura más adelante.
func TestNewsDoesNotCollideWithNewCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	conversationsBefore := len(model.conversations)

	model.input.SetValue("/new")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if len(model.conversations) != conversationsBefore+1 {
		t.Fatalf("/new debió crear una conversación nueva, conversations=%d", len(model.conversations))
	}
	if strings.Contains(model.status, "Uso: /news") || len(provider.requests) != 0 {
		t.Fatalf("/new no debe pasar por el flujo de /news: status=%q requests=%d", model.status, len(provider.requests))
	}
}

func TestNewsInjectsResultsWithAgeBeforeStreaming(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	brave := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/news/search" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		if request.URL.Query().Get("freshness") != "pd" {
			t.Errorf("freshness inesperado: %q", request.URL.Query().Get("freshness"))
		}
		fmt.Fprint(response, `{"results":[{"title":"Terremoto en la costa","url":"https://example.com/n","description":"Reportes iniciales","age":"3 hours ago","breaking":true}]}`)
	}))
	defer brave.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := search.New("clave")
	client.BaseURL = brave.URL
	model.SetWebSearch(client)

	model.input.SetValue("/news terremoto")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	if len(provider.requests) != 1 {
		t.Fatalf("se esperaba una request al provider, hubo %d", len(provider.requests))
	}
	messages := provider.requests[0].Messages
	if len(messages) != 2 || messages[0].Role != "system" {
		t.Fatalf("no se inyectó contexto de noticias: %#v", messages)
	}
	if !strings.Contains(messages[0].Content, "3 hours ago") || !strings.Contains(messages[0].Content, "última hora") {
		t.Fatalf("faltó la antigüedad o la marca de última hora: %q", messages[0].Content)
	}
}

func TestNewsEmptyResultsStillAnswers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	brave := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"results":[]}`)
	}))
	defer brave.Close()

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := search.New("clave")
	client.BaseURL = brave.URL
	model.SetWebSearch(client)

	model.input.SetValue("/news algo muy específico y raro")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, func(Model) bool { return len(provider.requests) > 0 })

	messages := provider.requests[0].Messages
	if len(messages) != 2 || !strings.Contains(messages[0].Content, "no hubo resultados recientes") {
		t.Fatalf("debió avisar que no hubo noticias recientes: %#v", messages)
	}
}

func TestFormatNewsResultsUsesUnknownAgeFallback(t *testing.T) {
	results := []search.NewsResult{{Title: "T", URL: "https://example.com", Description: "D"}}
	text := formatNewsResults(results)
	if !strings.Contains(text, "antigüedad desconocida") {
		t.Fatalf("faltó el fallback de antigüedad: %q", text)
	}
}
