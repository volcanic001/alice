package ui

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/volcanic001/alice/internal/chat"
)

func statsRecord(model string, at time.Time) chat.UsageRecord {
	return chat.UsageRecord{
		Model: model, RequestedAt: at,
		PromptTokens: 1_900, CompletionTokens: 531, TotalTokens: 2_431,
		PromptCacheHitTokens: 1_024, PromptCacheMissTokens: 876,
	}
}

func TestStatsIsLocalAndShowsAllPeriods(t *testing.T) {
	model, provider := testCommandModel(t)
	model.usageRecords = []chat.UsageRecord{statsRecord("deepseek-flash", time.Now())}
	model = submitInput(t, model, "/stats")

	if len(provider.requests) != 0 || len(model.messages) != 0 || model.busy || model.input.Value() != "" {
		t.Fatalf("/stats no fue local: requests=%d messages=%d busy=%v input=%q", len(provider.requests), len(model.messages), model.busy, model.input.Value())
	}
	for _, expected := range []string{"API Usage", "Today", "Last 7 days", "Current month", "Historical"} {
		if !strings.Contains(model.localOutput, expected) {
			t.Fatalf("/stats no muestra %q: %q", expected, model.localOutput)
		}
	}
	if !strings.Contains(xansi.Strip(model.View()), "API Usage") {
		t.Fatalf("/stats no se muestra dentro del chat: %q", xansi.Strip(model.View()))
	}
}

func TestStatsShowsTokenTotalsAndCalculatedCosts(t *testing.T) {
	at := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	output := renderUsageStats([]chat.UsageRecord{statsRecord("deepseek-flash", at)}, at)
	for _, expected := range []string{
		"Requests        1", "Tokens          2,431", "Prompt          1,900",
		"Completion      531", "Cache hit       1,024", "Cache miss      876",
		"Cost            $0.000453072",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("resumen no contiene %q: %q", expected, output)
		}
	}
}

func TestStatsHandlesEmptyHistory(t *testing.T) {
	output := renderUsageStats(nil, time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC))
	if strings.Count(output, "Requests        0") != 4 || strings.Count(output, "Cost            $0.000000") != 4 {
		t.Fatalf("historial vacío inesperado: %q", output)
	}
}

func TestStatsHandlesUnknownPricingWithoutFalseCost(t *testing.T) {
	at := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	output := renderUsageStats([]chat.UsageRecord{statsRecord("unknown-model", at)}, at)
	if !strings.Contains(output, "Tokens          2,431") || !strings.Contains(output, "Requests        1") {
		t.Fatalf("faltan totales de uso: %q", output)
	}
	if strings.Count(output, "Cost            "+unavailableCost) != 4 || !strings.Contains(output, "Cost unavailable:") {
		t.Fatalf("pricing desconocido no fue señalado: %q", output)
	}
	if strings.Contains(output, "$0.000000") {
		t.Fatalf("se mostró un coste falso: %q", output)
	}
}
