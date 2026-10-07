package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// drive imita el bucle de Bubble Tea sobre un modelo: ejecuta los comandos
// pendientes, despliega los tea.BatchMsg en sus comandos hijos y descarta los
// fotogramas de la barra de trabajo, que no influyen en el flujo del stream.
// Para cuando stop devuelve true o cuando se agotan los comandos.
func drive(t *testing.T, model Model, command tea.Cmd, stop func(Model) bool) Model {
	t.Helper()
	queue := []tea.Cmd{command}
	for step := 0; len(queue) > 0; step++ {
		if step > 100 {
			t.Fatal("el bucle de comandos no terminó")
		}
		if stop != nil && stop(model) {
			return model
		}
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		switch message := next().(type) {
		case tea.BatchMsg:
			queue = append(queue, message...)
		case thinkingTickMsg:
			// Fotograma de animación: no toca el flujo bajo prueba.
		default:
			updated, command := model.Update(message)
			model = updated.(Model)
			queue = append(queue, command)
		}
	}
	return model
}
