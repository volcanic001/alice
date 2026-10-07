package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/memory"
)

// Integración con mem0 Cloud (ver internal/memory). Es un servicio aparte de
// DeepSeek y totalmente opcional: si no hay cliente configurado (memory ==
// nil), estos comandos son no-ops y Alice funciona igual que sin memoria.
const (
	memorySearchTimeout = 3 * time.Second
	memoryAddTimeout    = 5 * time.Second
)

type memoryContextMsg struct {
	messages []chat.Message
}

type memoryStoredMsg struct{ err error }

type memoryKeyCheckedMsg struct{ err error }

// SetMemory activa el recuerdo persistente para esta sesión. Se llama una
// vez al arrancar, después de New, solo cuando hay MEM0_API_KEY configurada.
func (m *Model) SetMemory(client *memory.Client) { m.memory = client }

// handleMemoryCommand resuelve "/memory", "/memory <api_key>" y
// "/memory clear" — el apartado de la TUI para activar, cambiar o quitar la
// memoria sin tocar variables de entorno ni archivos a mano. La clave se
// guarda en ~/.config/alice/, igual que haría MEM0_API_KEY del entorno, así
// que sigue activa en el siguiente arranque.
func (m Model) handleMemoryCommand(argument string) (tea.Model, tea.Cmd) {
	argument = stripPlaceholderBrackets(argument)
	switch {
	case argument == "":
		if m.memory == nil {
			m.status = "Memoria: desactivada\nUsa /memory <api_key> para activarla con tu clave de mem0.ai"
		} else {
			m.status = "Memoria: activada\nUsuario mem0: " + m.memory.UserID + "\nUsa /memory clear para desactivarla"
		}
		return m, nil
	case argument == "clear":
		if err := config.ClearMem0APIKey(); err != nil {
			m.status = "No se pudo desactivar la memoria: " + err.Error()
			return m, nil
		}
		m.memory = nil
		m.status = "✓ Memoria desactivada"
		return m, nil
	default:
		userID, err := config.SaveMem0APIKey(argument)
		if err != nil {
			m.status = "No se pudo guardar la clave: " + err.Error()
			return m, nil
		}
		client := memory.New(argument, userID)
		if m.memoryBaseURL != "" {
			client.BaseURL = m.memoryBaseURL
		}
		m.memory = client
		m.status = "Memoria activada, verificando con mem0…"
		return m, m.checkMemoryKey()
	}
}

// stripPlaceholderBrackets quita "<" y ">" si envuelven la clave entera. Es
// común copiar la documentación literal ("/memory <api_key>") sin notar que
// los símbolos eran solo un marcador de posición; mem0 rechazaría esa clave
// con un 401 silencioso y confuso de diagnosticar, así que lo corregimos.
func stripPlaceholderBrackets(argument string) string {
	if len(argument) >= 2 && strings.HasPrefix(argument, "<") && strings.HasSuffix(argument, ">") {
		return strings.TrimSpace(argument[1 : len(argument)-1])
	}
	return argument
}

// checkMemoryKey confirma contra mem0 que la clave recién guardada funciona.
// No bloquea el chat: corre en segundo plano y solo actualiza el estado.
func (m Model) checkMemoryKey() tea.Cmd {
	client := m.memory
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), memorySearchTimeout)
		defer cancel()
		_, err := client.Search(ctx, "alice: verificación de clave")
		return memoryKeyCheckedMsg{err: err}
	}
}

// fetchMemoryContext busca hechos relevantes en mem0 antes de responder y
// siempre antepone un mensaje de sistema cuando la memoria está activa —
// incluso sin hechos encontrados— para que el modelo sepa que sí tiene
// memoria persistente y no responda con su disclaimer genérico de "no puedo
// recordar entre conversaciones". Si mem0 no está configurado o la búsqueda
// tarda más de memorySearchTimeout, Alice sigue sin memoria para este turno
// en vez de bloquear la conversación.
func (m Model) fetchMemoryContext(ctx context.Context, query string) tea.Cmd {
	client := m.memory
	messages := m.messages
	return func() tea.Msg {
		if client == nil {
			return memoryContextMsg{messages: messages}
		}
		searchCtx, cancel := context.WithTimeout(ctx, memorySearchTimeout)
		defer cancel()
		facts, _ := client.Search(searchCtx, query) // un fallo aquí deja la lista vacía, no corta el turno
		augmented := make([]chat.Message, 0, len(messages)+1)
		augmented = append(augmented, chat.Message{Role: "system", Content: formatMemoryContext(facts)})
		augmented = append(augmented, messages...)
		return memoryContextMsg{messages: augmented}
	}
}

func formatMemoryContext(facts []string) string {
	var text strings.Builder
	text.WriteString("Tienes memoria persistente activada (mem0): los hechos relevantes que el usuario comparta se recuerdan automáticamente para futuras conversaciones, sin que nadie tenga que activar nada más. Nunca digas que no puedes recordar información entre conversaciones.\n")
	if len(facts) > 0 {
		text.WriteString("Esto es lo que recuerdas de conversaciones anteriores. Úsalo solo si es relevante:\n")
		for _, fact := range facts {
			text.WriteString("- " + fact + "\n")
		}
	}
	return text.String()
}

// storeMemory manda el intercambio recién terminado a mem0 en segundo plano
// para que extraiga hechos nuevos. No bloquea ni se refleja en la UI: un
// fallo aquí no debe interrumpir el chat.
func (m Model) storeMemory(userContent, assistantContent string) tea.Cmd {
	client := m.memory
	if client == nil || userContent == "" || assistantContent == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), memoryAddTimeout)
		defer cancel()
		err := client.Add(ctx, []chat.Message{
			{Role: "user", Content: userContent},
			{Role: "assistant", Content: assistantContent},
		})
		return memoryStoredMsg{err: err}
	}
}
