package ui

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/provider"
	"github.com/volcanic001/alice/internal/store"
	"github.com/volcanic001/alice/internal/usage"
)

func TestContextPercentUsesConfirmedTotalTokens(t *testing.T) {
	percent, ok := contextPercent(2_517, provider.DeepSeekFlashContextWindow)
	if !ok || math.Abs(percent-0.24003982543945312) > 0.0000001 {
		t.Fatalf("porcentaje inesperado: %.10f, válido=%v", percent, ok)
	}
	if got := formatContextPercent(percent); got != "0.24%" {
		t.Fatalf("formato bajo 1%% = %q", got)
	}

	percent, ok = contextPercent(58_720, provider.DeepSeekFlashContextWindow)
	if !ok || formatContextPercent(percent) != "5.6%" {
		t.Fatalf("formato entre 1%% y 10%% inesperado: %.10f", percent)
	}

	percent, ok = contextPercent(230_687, provider.DeepSeekFlashContextWindow)
	if !ok || formatContextPercent(percent) != "22%" {
		t.Fatalf("formato desde 10%% inesperado: %.10f", percent)
	}

	percent, ok = contextPercent(provider.DeepSeekFlashContextWindow+1, provider.DeepSeekFlashContextWindow)
	if !ok || percent != 100 {
		t.Fatalf("el porcentaje no se limitó a 100: %.10f", percent)
	}
}

func TestContextMeterFillMatchesPercentage(t *testing.T) {
	model, _ := testCommandModel(t)
	model.usageRecords = []chat.UsageRecord{{
		ConversationID: model.conversation.ID,
		Model:          provider.DeepSeekFlashModel,
		TotalTokens:    398_459,
	}}
	header := xansi.Strip(model.chatHeader(model.viewport.Width))

	if filled, empty := strings.Count(header, "●"), strings.Count(header, "○"); filled != 5 || empty != 9 {
		t.Fatalf("relleno no proporcional para 38%%: llenos=%d vacíos=%d header=%q", filled, empty, header)
	}
	veryLow := xansi.Strip(contextMeter(0.24, true, maxContextSegments))
	if strings.Count(veryLow, "●") != 0 || strings.Count(veryLow, "○") != maxContextSegments {
		t.Fatalf("0.24%% llenó un segmento sin alcanzarlo matemáticamente: %q", veryLow)
	}
}

func TestContextMeterStartsUnknownAndOmitsConversationTitle(t *testing.T) {
	model, _ := testCommandModel(t)
	model.conversation.Title = "Título que no debe aparecer"
	header := xansi.Strip(model.chatHeader(model.viewport.Width))

	if !strings.Contains(header, "◆ ALICE") || !strings.Contains(header, "Context") || !strings.Contains(header, "--%") {
		t.Fatalf("header desconocido inesperado: %q", header)
	}
	if strings.Contains(header, model.conversation.Title) {
		t.Fatalf("el header todavía contiene el título: %q", header)
	}
	if strings.Count(header, "●") != 0 || strings.Count(header, "○") != maxContextSegments {
		t.Fatalf("estado desconocido inesperado: %q", header)
	}
}

func TestContextMeterUpdatesOnlyAfterFinalUsage(t *testing.T) {
	model, _ := testCommandModel(t)
	conversationID := model.conversation.ID
	model.usageRecords = []chat.UsageRecord{{
		ConversationID: conversationID,
		Model:          provider.DeepSeekFlashModel,
		TotalTokens:    104_858,
	}}
	before, ok := model.confirmedContextPercent()
	if !ok {
		t.Fatal("faltó la medición inicial")
	}

	next, _ := model.Update(streamMsg(chat.Event{Usage: &chat.UsageRecord{
		Model:          provider.DeepSeekFlashModel,
		ConversationID: conversationID,
		TotalTokens:    209_715,
	}}))
	model = next.(Model)
	during, ok := model.confirmedContextPercent()
	if !ok || during != before {
		t.Fatalf("el medidor cambió antes de finalizar: antes=%.4f durante=%.4f", before, during)
	}

	next, _ = model.Update(streamMsg(chat.Event{Done: true}))
	model = next.(Model)
	after, ok := model.confirmedContextPercent()
	if !ok || after <= during || model.usageRecords[len(model.usageRecords)-1].ConversationID != conversationID {
		t.Fatalf("usage final no aplicado: antes=%.4f después=%.4f records=%#v", during, after, model.usageRecords)
	}
}

