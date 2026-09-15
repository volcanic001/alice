package usage

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

func assertFloat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("valor inesperado: obtuve %.15f, quiero %.15f", got, want)
	}
}

func TestCostCalculatorComponentsAndCombination(t *testing.T) {
	calculator := CostCalculator{Catalog: DeepSeekPricingCatalog()}
	at := time.Date(2026, time.September, 14, 0, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		record chat.UsageRecord
		want   CostBreakdown
	}{
		{"cache hit", chat.UsageRecord{Model: "deepseek-flash", RequestedAt: at, PromptCacheHitTokens: 1_000_000}, CostBreakdown{CacheHitInputUSD: 0.003, TotalUSD: 0.003}},
		{"cache miss", chat.UsageRecord{Model: "deepseek-flash", RequestedAt: at, PromptCacheMissTokens: 1_000_000}, CostBreakdown{CacheMissInputUSD: 0.15, TotalUSD: 0.15}},
		{"output", chat.UsageRecord{Model: "deepseek-flash", RequestedAt: at, CompletionTokens: 1_000_000}, CostBreakdown{OutputUSD: 0.60, TotalUSD: 0.60}},
		{"combinación", chat.UsageRecord{Model: "deepseek-flash", RequestedAt: at, PromptCacheHitTokens: 1_000_000, PromptCacheMissTokens: 2_000_000, CompletionTokens: 3_000_000}, CostBreakdown{CacheHitInputUSD: 0.003, CacheMissInputUSD: 0.30, OutputUSD: 1.80, TotalUSD: 2.103}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := calculator.Calculate(test.record)
			if err != nil {
				t.Fatal(err)
			}
			assertFloat(t, result.Cost.CacheHitInputUSD, test.want.CacheHitInputUSD)
			assertFloat(t, result.Cost.CacheMissInputUSD, test.want.CacheMissInputUSD)
			assertFloat(t, result.Cost.OutputUSD, test.want.OutputUSD)
			assertFloat(t, result.Cost.TotalUSD, test.want.TotalUSD)
		})
	}
}

func TestCostCalculatorUsesBothPeakIntervals(t *testing.T) {
	calculator := CostCalculator{Catalog: DeepSeekPricingCatalog()}
	for _, hour := range []int{1, 6} {
		record := chat.UsageRecord{
			Model: "deepseek-flash", RequestedAt: time.Date(2026, time.September, 14, hour, 0, 0, 0, time.UTC),
			PromptCacheHitTokens: 1_000_000, PromptCacheMissTokens: 1_000_000, CompletionTokens: 1_000_000,
		}
		result, err := calculator.Calculate(record)
		if err != nil {
			t.Fatal(err)
		}
		assertFloat(t, result.Cost.CacheHitInputUSD, 0.006)
		assertFloat(t, result.Cost.CacheMissInputUSD, 0.30)
		assertFloat(t, result.Cost.OutputUSD, 1.20)
		assertFloat(t, result.Cost.TotalUSD, 1.506)
	}
}

func TestCostCalculatorReturnsExplicitPricingErrors(t *testing.T) {
	calculator := CostCalculator{Catalog: DeepSeekPricingCatalog()}
	_, err := calculator.Calculate(chat.UsageRecord{Model: "unknown", RequestedAt: time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)})
	if !errors.Is(err, ErrPricingNotFound) {
		t.Fatalf("error inesperado para modelo desconocido: %v", err)
	}
	_, err = calculator.Calculate(chat.UsageRecord{Model: "deepseek-flash", RequestedAt: time.Date(2026, time.September, 10, 3, 59, 59, 0, time.UTC)})
	if !errors.Is(err, ErrPricingNotFound) {
		t.Fatalf("error inesperado para fecha sin tarifa: %v", err)
	}
}

func TestDeepSeekV4ProPricingChangesAtRedirectBoundary(t *testing.T) {
	boundary := time.Date(2026, time.September, 14, 4, 0, 0, 0, time.UTC)
	calculator := CostCalculator{Catalog: DeepSeekPricingCatalog()}
	record := func(at time.Time) chat.UsageRecord {
		return chat.UsageRecord{
			Model: "deepseek-v4-pro", RequestedAt: at,
			PromptCacheHitTokens: 1_000_000, PromptCacheMissTokens: 1_000_000, CompletionTokens: 1_000_000,
		}
	}

	before, err := calculator.Calculate(record(boundary.Add(-time.Nanosecond)))
	if err != nil {
		t.Fatal(err)
	}
	atBoundary, err := calculator.Calculate(record(boundary))
	if err != nil {
		t.Fatal(err)
	}
	if before.Pricing.EffectiveTo == nil || !before.Pricing.EffectiveTo.Equal(boundary) {
		t.Fatalf("no seleccionó pricing Pro histórico: %#v", before.Pricing)
	}
	if !atBoundary.Pricing.EffectiveFrom.Equal(boundary) || atBoundary.Pricing.EffectiveTo != nil {
		t.Fatalf("no seleccionó pricing Flash redirigido: %#v", atBoundary.Pricing)
	}
	assertFloat(t, before.Cost.CacheHitInputUSD, 0.044)
	assertFloat(t, before.Cost.CacheMissInputUSD, 1.32)
	assertFloat(t, before.Cost.OutputUSD, 3.96)
	assertFloat(t, before.Cost.TotalUSD, 5.324)
	assertFloat(t, atBoundary.Cost.CacheHitInputUSD, 0.003)
	assertFloat(t, atBoundary.Cost.CacheMissInputUSD, 0.15)
	assertFloat(t, atBoundary.Cost.OutputUSD, 0.60)
	assertFloat(t, atBoundary.Cost.TotalUSD, 0.753)
}

