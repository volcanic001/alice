package usage

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrPricingNotFound  = errors.New("pricing no encontrado")
	ErrAmbiguousPricing = errors.New("pricing ambiguo")
)

// TokenRates contains USD prices per one million tokens.
type TokenRates struct {
	CacheHitInputUSDPerMillion  float64
	CacheMissInputUSDPerMillion float64
	OutputUSDPerMillion         float64
}

// UTCInterval is a half-open time-of-day interval in UTC.
type UTCInterval struct {
	Start time.Duration
	End   time.Duration
}

// PricingRule overrides the default rates on selected UTC weekdays and intervals.
type PricingRule struct {
	Weekdays  []time.Weekday
	Intervals []UTCInterval
	Rates     TokenRates
}

// PricingVersion describes one immutable set of rates and temporal rules.
// EffectiveTo is exclusive; nil means that the version is current.
type PricingVersion struct {
	Model         string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	DefaultRates  TokenRates
	Rules         []PricingRule
}

// RatesAt resolves temporal rules without exposing them to CostCalculator.
func (p PricingVersion) RatesAt(timestamp time.Time) TokenRates {
	for _, rule := range p.Rules {
		if rule.matches(timestamp) {
			return rule.Rates
		}
	}
	return p.DefaultRates
}

func (r PricingRule) matches(timestamp time.Time) bool {
	utc := timestamp.UTC()
	if !containsWeekday(r.Weekdays, utc.Weekday()) {
		return false
	}
	sinceMidnight := time.Duration(utc.Hour())*time.Hour +
		time.Duration(utc.Minute())*time.Minute +
		time.Duration(utc.Second())*time.Second +
		time.Duration(utc.Nanosecond())
	for _, interval := range r.Intervals {
		if sinceMidnight >= interval.Start && sinceMidnight < interval.End {
			return true
		}
	}
	return false
}

func containsWeekday(weekdays []time.Weekday, weekday time.Weekday) bool {
	for _, candidate := range weekdays {
		if candidate == weekday {
			return true
		}
	}
	return false
}

// PricingCatalog keeps every historical pricing version.
type PricingCatalog struct {
	Versions []PricingVersion
}

// Select returns the single version valid for model at timestamp.
func (c PricingCatalog) Select(model string, timestamp time.Time) (PricingVersion, error) {
	var selected *PricingVersion
	for i := range c.Versions {
		version := &c.Versions[i]
		if version.Model != model || timestamp.Before(version.EffectiveFrom) ||
			(version.EffectiveTo != nil && !timestamp.Before(*version.EffectiveTo)) {
			continue
		}
		if selected != nil {
			return PricingVersion{}, fmt.Errorf("%w para modelo %q en %s", ErrAmbiguousPricing, model, timestamp.Format(time.RFC3339))
		}
		selected = version
	}
	if selected == nil {
		return PricingVersion{}, fmt.Errorf("%w para modelo %q en %s", ErrPricingNotFound, model, timestamp.Format(time.RFC3339))
	}
	return *selected, nil
}

// DeepSeekPricingCatalog returns Alice built-in, append-only pricing history.
func DeepSeekPricingCatalog() PricingCatalog {
	weekdays := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	intervals := []UTCInterval{
		{Start: time.Hour, End: 4 * time.Hour},
		{Start: 6 * time.Hour, End: 10 * time.Hour},
	}
	flashEffectiveFrom := time.Date(2026, time.September, 10, 4, 0, 0, 0, time.UTC)
	proRedirectFrom := time.Date(2026, time.September, 14, 4, 0, 0, 0, time.UTC)
	flashOffPeak := TokenRates{
		CacheHitInputUSDPerMillion:  0.003,
		CacheMissInputUSDPerMillion: 0.15,
		OutputUSDPerMillion:         0.60,
	}
	flashPeak := TokenRates{
		CacheHitInputUSDPerMillion:  0.006,
		CacheMissInputUSDPerMillion: 0.30,
		OutputUSDPerMillion:         1.20,
	}
	proOffPeak := TokenRates{
		CacheHitInputUSDPerMillion:  0.022,
		CacheMissInputUSDPerMillion: 0.66,
		OutputUSDPerMillion:         1.98,
	}
	proPeak := TokenRates{
		CacheHitInputUSDPerMillion:  0.044,
		CacheMissInputUSDPerMillion: 1.32,
		OutputUSDPerMillion:         3.96,
	}
	rules := func(rates TokenRates) []PricingRule {
		return []PricingRule{{Weekdays: weekdays, Intervals: intervals, Rates: rates}}
	}
	return PricingCatalog{Versions: []PricingVersion{
		{
			Model:         "deepseek-flash",
			EffectiveFrom: flashEffectiveFrom,
			DefaultRates:  flashOffPeak,
			Rules:         rules(flashPeak),
		},
		{
			Model:         "deepseek-v4-pro",
			EffectiveFrom: time.Time{},
			EffectiveTo:   &proRedirectFrom,
			DefaultRates:  proOffPeak,
			Rules:         rules(proPeak),
		},
		{
			Model:         "deepseek-v4-pro",
			EffectiveFrom: proRedirectFrom,
			DefaultRates:  flashOffPeak,
			Rules:         rules(flashPeak),
		},
	}}
}