func TestContextMeterPreservesLastUsageAfterFailure(t *testing.T) {
	model, _ := testCommandModel(t)
	model.usageRecords = []chat.UsageRecord{{
		ConversationID: model.conversation.ID,
		Model:          provider.DeepSeekFlashModel,
		TotalTokens:    104_858,
	}}
	want, _ := model.confirmedContextPercent()

	next, _ := model.Update(streamMsg(chat.Event{Err: context.Canceled}))
	model = next.(Model)
	got, ok := model.confirmedContextPercent()
	if !ok || got != want {
		t.Fatalf("un fallo alteró la última medición: obtuve %.4f, esperaba %.4f", got, want)
	}
}

func TestContextMeterFollowsActiveConversation(t *testing.T) {
	model, _ := testHistoryModel(t, "Primera", "Segunda", "Sin medición")
	model.usageRecords = []chat.UsageRecord{
		{ConversationID: model.conversations[0].ID, Model: provider.DeepSeekFlashModel, TotalTokens: 104_858},
		{ConversationID: model.conversations[1].ID, Model: provider.DeepSeekFlashModel, TotalTokens: 209_715},
	}

	first, ok := model.confirmedContextPercent()
	if !ok {
		t.Fatal("la primera conversación no recuperó su medición")
	}
	next, _ := model.openConversation(1)
	model = next.(Model)
	second, ok := model.confirmedContextPercent()
	if !ok || second <= first {
		t.Fatalf("la segunda conversación no usó su medición: primera=%.4f segunda=%.4f", first, second)
	}
	next, _ = model.openConversation(2)
	model = next.(Model)
	if _, ok := model.confirmedContextPercent(); ok {
		t.Fatal("una conversación sin usage no mostró estado desconocido")
	}
}

func TestContextMeterRestoresPersistedConversationUsage(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	conversation, err := database.CreateConversation()
	if err != nil {
		t.Fatal(err)
	}
	usageStore := usage.New(filepath.Join(t.TempDir(), "usage.json"))
	if err := usageStore.Append(chat.UsageRecord{
		ConversationID: conversation.ID,
		Model:          provider.DeepSeekFlashModel,
		TotalTokens:    17_826,
	}); err != nil {
		t.Fatal(err)
	}

	model, err := New(database, &recordingProvider{}, provider.DeepSeekFlashModel, 0.7, usageStore)
	if err != nil {
		t.Fatal(err)
	}
	percent, ok := model.confirmedContextPercent()
	if !ok || formatContextPercent(percent) != "1.7%" {
		t.Fatalf("usage persistido no restaurado: porcentaje=%.4f válido=%v", percent, ok)
	}
}

func TestContextHeaderAlignsIndependentGroups(t *testing.T) {
	model, _ := testCommandModel(t)
	model.usageRecords = []chat.UsageRecord{{
		ConversationID: model.conversation.ID,
		Model:          provider.DeepSeekFlashModel,
		TotalTokens:    230_687,
	}}
	header60 := xansi.Strip(model.chatHeader(60))
	header72 := xansi.Strip(model.chatHeader(72))

	if !strings.HasPrefix(header60, "◆ ALICE") || !strings.HasPrefix(header72, "◆ ALICE") {
		t.Fatalf("ALICE no quedó alineada a la izquierda: %q / %q", header60, header72)
	}
	if xansi.StringWidth(header60) != 60 || xansi.StringWidth(header72) != 72 {
		t.Fatalf("el grupo derecho no llegó al borde: widths=%d/%d", xansi.StringWidth(header60), xansi.StringWidth(header72))
	}
	if !strings.HasSuffix(header60, "22%") || !strings.HasSuffix(header72, "22%") {
		t.Fatalf("el porcentaje no quedó al extremo derecho: %q / %q", header60, header72)
	}
	right60 := strings.Index(header60, "Context")
	right72 := strings.Index(header72, "Context")
	if right60 < 0 || right72 < 0 {
		t.Fatalf("faltó el grupo Context: %q / %q", header60, header72)
	}
	if right72-right60 != 12 {
		t.Fatalf("el espacio flexible no siguió el ancho: posiciones=%d/%d", right60, right72)
	}
	if !strings.Contains(header60, "●●●○") {
		t.Fatalf("los puntos no forman una unidad visual compacta: %q", header60)
	}
}

