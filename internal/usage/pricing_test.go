package usage

import (
	"errors"
	"testing"
	"time"
)

func assertRates(t *testing.T, got TokenRates, hit, miss, output float64) {
	t.Helper()
	want := TokenRates{
		CacheHitInputUSDPerMillion: hit, CacheMissInputUSDPerMillion: miss, OutputUSDPerMillion: output,
	}
	if got != want {
		t.Fatalf("tarifas inesperadas: obtuve %#v, quiero %#v", got, want)
	}
}

func TestDeepSeekFlashPeakScheduleAndBoundaries(t *testing.T) {
	pricing, err := DeepSeekPricingCatalog().Select("deepseek-flash", time.Date(2026, time.September, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		at     time.Time
		isPeak bool
	}{
		{"antes del primer intervalo", time.Date(2026, time.September, 14, 0, 59, 59, 0, time.UTC), false},
		{"inicio del primer intervalo", time.Date(2026, time.September, 14, 1, 0, 0, 0, time.UTC), true},
		{"dentro del primer intervalo", time.Date(2026, time.September, 14, 3, 59, 59, 0, time.UTC), true},
		{"fin exclusivo del primer intervalo", time.Date(2026, time.September, 14, 4, 0, 0, 0, time.UTC), false},
		{"inicio del segundo intervalo", time.Date(2026, time.September, 14, 6, 0, 0, 0, time.UTC), true},
		{"dentro del segundo intervalo", time.Date(2026, time.September, 14, 9, 59, 59, 0, time.UTC), true},
		{"fin exclusivo del segundo intervalo", time.Date(2026, time.September, 14, 10, 0, 0, 0, time.UTC), false},
		{"fin de semana", time.Date(2026, time.September, 19, 2, 0, 0, 0, time.UTC), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rates := pricing.RatesAt(test.at)
			if test.isPeak {
				assertRates(t, rates, 0.006, 0.30, 1.20)
			} else {
				assertRates(t, rates, 0.003, 0.15, 0.60)
			}
		})
	}
}

func TestDeepSeekCatalogVersionBoundaries(t *testing.T) {
	catalog := DeepSeekPricingCatalog()
	flashStart := time.Date(2026, time.September, 10, 4, 0, 0, 0, time.UTC)
	flash, err := catalog.Select("deepseek-flash", flashStart)
	if err != nil {
		t.Fatal(err)
	}
	if !flash.EffectiveFrom.Equal(flashStart) {
		t.Fatalf("inicio de Flash inesperado: %v", flash.EffectiveFrom)
	}
	if _, err := catalog.Select("deepseek-flash", flashStart.Add(-time.Nanosecond)); !errors.Is(err, ErrPricingNotFound) {
		t.Fatalf("Flash se aplicó antes de su vigencia: %v", err)
	}

	proBoundary := time.Date(2026, time.September, 14, 4, 0, 0, 0, time.UTC)
	proBefore, err := catalog.Select("deepseek-v4-pro", proBoundary.Add(-time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	proAfter, err := catalog.Select("deepseek-v4-pro", proBoundary)
	if err != nil {
		t.Fatal(err)
	}
	assertRates(t, proBefore.DefaultRates, 0.022, 0.66, 1.98)
	assertRates(t, proAfter.DefaultRates, 0.003, 0.15, 0.60)
}

func TestPricingSelectionRejectsUnknownModelAndVersionGap(t *testing.T) {
	catalog := DeepSeekPricingCatalog()
	if _, err := catalog.Select("deepseek-unknown", time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)); !errors.Is(err, ErrPricingNotFound) {
		t.Fatalf("modelo desconocido: %v", err)
	}
	if _, err := catalog.Select("deepseek-flash", time.Date(2026, time.September, 10, 3, 59, 59, 0, time.UTC)); !errors.Is(err, ErrPricingNotFound) {
		t.Fatalf("fecha sin pricing: %v", err)
	}

	february := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	march := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	gapCatalog := PricingCatalog{Versions: []PricingVersion{
		{Model: "model", EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: &february},
		{Model: "model", EffectiveFrom: march},
	}}
	if _, err := gapCatalog.Select("model", time.Date(2026, time.February, 15, 0, 0, 0, 0, time.UTC)); !errors.Is(err, ErrPricingNotFound) {
		t.Fatalf("hueco entre versiones: %v", err)
	}
}
