package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/search"
)

// Integración con Brave Search (ver internal/search). A diferencia de mem0,
// que es un realce silencioso de fondo, la búsqueda web solo se dispara con
// el comando explícito /search: un fallo aquí sí se le avisa al usuario, en
// vez de degradarse en silencio, porque todo el sentido del comando era
// buscar.
const (
	searchTimeout      = 8 * time.Second
	searchResultCount  = 5
	searchKeyCheckText = "alice: verificación de clave"
)

type searchContextMsg struct {
	context string // bloque de contexto para el modelo; vacío si no hubo resultados
	err     error  // fallo real (red, auth, HTTP) — distinto de "sin resultados"
}

type searchKeyCheckedMsg struct{ err error }

// SetWebSearch activa la búsqueda web para esta sesión. Se llama una vez al
// arrancar, después de New, solo cuando hay BRAVE_API_KEY configurada.
func (m *Model) SetWebSearch(client *search.Client) { m.webSearch = client }

// handleSearchKeyCommand resuelve "/search-key", "/search-key <api_key>" y
// "/search-key clear" — el apartado de la TUI para activar, cambiar o
// quitar la búsqueda web sin tocar variables de entorno ni archivos a mano.
func (m Model) handleSearchKeyCommand(argument string) (tea.Model, tea.Cmd) {
	argument = stripPlaceholderBrackets(argument)
	switch {
	case argument == "":
		if m.webSearch == nil {
			m.status = "Búsqueda web: desactivada\nUsa /search-key <api_key> para activarla con tu clave de Brave Search (brave.com/search/api)"
		} else {
			m.status = "Búsqueda web: activada\nUsa /search-key clear para desactivarla"
		}
		return m, nil
	case argument == "clear":
		if err := config.ClearBraveAPIKey(); err != nil {
			m.status = "No se pudo desactivar la búsqueda: " + err.Error()
			return m, nil
		}
		m.webSearch = nil
		m.status = "✓ Búsqueda web desactivada"
		return m, nil
	default:
		if err := config.SaveBraveAPIKey(argument); err != nil {
			m.status = "No se pudo guardar la clave: " + err.Error()
			return m, nil
		}
		client := search.New(argument)
		if m.webSearchBaseURL != "" {
			client.BaseURL = m.webSearchBaseURL
		}
		m.webSearch = client
		m.status = "Búsqueda web activada, verificando con Brave…"
		return m, m.checkSearchKey()
	}
}

// checkSearchKey confirma contra Brave que la clave recién guardada
// funciona. No bloquea el chat: corre en segundo plano.
func (m Model) checkSearchKey() tea.Cmd {
	client := m.webSearch
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
		defer cancel()
		_, err := client.Context(ctx, searchKeyCheckText)
		return searchKeyCheckedMsg{err: err}
	}
}

// beginWebSearch arranca un turno iniciado con /search: valida que haya algo
// que buscar y una clave configurada, y si todo está en orden usa el mismo
// beginTurn que un mensaje normal, pero encadenando primero fetchSearchContext
// en vez de ir directo a mem0.
func (m Model) beginWebSearch(query string) (tea.Model, tea.Cmd) {
	if query == "" {
		m.status = "Uso: /search <pregunta>\nEjemplo: /search ¿quién ganó el partido anoche?"
		return m, nil
	}
	if m.webSearch == nil {
		m.status = "Búsqueda web: desactivada\nUsa /search-key <api_key> para activarla con tu clave de Brave Search (brave.com/search/api)"
		return m, nil
	}
	next, ctx, err := m.beginTurn(query)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	next.status = "Buscando en internet…"
	return next, tea.Batch(next.fetchSearchContext(ctx, query), thinkingTick())
}

// fetchSearchContext busca en Brave (vía /llm/context: contenido real de la
// página, no solo título y descripción) y arma el bloque de contexto que se
// le antepone al mensaje del usuario. Sin resultados no es un error —se le
// avisa al modelo igual, para que lo diga en vez de inventar una
// respuesta—; un error real (red, clave inválida, HTTP) sí se reporta para
// que el usuario sepa que la búsqueda no ocurrió.
func (m Model) fetchSearchContext(ctx context.Context, query string) tea.Cmd {
	client := m.webSearch
	return func() tea.Msg {
		searchCtx, cancel := context.WithTimeout(ctx, searchTimeout)
		defer cancel()
		sources, err := client.Context(searchCtx, query)
		if err != nil {
			return searchContextMsg{err: err}
		}
		return searchContextMsg{context: formatSearchResults(sources)}
	}
}

