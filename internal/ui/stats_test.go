package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/usage"
)

func statsRecord(model string, at time.Time) chat.UsageRecord {
	return chat.UsageRecord{
		Model: model, RequestedAt: at,
		PromptTokens: 1_900, CompletionTokens: 531, TotalTokens: 2_431,
		PromptCacheHitTokens: 1_024, PromptCacheMissTokens: 876,
	}
}

func resizeStatsModel(t *testing.T, model Model, width, height int) Model {
	t.Helper()
	next, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

func TestStatsOpensDedicatedLocalViewAndEscReturnsToChat(t *testing.T) {
	model, provider := testCommandModel(t)
	originalMessages := []chat.Message{{Role: "user", Content: "conserva esto"}, {Role: "assistant", Content: "respuesta"}}
	model.messages = append([]chat.Message(nil), originalMessages...)
	model.usageRecords = []chat.UsageRecord{statsRecord("deepseek-flash", time.Now())}
	model = submitInput(t, model, "/stats")

	if model.screen != statsScreen || len(provider.requests) != 0 || model.busy || model.input.Value() != "" {
		t.Fatalf("/stats no abrió localmente: screen=%d requests=%d busy=%v input=%q", model.screen, len(provider.requests), model.busy, model.input.Value())
	}
	if len(model.messages) != len(originalMessages) || model.messages[0].Content != originalMessages[0].Content || model.messages[1].Content != originalMessages[1].Content {
		t.Fatalf("Stats modificó la conversación: %#v", model.messages)
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)
	if model.screen != chatScreen || !model.input.Focused() {
		t.Fatalf("Esc no volvió al chat: screen=%d focused=%v", model.screen, model.input.Focused())
	}
	if len(model.messages) != len(originalMessages) || model.messages[0].Content != originalMessages[0].Content || model.messages[1].Content != originalMessages[1].Content {
		t.Fatalf("volver de Stats destruyó la conversación: %#v", model.messages)
	}
}

func TestStatsRendersPeriodsAndRealValues(t *testing.T) {
	model, _ := testCommandModel(t)
	at := time.Now()
	record := statsRecord("deepseek-flash", at)
	expectedCost, err := (usage.CostCalculator{Catalog: usage.DeepSeekPricingCatalog()}).Calculate(record)
	if err != nil {
		t.Fatal(err)
	}
	model.usageRecords = []chat.UsageRecord{record}
	model = submitInput(t, model, "/stats")
	view := xansi.Strip(model.View())

	for _, expected := range []string{
		"TODAY", "LAST 7 DAYS", "CURRENT MONTH", "HISTORICAL",
		"2,431", "1,900", "531", "1,024", "876", formatUSD(expectedCost.Cost.TotalUSD),
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Stats no muestra %q: %q", expected, view)
		}
	}
	if strings.Count(view, "Requests") != 4 || strings.Count(view, "Tokens") != 4 {
		t.Fatalf("faltan valores principales por período: %q", view)
	}
}

func TestStatsEmptyState(t *testing.T) {
	model, _ := testCommandModel(t)
	model = submitInput(t, model, "/stats")
	view := xansi.Strip(model.View())
	if !strings.Contains(view, "No API usage recorded yet.") {
		t.Fatalf("falta estado vacío: %q", view)
	}
	if strings.Contains(view, "TODAY") || strings.Contains(view, "$0.000000") {
		t.Fatalf("estado vacío mostró secciones en cero: %q", view)
	}
}

func TestStatsUnknownPricingKeepsUsageVisible(t *testing.T) {
	model, _ := testCommandModel(t)
	model.usageRecords = []chat.UsageRecord{statsRecord("unknown-model", time.Now())}
	model = submitInput(t, model, "/stats")
	view := xansi.Strip(model.View())
	if !strings.Contains(view, "2,431") || !strings.Contains(view, "Requests") {
		t.Fatalf("pricing desconocido ocultó el uso: %q", view)
	}
	if strings.Count(view, unavailableCost) != 4 || !strings.Contains(view, "Pricing incomplete") {
		t.Fatalf("pricing incompleto no fue indicado: %q", view)
	}
}

func TestStatsNarrowTerminalUsesOneColumnWithoutOverflow(t *testing.T) {
	model, _ := testCommandModel(t)
	model = resizeStatsModel(t, model, 40, 40)
	model.usageRecords = []chat.UsageRecord{statsRecord("deepseek-flash", time.Now())}
	model = submitInput(t, model, "/stats")
	view := xansi.Strip(model.View())

	for _, line := range strings.Split(view, "\n") {
		if xansi.StringWidth(line) > 40 {
			t.Fatalf("overflow estrecho de %d columnas: %q", xansi.StringWidth(line), line)
		}
		if strings.Contains(line, "TODAY") && strings.Contains(line, "LAST 7 DAYS") {
			t.Fatalf("terminal estrecha usó dos columnas: %q", line)
		}
	}
}

func TestStatsWideTerminalUsesTwoColumns(t *testing.T) {
	model, _ := testCommandModel(t)
	model = resizeStatsModel(t, model, 100, 40)
	model.usageRecords = []chat.UsageRecord{statsRecord("deepseek-flash", time.Now())}
	model = submitInput(t, model, "/stats")
	view := xansi.Strip(model.View())

	var foundPair bool
	for _, line := range strings.Split(view, "\n") {
		if xansi.StringWidth(line) > 100 {
			t.Fatalf("overflow ancho de %d columnas: %q", xansi.StringWidth(line), line)
		}
		if strings.Contains(line, "TODAY") && strings.Contains(line, "LAST 7 DAYS") {
			foundPair = true
		}
	}
	if !foundPair {
		t.Fatalf("terminal ancha no usó dos columnas: %q", view)
	}
}

func TestHelpAliasesIncludeStats(t *testing.T) {
	for _, command := range []string{"/help", "/ayuda"} {
		model, provider := testCommandModel(t)
		model = submitInput(t, model, command)
		view := xansi.Strip(model.View())
		if model.screen != helpScreen || len(provider.requests) != 0 || !strings.Contains(view, "/stats") {
			t.Fatalf("%s no muestra /stats localmente: screen=%d requests=%d view=%q", command, model.screen, len(provider.requests), view)
		}
	}
}
