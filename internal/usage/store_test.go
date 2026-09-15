package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

func TestStoreAppendAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	store := New(path)
	first := chat.UsageRecord{
		Model: "deepseek-chat", PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18,
		PromptCacheHitTokens: 4, PromptCacheMissTokens: 7, RequestedAt: time.Date(2026, 9, 14, 10, 0, 0, 0, time.Local),
	}
	second := chat.UsageRecord{Model: "deepseek-reasoner", RequestedAt: time.Date(2026, 9, 14, 11, 0, 0, 0, time.Local)}
	if err := store.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(second); err != nil {
		t.Fatal(err)
	}
	records, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Model != first.Model || records[0].PromptTokens != first.PromptTokens || records[0].CompletionTokens != first.CompletionTokens || records[0].TotalTokens != first.TotalTokens || records[0].PromptCacheHitTokens != first.PromptCacheHitTokens || records[0].PromptCacheMissTokens != first.PromptCacheMissTokens || !records[0].RequestedAt.Equal(first.RequestedAt) || records[1].Model != second.Model || !records[1].RequestedAt.Equal(second.RequestedAt) {
		t.Fatalf("registros inesperados: %#v", records)
	}
	if records[1].PromptTokens != 0 || records[1].CompletionTokens != 0 || records[1].TotalTokens != 0 || records[1].PromptCacheHitTokens != 0 || records[1].PromptCacheMissTokens != 0 {
		t.Fatalf("campos opcionales no quedaron en cero: %#v", records[1])
	}
}

func TestStoreLoadMissingEmptyAndInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	store := New(path)
	records, err := store.Load()
	if err != nil || len(records) != 0 {
		t.Fatalf("archivo inexistente: records=%#v err=%v", records, err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	records, err = store.Load()
	if err != nil || len(records) != 0 {
		t.Fatalf("archivo vacío: records=%#v err=%v", records, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("se esperaba un error seguro para JSON inválido")
	}
}