func TestHistoricalRequestKeepsItsPricingVersion(t *testing.T) {
	december := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	versionA := PricingVersion{
		Model: "fictional", EffectiveFrom: time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC), EffectiveTo: &december,
		DefaultRates: TokenRates{CacheHitInputUSDPerMillion: 1, CacheMissInputUSDPerMillion: 2, OutputUSDPerMillion: 3},
	}
	versionB := PricingVersion{
		Model: "fictional", EffectiveFrom: december,
		DefaultRates: TokenRates{CacheHitInputUSDPerMillion: 10, CacheMissInputUSDPerMillion: 20, OutputUSDPerMillion: 30},
	}
	calculator := CostCalculator{Catalog: PricingCatalog{Versions: []PricingVersion{versionA, versionB}}}
	oldRecord := chat.UsageRecord{Model: "fictional", RequestedAt: time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC), PromptCacheHitTokens: 1_000_000, PromptCacheMissTokens: 1_000_000, CompletionTokens: 1_000_000}
	newRecord := chat.UsageRecord{Model: "fictional", RequestedAt: time.Date(2026, time.December, 20, 0, 0, 0, 0, time.UTC), PromptCacheHitTokens: 1_000_000, PromptCacheMissTokens: 1_000_000, CompletionTokens: 1_000_000}

	oldResult, err := calculator.Calculate(oldRecord)
	if err != nil {
		t.Fatal(err)
	}
	newResult, err := calculator.Calculate(newRecord)
	if err != nil {
		t.Fatal(err)
	}
	if !oldResult.Pricing.EffectiveFrom.Equal(versionA.EffectiveFrom) || !newResult.Pricing.EffectiveFrom.Equal(versionB.EffectiveFrom) {
		t.Fatalf("versiones incorrectas: antigua=%v nueva=%v", oldResult.Pricing.EffectiveFrom, newResult.Pricing.EffectiveFrom)
	}
	assertFloat(t, oldResult.Cost.TotalUSD, 6)
	assertFloat(t, newResult.Cost.TotalUSD, 60)
}

func aggregateCostRecord(at time.Time, multiplier int) chat.UsageRecord {
	return chat.UsageRecord{
		Model: "aggregate", RequestedAt: at,
		PromptCacheHitTokens: multiplier * 1_000_000, PromptCacheMissTokens: multiplier * 2_000_000, CompletionTokens: multiplier * 3_000_000,
	}
}

func assertCostSummary(t *testing.T, got CostSummary, requests int, hit, miss, output, total float64) {
	t.Helper()
	if got.Requests != requests {
		t.Fatalf("requests inesperadas: obtuve %d, quiero %d", got.Requests, requests)
	}
	assertFloat(t, got.Cost.CacheHitInputUSD, hit)
	assertFloat(t, got.Cost.CacheMissInputUSD, miss)
	assertFloat(t, got.Cost.OutputUSD, output)
	assertFloat(t, got.Cost.TotalUSD, total)
}

func TestCostAggregationReusesCalendarPeriods(t *testing.T) {
	location := time.FixedZone("UTC-3", -3*60*60)
	now := time.Date(2026, time.March, 15, 12, 0, 0, 0, location)
	pricing := PricingVersion{
		Model: "aggregate", EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		DefaultRates: TokenRates{CacheHitInputUSDPerMillion: 1, CacheMissInputUSDPerMillion: 1, OutputUSDPerMillion: 1},
	}
	calculator := CostCalculator{Catalog: PricingCatalog{Versions: []PricingVersion{pricing}}}
	records := []chat.UsageRecord{
		aggregateCostRecord(time.Date(2026, time.March, 15, 8, 0, 0, 0, location), 1),
		aggregateCostRecord(time.Date(2026, time.March, 15, 9, 0, 0, 0, location), 2),
		aggregateCostRecord(time.Date(2026, time.March, 14, 23, 0, 0, 0, location), 3),
		aggregateCostRecord(time.Date(2026, time.March, 9, 0, 0, 0, 0, location), 4),
		aggregateCostRecord(time.Date(2026, time.March, 8, 23, 59, 0, 0, location), 5),
		aggregateCostRecord(time.Date(2026, time.February, 28, 23, 59, 0, 0, location), 6),
	}
	summaries, err := calculator.summarizeIn(records, now, location)
	if err != nil {
		t.Fatal(err)
	}
	assertCostSummary(t, summaries.Today, 2, 3, 6, 9, 18)
	assertCostSummary(t, summaries.Last7Days, 4, 10, 20, 30, 60)
	assertCostSummary(t, summaries.CurrentMonth, 5, 15, 30, 45, 90)
	assertCostSummary(t, summaries.Historical, 6, 21, 42, 63, 126)
}
