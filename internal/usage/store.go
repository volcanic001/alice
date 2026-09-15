// Package usage persists API usage records independently from the UI.
package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/volcanic001/alice/internal/chat"
)

// Store owns the local JSON file containing usage records.
type Store struct {
	path string
}

func New(path string) *Store { return &Store{path: path} }

func (s *Store) Path() string { return s.path }

// Load returns an empty list when no usage has been recorded yet.
func (s *Store) Load() ([]chat.UsageRecord, error) {
	contents, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leer uso: %w", err)
	}
	if strings.TrimSpace(string(contents)) == "" {
		return nil, nil
	}
	var records []chat.UsageRecord
	if err := json.Unmarshal(contents, &records); err != nil {
		return nil, fmt.Errorf("decodificar uso: %w", err)
	}
	return records, nil
}

// Append preserves all existing records and replaces the file atomically.
func (s *Store) Append(record chat.UsageRecord) error {
	records, err := s.Load()
	if err != nil {
		return err
	}
	records = append(records, record)
	return s.save(records)
}

func (s *Store) save(records []chat.UsageRecord) error {
	contents, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("codificar uso: %w", err)
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("crear directorio de uso: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".usage-*.json")
	if err != nil {
		return fmt.Errorf("crear archivo temporal de uso: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("proteger archivo temporal de uso: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("escribir uso: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("cerrar archivo temporal de uso: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("guardar uso: %w", err)
	}
	return nil
}
