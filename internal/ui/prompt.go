package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/volcanic001/alice/internal/config"
)

// El prompt de sistema personalizado vive en disco (ver config.SaveSystemPrompt),
// igual que la clave de mem0 o de Brave, así que sigue activo en el siguiente
// arranque. A diferencia de esas dos integraciones, no hay nada que verificar
// contra un servicio externo: es solo texto que se antepone a cada turno (ver
// fetchMemoryContext en memory.go), así que guardarlo es instantáneo.

// SetSystemPrompt carga el prompt de sistema guardado para esta sesión. Se
// llama una vez al arrancar, después de New.
func (m *Model) SetSystemPrompt(prompt string) { m.systemPrompt = prompt }

// handleSystemPromptCommand resuelve "/prompt" (abre la pantalla dedicada),
// "/prompt <texto>" (lo define directamente, sin pasar por la pantalla) y
// "/prompt clear" (lo quita).
func (m Model) handleSystemPromptCommand(argument string) (tea.Model, tea.Cmd) {
	switch {
	case argument == "":
		return m.showSystemPrompt()
	case argument == "clear":
		if err := config.ClearSystemPrompt(); err != nil {
			m.status = "No se pudo quitar el prompt de sistema: " + err.Error()
			return m, nil
		}
		m.systemPrompt = ""
		m.status = "✓ Prompt de sistema eliminado"
		return m, nil
	default:
		if err := config.SaveSystemPrompt(argument); err != nil {
			m.status = "No se pudo guardar el prompt de sistema: " + err.Error()
			return m, nil
		}
		m.systemPrompt = argument
		m.status = "✓ Prompt de sistema actualizado"
		return m, nil
	}
}

// showSystemPrompt abre la pantalla dedicada con el prompt actual precargado
// para editarlo.
func (m Model) showSystemPrompt() (tea.Model, tea.Cmd) {
	m.screen = promptScreen
	m.promptInput = newPromptInput(m.systemPrompt, m.columnLayout().contentWidth, max(5, m.height-10))
	return m, m.promptInput.Focus()
}

func newPromptInput(value string, width, height int) textarea.Model {
	input := textarea.New()
	// El default de bubbles (MaxHeight: 99) corta en silencio cualquier
	// salto de línea más allá de la fila 99 — un prompt de sistema largo
	// se queda sin romper líneas desde ahí en vez de fallar visiblemente.
	// Sin límite: el prompt puede ser tan largo como el usuario necesite.
	input.MaxHeight = 0
	input.Placeholder = "Ej: Eres un asistente conciso y directo. Responde siempre en español neutro y evita rodeos…"
	input.SetValue(value)
	input.SetWidth(max(1, width))
	input.SetHeight(max(1, height))
	input.ShowLineNumbers = false
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(coral).Bold(true)
	return input
}

// updateSystemPrompt maneja las teclas de la pantalla dedicada: Ctrl+S
// guarda, Esc descarta los cambios, Ctrl+L vacía el textarea (sin guardar
// todavía — Esc después de limpiar deja el prompt anterior intacto, igual
// que con cualquier otro cambio sin confirmar). Cualquier otra tecla sigue
// el flujo normal del textarea (incluyendo Enter, que inserta un salto de
// línea en vez de enviar — el prompt suele ser varias líneas).
func (m Model) updateSystemPrompt(message tea.Msg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		return m.showChat()
	case "ctrl+s":
		return m.saveSystemPrompt()
	case "ctrl+l":
		// Solo tiene efecto si hay algo que limpiar; vacío no hace nada.
		if m.promptInput.Value() != "" {
			m.promptInput.SetValue("")
		}
		return m, nil
	}
	var command tea.Cmd
	m.promptInput, command = m.promptInput.Update(message)
	return m, command
}

func (m Model) saveSystemPrompt() (tea.Model, tea.Cmd) {
	content := strings.TrimSpace(m.promptInput.Value())
	var err error
	if content == "" {
		err = config.ClearSystemPrompt()
	} else {
		err = config.SaveSystemPrompt(content)
	}
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.systemPrompt = content
	if content == "" {
		m.status = "✓ Prompt de sistema eliminado"
	} else {
		m.status = "✓ Prompt de sistema guardado"
	}
	return m.showChat()
}

func (m Model) promptView() string {
	width := m.columnLayout().contentWidth
	var body strings.Builder
	body.WriteString(logoStyle.Render("◆ ALICE") + " · " + mutedStyle.Render("PROMPT DE SISTEMA") + "\n\n")
	body.WriteString(mutedStyle.Render("Define cómo se comporta Alice en todas las conversaciones. Déjalo vacío para volver al comportamiento default.") + "\n\n")
	body.WriteString(m.promptInput.View())
	help := "Ctrl+S guardar · Esc cancelar"
	if m.promptInput.Value() != "" {
		help = "Ctrl+S guardar · Ctrl+L limpiar · Esc cancelar"
	}
	body.WriteString("\n\n" + mutedStyle.Render(help))
	column := lipgloss.NewStyle().Width(width).Render(body.String())
	return m.placeColumn(column)
}
