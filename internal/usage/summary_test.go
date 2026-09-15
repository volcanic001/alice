package usage

import (
	"testing"
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

func usageRecordAt(at time.Time, multiplier int) chat.UsageRecord {
	return chat.UsageRecord{
		Model: "deepseek-chat", RequestedAt: at,
		PromptTokens: 2 * multiplier, CompletionTokens: 3 * multiplier, TotalTokens: 5 * multiplier,
		PromptCacheHitTokens: multiplier, PromptCacheMissTokens: multiplier,
	}
}

func assertSummary(t *testing.T, got Summary, requests, prompt, completion, total, hit, miss int) {
	t.Helper()
	want := Summary{
		Requests: requests, PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total,
		PromptCacheHitTokens: hit, PromptCacheMissTokens: miss,
	}
	if got != want {
		t.Fatalf("resumen inesperado: obtuve %#v, quiero %#v", got, want)
	}
}

func TestSummarizeAtCalendarPeriodsAndTokenTotals(t *testing.T) {
	location := time.FixedZone("UTC-3", -3*60*60)
	now := time.Date(2026, time.March, 15, 12, 0, 0, 0, location)
	records := []chat.UsageRecord{
		usageRecordAt(time.Date(2026, time.March, 15, 8, 0, 0, 0, location), 1),
		usageRecordAt(time.Date(2026, time.March, 15, 9, 0, 0, 0, location), 2),
		usageRecordAt(time.Date(2026, time.March, 14, 23, 0, 0, 0, location), 3),
		usageRecordAt(time.Date(2026, time.March, 9, 0, 0, 0, 0, location), 4),
		usageRecordAt(time.Date(2026, time.March, 8, 23, 59, 0, 0, location), 5),
		usageRecordAt(time.Date(2026, time.February, 28, 23, 59, 0, 0, location), 6),
	}
	summaries := summarizeIn(records, now, location)

	assertSummary(t, summaries.Today, 2, 6, 9, 15, 3, 3)
	assertSummary(t, summaries.Last7Days, 4, 20, 30, 50, 10, 10)
	assertSummary(t, summaries.CurrentMonth, 5, 30, 45, 75, 15, 15)
	assertSummary(t, summaries.Historical, 6, 42, 63, 105, 21, 21)
}

func TestSummarizeAtUsesLocalDateInsteadOfUTCDate(t *testing.T) {
	location := time.FixedZone("UTC-3", -3*60*60)
	originalLocation := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = originalLocation })
	previousLocalDay := usageRecordAt(time.Date(2026, time.March, 15, 1, 30, 0, 0, time.UTC), 1)
	currentLocalDay := usageRecordAt(time.Date(2026, time.March, 15, 3, 30, 0, 0, time.UTC), 2)
	summaries := SummarizeAt([]chat.UsageRecord{previousLocalDay, currentLocalDay}, time.Date(2026, time.March, 15, 12, 0, 0, 0, location))

	assertSummary(t, summaries.Today, 1, 4, 6, 10, 2, 2)
	assertSummary(t, summaries.Last7Days, 2, 6, 9, 15, 3, 3)
}

func TestSummarizeEmptyRecords(t *testing.T) {
	summaries := summarizeIn(nil, time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC), time.UTC)
	assertSummary(t, summaries.Today, 0, 0, 0, 0, 0, 0)
	assertSummary(t, summaries.Last7Days, 0, 0, 0, 0, 0, 0)
	assertSummary(t, summaries.CurrentMonth, 0, 0, 0, 0, 0, 0)
	assertSummary(t, summaries.Historical, 0, 0, 0, 0, 0, 0)
}