func TestContextMeterUsesCompactLabelBeforeReducingSegments(t *testing.T) {
	model, _ := testCommandModel(t)
	header := xansi.Strip(model.chatHeader(35))

	if strings.Contains(header, "Context") || !strings.Contains(header, "CTX") {
		t.Fatalf("el label no cambió a CTX: %q", header)
	}
	if count := strings.Count(header, "○"); count != maxContextSegments {
		t.Fatalf("se redujeron segmentos antes de abreviar el label: %d en %q", count, header)
	}
}

func TestContextMeterShrinksWithoutWrappingInNarrowTerminal(t *testing.T) {
	model, _ := testCommandModel(t)
	model.conversation.Title = "Título oculto"
	next, _ := model.Update(windowSizeForTest(32, 24))
	model = next.(Model)
	header := xansi.Strip(model.chatHeader(model.viewport.Width))

	if strings.Contains(header, "\n") || xansi.StringWidth(header) > model.viewport.Width {
		t.Fatalf("header estrecho hizo wrap o overflow: width=%d header=%q", model.viewport.Width, header)
	}
	if !strings.Contains(header, "◆ ALICE") || !strings.Contains(header, "CTX") || !strings.Contains(header, "--%") {
		t.Fatalf("faltan elementos del header estrecho: %q", header)
	}
	if count := strings.Count(header, "○"); count < 1 || count >= maxContextSegments {
		t.Fatalf("el medidor no se redujo proporcionalmente al ancho: %d en %q", count, header)
	}
	if strings.Contains(header, model.conversation.Title) {
		t.Fatalf("el título apareció en el header estrecho: %q", header)
	}
}

func TestContextMeterDropsSegmentsBeforeTruncatingPercent(t *testing.T) {
	model, _ := testCommandModel(t)
	model.usageRecords = []chat.UsageRecord{{
		ConversationID: model.conversation.ID,
		Model:          provider.DeepSeekFlashModel,
		TotalTokens:    2_517,
	}}
	header := xansi.Strip(model.chatHeader(13))

	if strings.Count(header, "●") != 0 || strings.Count(header, "○") != 0 {
		t.Fatalf("el medidor no desapareció en ancho extremo: %q", header)
	}
	if !strings.HasSuffix(header, "0.24%") {
		t.Fatalf("el porcentaje fue truncado: %q", header)
	}
	if strings.Contains(header, "\n") || xansi.StringWidth(header) > 13 {
		t.Fatalf("el header extremo hizo wrap o overflow: %q", header)
	}
}

func TestContextHeaderNeverWraps(t *testing.T) {
	model, _ := testCommandModel(t)
	model.usageRecords = []chat.UsageRecord{{
		ConversationID: model.conversation.ID,
		Model:          provider.DeepSeekFlashModel,
		TotalTokens:    2_517,
	}}
	for width := 1; width <= 80; width++ {
		header := xansi.Strip(model.chatHeader(width))
		if strings.Contains(header, "\n") || xansi.StringWidth(header) > width {
			t.Fatalf("header inválido para width=%d: %q", width, header)
		}
	}
}

func windowSizeForTest(width, height int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: width, Height: height}
}
