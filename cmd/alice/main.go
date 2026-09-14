package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/provider"
	"github.com/volcanic001/alice/internal/store"
	"github.com/volcanic001/alice/internal/ui"
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
	model, err := ui.New(database, provider.DeepSeek{APIKey: configuration.APIKey, BaseURL: configuration.BaseURL})
	if err != nil {
		fail(err)
	}
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "alice:", err)
	os.Exit(1)
}