// formatSearchResults arma el contexto a partir de contenido real de página
// (snippets), no de título+descripción. El aviso sobre Age es crítico: Brave
// la calcula de cuándo rastreó la página, no de cuándo ocurrió lo que dice
// el texto — un fragmento con pinta de reloj en vivo puede ser una captura
// de hace meses (lo confirmamos probándolo: ver conversación). Sin ese aviso
// explícito, el modelo tomaría cualquier hora/fecha del texto como actual.
func formatSearchResults(sources []search.ContextSource) string {
	var text strings.Builder
	if len(sources) == 0 {
		text.WriteString("Buscaste en internet (Brave) para esta pregunta, pero no hubo resultados. Dile al usuario que no encontraste nada, no inventes una respuesta.\n")
		return text.String()
	}
	text.WriteString("Contenido real extraído de internet (Brave) para esta pregunta. Básate en esto para responder; si no alcanza, dilo.\n")
	text.WriteString("Aviso: la antigüedad de cada fuente es de cuándo Brave rastreó la página, no de cuándo ocurrió lo que describe el texto. Si un fragmento muestra algo con pinta de reloj o valor \"en vivo\", es casi seguro una captura vieja del rastreo — nunca lo trates como el valor actual real.\n")
	for i, source := range sources {
		age := "antigüedad desconocida"
		if len(source.Age) > 0 {
			age = strings.Join(source.Age, " / ")
		}
		fmt.Fprintf(&text, "%d. %s — %s (rastreado: %s)\n", i+1, source.Title, source.URL, age)
		for _, snippet := range source.Snippets {
			fmt.Fprintf(&text, "   %s\n", snippet)
		}
	}
	return text.String()
}

// beginNews arranca un turno iniciado con /news: igual que /search, pero
// encadena fetchNewsContext (Brave News, últimas 24h) en vez de la búsqueda
// web genérica.
func (m Model) beginNews(query string) (tea.Model, tea.Cmd) {
	if query == "" {
		m.status = "Uso: /news <tema>\nEjemplo: /news elecciones en El Salvador"
		return m, nil
	}
	if m.webSearch == nil {
		m.status = "Búsqueda web: desactivada\nUsa /search-key <api_key> para activarla con tu clave de Brave Search (brave.com/search/api)"
		return m, nil
	}
	next, ctx, err := m.beginTurn(query)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	next.status = "Buscando noticias…"
	return next, tea.Batch(next.fetchNewsContext(ctx, query), thinkingTick())
}

// fetchNewsContext busca en Brave News (limitado a las últimas 24 horas,
// ver internal/search) y arma el contexto para el modelo. Reutiliza
// searchContextMsg: el manejador en Update no necesita saber si el contexto
// vino de /web/search o de /news/search, solo lo encadena igual hacia mem0 y
// después hacia DeepSeek.
func (m Model) fetchNewsContext(ctx context.Context, query string) tea.Cmd {
	client := m.webSearch
	return func() tea.Msg {
		searchCtx, cancel := context.WithTimeout(ctx, searchTimeout)
		defer cancel()
		results, err := client.News(searchCtx, query, searchResultCount)
		if err != nil {
			return searchContextMsg{err: err}
		}
		return searchContextMsg{context: formatNewsResults(results)}
	}
}

func formatNewsResults(results []search.NewsResult) string {
	var text strings.Builder
	if len(results) == 0 {
		text.WriteString("Buscaste noticias (Brave News, limitado a las últimas 24 horas) sobre este tema, pero no hubo resultados recientes. Dile al usuario que no encontraste nada reciente — no uses noticias viejas de tu entrenamiento ni inventes una respuesta.\n")
		return text.String()
	}
	text.WriteString("Noticias recientes (Brave News, últimas 24 horas) sobre este tema. Cada una indica hace cuánto se publicó entre paréntesis — confía en ese dato de antigüedad en vez de asumir que todas son de hoy.\n")
	for i, result := range results {
		age := result.Age
		if age == "" {
			age = "antigüedad desconocida"
		}
		breaking := ""
		if result.Breaking {
			breaking = " · última hora"
		}
		fmt.Fprintf(&text, "%d. %s (%s%s) — %s\n   %s\n", i+1, result.Title, age, breaking, result.URL, result.Description)
	}
	return text.String()
}
