package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/provider"
)

// SetVenice registra el provider de DeepSeek original (para poder volver a
// él con "/venice-key clear") y, si hay API key, activa Venice de entrada.
// Se llama una vez al arrancar, después de New, igual que SetWebSearch.
func (m *Model) SetVenice(apiKey, baseURL, model string) {
	if deepSeek, ok := m.provider.(provider.DeepSeek); ok {
		m.deepSeek = deepSeek
		m.deepSeekModel = m.requestModel
	}
	m.veniceAPIKey = apiKey
	m.veniceBaseURL = baseURL
	m.veniceModel = model
	if apiKey != "" {
		m.provider = provider.Venice{APIKey: apiKey, BaseURL: baseURL}
		m.requestModel = model
	}
}

// handleVeniceKeyCommand resuelve "/venice-key", "/venice-key <api_key>" y
// "/venice-key clear" — el equivalente a /search-key pero para cambiar el
// modelo de chat de DeepSeek a Venice (y viceversa) sin tocar archivos ni
// variables de entorno a mano.
func (m Model) handleVeniceKeyCommand(argument string) (tea.Model, tea.Cmd) {
	argument = stripPlaceholderBrackets(argument)
	switch {
	case argument == "":
		if _, usingVenice := m.provider.(provider.Venice); !usingVenice {
			m.status = "Venice: desactivado (usando DeepSeek)\nUsa /venice-key <api_key> para activarlo con tu clave de Venice (venice.ai)"
		} else {
			m.status = "Venice: activado, modelo " + m.requestModel + "\nUsa /venice-key clear para volver a DeepSeek"
		}
		return m, nil
	case argument == "clear":
		if err := config.ClearVeniceAPIKey(); err != nil {
			m.status = "No se pudo desactivar Venice: " + err.Error()
			return m, nil
		}
		m.provider = m.deepSeek
		m.requestModel = m.deepSeekModel
		m.veniceAPIKey = ""
		m.status = "✓ Venice desactivado, volviste a DeepSeek"
		return m, nil
	default:
		if err := config.SaveVeniceAPIKey(argument); err != nil {
			m.status = "No se pudo guardar la clave: " + err.Error()
			return m, nil
		}
		model := m.veniceModel
		if model == "" {
			model = provider.VeniceRecommendedModel
		}
		m.veniceAPIKey = argument
		m.provider = provider.Venice{APIKey: argument, BaseURL: m.veniceBaseURL}
		m.requestModel = model
		m.status = "✓ Venice activado (modelo " + model + ")"
		return m, nil
	}
}
