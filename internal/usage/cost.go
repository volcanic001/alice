package usage

import (
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

const tokensPerMillion = 1_000_000

// CostBreakdown contains unrounded costs in USD.
type CostBreakdown struct {
	CacheHitInputUSD  float64
	CacheMissInputUSD float64
	OutputUSD         float64
	TotalUSD          float64
}

// CostResult identifies both the applied pricing and the resulting cost.
type CostResult struct {
	Pricing PricingVersion
	Cost    CostBreakdown
}

// CostCalculator performs pricing-independent token arithmetic.
type CostCalculator struct {
	Catalog PricingCatalog
}

func (c CostCalculator) Calculate(record chat.UsageRecord) (CostResult, error) {
	pricing, err := c.Catalog.Select(record.Model, record.RequestedAt)
	if err != nil {
		return CostResult{}, err
	}
	rates := pricing.RatesAt(record.RequestedAt)
	cost := CostBreakdown{
		CacheHitInputUSD:  tokenCost(record.PromptCacheHitTokens, rates.CacheHitInputUSDPerMillion),
		CacheMissInputUSD: tokenCost(record.PromptCacheMissTokens, rates.CacheMissInputUSDPerMillion),
		OutputUSD:         tokenCost(record.CompletionTokens, rates.OutputUSDPerMillion),
	}
	cost.TotalUSD = cost.CacheHitInputUSD + cost.CacheMissInputUSD + cost.OutputUSD
	return CostResult{Pricing: pricing, Cost: cost}, nil
}

func tokenCost(tokens int, usdPerMillion float64) float64 {
	return float64(tokens) * usdPerMillion / tokensPerMillion
}

// CostSummary aggregates request count and unrounded costs for one period.
type CostSummary struct {
	Requests int
	Cost     CostBreakdown
}

// CostSummaries contains the same calendar periods as Summaries.
type CostSummaries struct {
	Today        CostSummary
	Last7Days    CostSummary
	CurrentMonth CostSummary
	Historical   CostSummary
}

func (c CostCalculator) Summarize(records []chat.UsageRecord) (CostSummaries, error) {
	return c.SummarizeAt(records, time.Now())
}

// SummarizeAt reuses the calendar period definitions from summary.go.
func (c CostCalculator) SummarizeAt(records []chat.UsageRecord, now time.Time) (CostSummaries, error) {
	return c.summarizeIn(records, now, time.Local)
}

func (c CostCalculator) summarizeIn(records []chat.UsageRecord, now time.Time, location *time.Location) (CostSummaries, error) {
	periods := calendarPeriods(now, location)
	var summaries CostSummaries
	for _, record := range records {
		result, err := c.Calculate(record)
		if err != nil {
			return CostSummaries{}, err
		}
		addCost(&summaries.Historical, result.Cost)
		timestamp := record.RequestedAt.In(location)
		if periods.today.contains(timestamp) {
			addCost(&summaries.Today, result.Cost)
		}
		if periods.last7Days.contains(timestamp) {
			addCost(&summaries.Last7Days, result.Cost)
		}
		if periods.currentMonth.contains(timestamp) {
			addCost(&summaries.CurrentMonth, result.Cost)
		}
	}
	return summaries, nil
}

func addCost(summary *CostSummary, cost CostBreakdown) {
	summary.Requests++
	summary.Cost.CacheHitInputUSD += cost.CacheHitInputUSD
	summary.Cost.CacheMissInputUSD += cost.CacheMissInputUSD
	summary.Cost.OutputUSD += cost.OutputUSD
	summary.Cost.TotalUSD += cost.TotalUSD
}
