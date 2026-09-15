package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/usage"
)

const unavailableCost = "Unavailable (pricing incomplete)"

func renderUsageStats(records []chat.UsageRecord, now time.Time) string {
	tokenSummaries := usage.SummarizeAt(records, now)
	costSummaries, costErr := (usage.CostCalculator{Catalog: usage.DeepSeekPricingCatalog()}).SummarizeAt(records, now)
	periods := []struct {
		name   string
		tokens usage.Summary
		costs  usage.CostSummary
	}{
		{"Today", tokenSummaries.Today, costSummaries.Today},
		{"Last 7 days", tokenSummaries.Last7Days, costSummaries.Last7Days},
		{"Current month", tokenSummaries.CurrentMonth, costSummaries.CurrentMonth},
		{"Historical", tokenSummaries.Historical, costSummaries.Historical},
	}

	var output strings.Builder
	output.WriteString("API Usage\n")
	for _, period := range periods {
		output.WriteString("\n" + period.name + "\n")
		writeUsagePeriod(&output, period.tokens, period.costs, costErr == nil)
	}
	if costErr != nil {
		output.WriteString("\nCost unavailable: ")
		output.WriteString(costErr.Error())
		output.WriteByte('\n')
	}
	return strings.TrimRight(output.String(), "\n")
}

func writeUsagePeriod(output *strings.Builder, tokens usage.Summary, costs usage.CostSummary, costAvailable bool) {
	fmt.Fprintf(output, "%-16s%s\n", "Requests", formatCount(tokens.Requests))
	fmt.Fprintf(output, "%-16s%s\n", "Tokens", formatCount(tokens.TotalTokens))
	fmt.Fprintf(output, "%-16s%s\n", "Prompt", formatCount(tokens.PromptTokens))
	fmt.Fprintf(output, "%-16s%s\n", "Completion", formatCount(tokens.CompletionTokens))
	fmt.Fprintf(output, "%-16s%s\n", "Cache hit", formatCount(tokens.PromptCacheHitTokens))
	fmt.Fprintf(output, "%-16s%s\n", "Cache miss", formatCount(tokens.PromptCacheMissTokens))
	if costAvailable {
		fmt.Fprintf(output, "%-16s%s\n", "Cost", formatUSD(costs.Cost.TotalUSD))
	} else {
		fmt.Fprintf(output, "%-16s%s\n", "Cost", unavailableCost)
	}
}

func formatCount(value int) string {
	digits := strconv.Itoa(value)
	start := 0
	if strings.HasPrefix(digits, "-") {
		start = 1
	}
	for index := len(digits) - 3; index > start; index -= 3 {
		digits = digits[:index] + "," + digits[index:]
	}
	return digits
}

func formatUSD(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', 9, 64)
	decimal := strings.IndexByte(formatted, '.')
	for len(formatted)-decimal-1 > 6 && strings.HasSuffix(formatted, "0") {
		formatted = strings.TrimSuffix(formatted, "0")
	}
	return "$" + formatted
}
