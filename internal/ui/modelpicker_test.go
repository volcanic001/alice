package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/provider"
	"github.com/volcanic001/alice/internal/store"
)

func newModelPickerTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	model, err := New(database, provider.DeepSeek{APIKey: "ds-key"}, "deepseek-flash", 0.7)
	if err != nil {
		t.Fatal(err)
	}
	model.SetVenice("", "", provider.VeniceRecommendedModel)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 84, Height: 28})
	return next.(Model)
}

func TestModelCommandShowsPickerWithVeniceUnavailable(t *testing.T) {
	model := newModelPickerTestModel(t)

	model.input.SetValue("/model")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.screen != modelScreen {
		t.Fatalf("se esperaba modelScreen, hay: %v", model.screen)
	}
	options := model.modelOptions()
	if len(options) != 2 || options[0].label == "" || options[1].available {
		t.Fatalf("opciones inesperadas: %+v", options)
	}
}

func TestModelPickerSelectingUnavailableVeniceShowsHint(t *testing.T) {
	model := newModelPickerTestModel(t)
	model, _ = asModel(model.showModelPicker())
	model.modelIndex = 1 // Venice, sin clave configurada

	next, _ := model.updateModelPicker("enter")
	model = next.(Model)

	if _, usingVenice := model.provider.(provider.Venice); usingVenice {
		t.Fatal("no debería activar Venice sin clave configurada")
	}
	if !strings.Contains(model.status, "/venice-key") {
		t.Fatalf("se esperaba indicación de configurar la clave, hay: %q", model.status)
	}
}

func TestModelPickerSwitchesToVeniceAndBack(t *testing.T) {
	model := newModelPickerTestModel(t)
	model.input.SetValue("/venice-key clave-venice")
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), cmd, nil)

	model, _ = asModel(model.showModelPicker())
	if model.modelIndex != 1 {
		t.Fatalf("se esperaba que el picker abriera en Venice (activo), hay índice %d", model.modelIndex)
	}

	model.modelIndex = 0
	next, _ = model.updateModelPicker("enter")
	model = next.(Model)
	if _, usingVenice := model.provider.(provider.Venice); usingVenice {
		t.Fatal("se esperaba volver a DeepSeek")
	}
	if model.requestModel != "deepseek-flash" {
		t.Fatalf("modelo inesperado tras volver: %q", model.requestModel)
	}

	model.screen = modelScreen
	model.modelIndex = 1
	next, _ = model.updateModelPicker("enter")
	model = next.(Model)
	veniceProvider, usingVenice := model.provider.(provider.Venice)
	if !usingVenice || veniceProvider.APIKey != "clave-venice" {
		t.Fatalf("se esperaba Venice activo con la clave guardada, hay: %+v", model.provider)
	}
	if model.requestModel != provider.VeniceRecommendedModel {
		t.Fatalf("se esperaba el modelo recomendado, hay: %q", model.requestModel)
	}
}

func asModel(m tea.Model, _ tea.Cmd) (Model, tea.Cmd) {
	return m.(Model), nil
}
