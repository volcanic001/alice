package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/memory"
	"github.com/volcanic001/alice/internal/provider"
	"github.com/volcanic001/alice/internal/search"
	"github.com/volcanic001/alice/internal/store"
	"github.com/volcanic001/alice/internal/ui"
	"github.com/volcanic001/alice/internal/usage"
)

func main() {
	configuration, err := config.Load()
	if err != nil {
		fail(err)
	}
	database, err := store.Open(configuration.DataPath)
	if err != nil {
		fail(err)
	}
	defer database.Close()
	model, err := ui.New(database, provider.DeepSeek{APIKey: configuration.APIKey, BaseURL: configuration.BaseURL}, configuration.Model, configuration.Temperature, usage.New(configuration.UsagePath))
	if err != nil {
		fail(err)
	}
	if configuration.Mem0APIKey != "" {
		model.SetMemory(memory.New(configuration.Mem0APIKey, configuration.Mem0UserID))
	}
	if configuration.BraveAPIKey != "" {
		model.SetWebSearch(search.New(configuration.BraveAPIKey))
	}
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "alice:", err)
	os.Exit(1)
}
