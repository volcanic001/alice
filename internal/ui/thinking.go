package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Indicador de trabajo inspirado en la barra de opencode: una cabeza encendida
// recorre una pista corta de cuadritos arrastrando una estela que se apaga por
// detrás. El recorrido es de ida y vuelta, no circular.
const (
	thinkingCells    = 8
	thinkingTrail    = 5
	thinkingInterval = 110 * time.Millisecond
	thinkingFull     = "■"
	thinkingEmpty    = "▪"
)

// thinkingRamp va del rastro más apagado a la cabeza. Son mezclas del lavanda
// de Alice sobre el color de la pista, para que la estela se funda con ella.
var thinkingRamp = [thinkingTrail]lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("#433B5B")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("#594E7E")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("#7668AD")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("#9785E2")),
	lipgloss.NewStyle().Foreground(lavender),
}

var thinkingTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#2F2A3C"))

type thinkingTickMsg time.Time

func thinkingTick() tea.Cmd {
	return tea.Tick(thinkingInterval, func(t time.Time) tea.Msg {
		return thinkingTickMsg(t)
	})
}

// thinkingHead sitúa la cabeza y devuelve el sentido en el que llegó a esa
// celda. La ida y vuelta ocupa 2*(celdas-1) fotogramas: la cabeza avanza hasta
// el extremo, se da la vuelta y regresa, sin reaparecer nunca en el otro lado.
// El sentido es el de llegada, no el de salida, para que la estela quede
// siempre sobre el tramo ya recorrido.
func thinkingHead(frame int) (position, direction int) {
	span := 2 * (thinkingCells - 1)
	cycle := ((frame % span) + span) % span
	if cycle == 0 {
		return 0, -1
	}
	if cycle < thinkingCells {
		return cycle, 1
	}
	return span - cycle, -1
}

// thinkingBar dibuja un fotograma. Las celdas de la estela que caen fuera de la
// pista se recortan, así la cabeza se come su propio rastro al rebotar.
func thinkingBar(frame int) string {
	position, direction := thinkingHead(frame)
	var bar strings.Builder
	for cell := range thinkingCells {
		behind := (position - cell) * direction
		if behind >= 0 && behind < thinkingTrail {
			bar.WriteString(thinkingRamp[thinkingTrail-1-behind].Render(thinkingFull))
			continue
		}
		bar.WriteString(thinkingTrackStyle.Render(thinkingEmpty))
	}
	return bar.String()
}
