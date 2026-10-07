package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const thinkingSpan = 2 * (thinkingCells - 1)

func barCells(frame int) string {
	return ansi.Strip(thinkingBar(frame))
}

// litCells devuelve, por celda, el nivel de la rampa que la pinta, o -1 si está
// apagada. Permite comprobar el degradado sin depender de los hex concretos.
func litCells(t *testing.T, frame int) []int {
	t.Helper()
	position, direction := thinkingHead(frame)
	levels := make([]int, thinkingCells)
	for cell := range thinkingCells {
		behind := (position - cell) * direction
		if behind >= 0 && behind < thinkingTrail {
			levels[cell] = thinkingTrail - 1 - behind
			continue
		}
		levels[cell] = -1
	}
	return levels
}

func TestThinkingBarKeepsFixedWidth(t *testing.T) {
	for frame := range thinkingSpan * 2 {
		if cells := []rune(barCells(frame)); len(cells) != thinkingCells {
			t.Fatalf("fotograma %d: %d celdas, se esperaban %d", frame, len(cells), thinkingCells)
		}
	}
}

// La cabeza nunca se apaga: era el defecto de la primera versión, que al dar la
// vuelta dejaba la pista partida en dos extremos.
func TestThinkingBarAlwaysShowsExactlyOneHead(t *testing.T) {
	for frame := range thinkingSpan * 2 {
		heads := 0
		for _, level := range litCells(t, frame) {
			if level == thinkingTrail-1 {
				heads++
			}
		}
		if heads != 1 {
			t.Fatalf("fotograma %d: %d cabezas, se esperaba 1 (%q)", frame, heads, barCells(frame))
		}
	}
}

// El recorrido es continuo: la cabeza se mueve una celda por fotograma y nunca
// salta de un extremo al otro.
func TestThinkingHeadMovesOneCellPerFrame(t *testing.T) {
	previous, _ := thinkingHead(0)
	for frame := 1; frame <= thinkingSpan*2; frame++ {
		position, _ := thinkingHead(frame)
		step := position - previous
		if step != 1 && step != -1 {
			t.Fatalf("fotograma %d: la cabeza saltó de %d a %d", frame, previous, position)
		}
		previous = position
	}
}

// Ida y vuelta: toca ambos extremos una sola vez por ciclo y vuelve al inicio.
func TestThinkingHeadBouncesBetweenEnds(t *testing.T) {
	var positions []int
	for frame := range thinkingSpan {
		position, _ := thinkingHead(frame)
		positions = append(positions, position)
	}
	left, right := 0, 0
	for _, position := range positions {
		switch position {
		case 0:
			left++
		case thinkingCells - 1:
			right++
		}
	}
	if left != 1 || right != 1 {
		t.Fatalf("extremos visitados %d veces a la izquierda y %d a la derecha, se esperaba 1 y 1: %v", left, right, positions)
	}
	if start, _ := thinkingHead(thinkingSpan); start != positions[0] {
		t.Fatalf("el ciclo no cierra: %d != %d", start, positions[0])
	}
}

// La estela se apaga de forma escalonada por detrás de la cabeza.
func TestThinkingTrailFadesBehindHead(t *testing.T) {
	for frame := range thinkingSpan * 2 {
		levels := litCells(t, frame)
		_, direction := thinkingHead(frame)
		previous := thinkingTrail
		for index := range levels {
			// Se recorre desde la cabeza hacia atrás: con la cabeza subiendo
			// la estela queda a su izquierda, y bajando queda a su derecha.
			cell := index
			if direction > 0 {
				cell = thinkingCells - 1 - index
			}
			if levels[cell] < 0 {
				continue
			}
			if levels[cell] >= previous {
				t.Fatalf("fotograma %d: la celda %d (nivel %d) no se apaga tras el nivel %d", frame, cell, levels[cell], previous)
			}
			previous = levels[cell]
		}
	}
}

func TestThinkingTrailStaysWithinTrack(t *testing.T) {
	for frame := range thinkingSpan * 2 {
		lit := 0
		for _, level := range litCells(t, frame) {
			if level >= 0 {
				lit++
			}
		}
		if lit < 1 || lit > thinkingTrail {
			t.Fatalf("fotograma %d: %d celdas encendidas, fuera de 1..%d (%q)", frame, lit, thinkingTrail, barCells(frame))
		}
	}
}

func TestThinkingBarHandlesNegativeFrames(t *testing.T) {
	if got, want := barCells(-thinkingSpan), barCells(0); got != want {
		t.Fatalf("fotograma negativo: %q != %q", got, want)
	}
}

func TestStatusLineShowsBarOnlyWhileBusy(t *testing.T) {
	model := Model{status: "Alice está pensando…", totalPages: 1}

	if idle := ansi.Strip(model.statusLine()); strings.Contains(idle, thinkingFull) {
		t.Fatalf("en reposo no debe dibujarse la barra: %q", idle)
	}

	model.busy = true
	busy := ansi.Strip(model.statusLine())
	if !strings.HasPrefix(busy, barCells(0)) {
		t.Fatalf("ocupada debe empezar por la barra: %q", busy)
	}
	if !strings.Contains(busy, "Alice está pensando…") {
		t.Fatalf("el estado debe seguir visible: %q", busy)
	}
}
