package usage

import (
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

// Summary is the aggregate token usage for one calendar period.
type Summary struct {
	Requests              int
	PromptTokens          int
	CompletionTokens      int
	TotalTokens           int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
}

// Summaries contains the reusable consumption periods for Alice.
type Summaries struct {
	Today        Summary
	Last7Days    Summary
	CurrentMonth Summary
	Historical   Summary
}

// Summarize calculates periods relative to the current local system time.
func Summarize(records []chat.UsageRecord) Summaries {
	return SummarizeAt(records, time.Now())
}

// SummarizeAt calculates periods relative to now in the local system timezone.
func SummarizeAt(records []chat.UsageRecord, now time.Time) Summaries {
	return summarizeIn(records, now, time.Local)
}

// summarizeIn exists to keep calendar-boundary tests independent of the host timezone.
func summarizeIn(records []chat.UsageRecord, now time.Time, location *time.Location) Summaries {
	localNow := now.In(location)
	todayStart := startOfDay(localNow)
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	sevenDayStart := todayStart.AddDate(0, 0, -6)
	monthStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, location)
	nextMonthStart := monthStart.AddDate(0, 1, 0)

	var summaries Summaries
	for _, record := range records {
		add(&summaries.Historical, record)
		timestamp := record.RequestedAt.In(location)
		if inRange(timestamp, todayStart, tomorrowStart) {
			add(&summaries.Today, record)
		}
		if inRange(timestamp, sevenDayStart, tomorrowStart) {
			add(&summaries.Last7Days, record)
		}
		if inRange(timestamp, monthStart, nextMonthStart) {
			add(&summaries.CurrentMonth, record)
		}
	}
	return summaries
}

func startOfDay(timestamp time.Time) time.Time {
	return time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())
}

func inRange(timestamp, start, end time.Time) bool {
	return !timestamp.Before(start) && timestamp.Before(end)
}

func add(summary *Summary, record chat.UsageRecord) {
	summary.Requests++
	summary.PromptTokens += record.PromptTokens
	summary.CompletionTokens += record.CompletionTokens
	summary.TotalTokens += record.TotalTokens
	summary.PromptCacheHitTokens += record.PromptCacheHitTokens
	summary.PromptCacheMissTokens += record.PromptCacheMissTokens
}
