package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/chat"
)

func paginationModel(height int, content string) Model {
	input := textarea.New()
	input.SetHeight(1)
	model := Model{input: input, viewport: viewport.New(40, height)}
	model.setPages(content)
	return model
}

func TestPaginationSplitsContentByVisibleLines(t *testing.T) {
	model := paginationModel(3, "uno\ndos\ntres\ncuatro\ncinco\nseis\nsiete")
	if model.totalPages != 3 {
		t.Fatalf("páginas = %d, se esperaban 3", model.totalPages)
	}
	if model.pages[0] != "uno\ndos\ntres" || model.pages[2] != "siete" {
		t.Fatalf("división inesperada: %#v", model.pages)
	}
}

func TestPaginationNavigationStopsAtFirstAndLastPage(t *testing.T) {
	model := paginationModel(2, "uno\ndos\ntres\ncuatro\ncinco")

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = next.(Model)
	if model.currentPage != 0 {
		t.Fatalf("PgUp en la primera página cambió a %d", model.currentPage)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = next.(Model)
	if model.currentPage != 1 {
		t.Fatalf("PgDn no avanzó: %d", model.currentPage)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = next.(Model)
	if model.currentPage != model.totalPages-1 {
		t.Fatalf("PgDn en la última página cambió a %d", model.currentPage)
	}
}

func TestPaginationHomeAndEnd(t *testing.T) {
	model := paginationModel(2, "uno\ndos\ntres\ncuatro\ncinco")
	model.goToPage(1)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = next.(Model)
	if model.currentPage != model.totalPages-1 {
		t.Fatalf("End no llegó a la última página: %d", model.currentPage)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = next.(Model)
	if model.currentPage != 0 {
		t.Fatalf("Home no llegó a la primera página: %d", model.currentPage)
	}
}

func TestPaginationResizeRecalculatesPages(t *testing.T) {
	model := paginationModel(4, "")
	model.markdownCache = make(map[string]string)
	model.messages = []chat.Message{{Role: "user", Content: strings.Join([]string{"1", "2", "3", "4", "5", "6", "7", "8"}, "\n")}}
	model.width, model.height = 44, 20
	model.resize()
	pagesBefore := model.totalPages
	model.width, model.height = 44, 10
	model.resize()
	if model.totalPages <= pagesBefore {
		t.Fatalf("el resize no incrementó páginas: antes=%d después=%d", pagesBefore, model.totalPages)
	}
}

func TestPaginationSinglePage(t *testing.T) {
	model := paginationModel(5, "uno\ndos\ntres")
	if model.totalPages != 1 || model.currentPage != 0 {
		t.Fatalf("paginación de contenido corto: actual=%d total=%d", model.currentPage, model.totalPages)
	}
}
