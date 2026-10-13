package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/volcanic001/alice/internal/provider"
)

// modelOption es una entrada seleccionable en "/model". apply solo se llama
// si available es true; si no, hint explica qué falta configurar.
type modelOption struct {
	label     string
	available bool
	hint      string
	apply     func(Model) Model
}

func (m Model) modelOptions() []modelOption {
	deepSeekModel := m.deepSeekModel
	if deepSeekModel == "" {
		deepSeekModel = provider.DeepSeekFlashModel
	}
	options := []modelOption{{
		label:     "DeepSeek — " + deepSeekModel,
		available: true,
		apply: func(next Model) Model {
			next.provider = next.deepSeek
			next.requestModel = deepSeekModel
			return next
		},
	}}
	veniceModel := m.veniceModel
	if veniceModel == "" {
		veniceModel = provider.VeniceRecommendedModel
	}
	if m.veniceAPIKey == "" {
		options = append(options, modelOption{
			label: "Venice — " + veniceModel,
			hint:  "Configura primero con /venice-key <api_key>",
		})
	} else {
		options = append(options, modelOption{
			label:     "Venice — " + veniceModel,
			available: true,
			apply: func(next Model) Model {
				next.provider = provider.Venice{APIKey: next.veniceAPIKey, BaseURL: next.veniceBaseURL}
				next.requestModel = veniceModel
				return next
			},
		})
	}
	return options
}

func (m Model) currentModelIndex() int {
	if _, usingVenice := m.provider.(provider.Venice); usingVenice {
		return 1
	}
	return 0
}

func (m Model) showModelPicker() (tea.Model, tea.Cmd) {
	m.screen = modelScreen
	m.modelIndex = m.currentModelIndex()
	return m, nil
}

func (m Model) updateModelPicker(key string) (tea.Model, tea.Cmd) {
	options := m.modelOptions()
	switch key {
	case "esc":
		return m.showChat()
	case "up":
		if m.modelIndex > 0 {
			m.modelIndex--
		}
		return m, nil
	case "down":
		if m.modelIndex < len(options)-1 {
			m.modelIndex++
		}
		return m, nil
	case "enter":
		option := options[m.modelIndex]
		if !option.available {
			m.status = option.hint
			return m, nil
		}
		next := option.apply(m)
		next.screen = chatScreen
		next.status = "✓ Modelo activo: " + option.label
		return next, next.input.Focus()
	}
	return m, nil
}

func (m Model) modelPickerView() string {
	var body strings.Builder
	body.WriteString(logoStyle.Render("◆ ALICE") + " · " + mutedStyle.Render("MODELO") + "\n\n")
	for index, option := range m.modelOptions() {
		prefix := "  "
		style := mutedStyle
		if index == m.modelIndex {
			prefix = "● "
			style = lipgloss.NewStyle().Foreground(lavender).Bold(true)
		}
		label := option.label
		if !option.available {
			label += "  (sin configurar)"
		}
		body.WriteString(style.Render(prefix+label) + "\n")
	}
	body.WriteString("\n" + mutedStyle.Render("↑/↓ elegir · Enter confirmar · Esc volver"))
	column := lipgloss.NewStyle().Width(m.viewport.Width).Render(body.String())
	return m.placeColumn(column)
}
