package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/usage"
)

const (
	unavailableCost        = "Unavailable"
	statsTwoColumnMinWidth = 72
	statsColumnGap         = 4
)

type statsPeriod struct {
	name   string
	tokens usage.Summary
	cost   usage.CostSummary
}

type statsViewData struct {
	periods       []statsPeriod
	costAvailable bool
	costError     error
	empty         bool
}

func buildStatsViewData(records []chat.UsageRecord, now time.Time) statsViewData {
	tokenSummaries := usage.SummarizeAt(records, now)
	costSummaries, costErr := (usage.CostCalculator{Catalog: usage.DeepSeekPricingCatalog()}).SummarizeAt(records, now)
	return statsViewData{
		periods: []statsPeriod{
			{"TODAY", tokenSummaries.Today, costSummaries.Today},
			{"LAST 7 DAYS", tokenSummaries.Last7Days, costSummaries.Last7Days},
			{"CURRENT MONTH", tokenSummaries.CurrentMonth, costSummaries.CurrentMonth},
			{"HISTORICAL", tokenSummaries.Historical, costSummaries.Historical},
		},
		costAvailable: costErr == nil,
		costError:     costErr,
		empty:         len(records) == 0,
	}
}

func (m Model) statsView() string {
	width := m.columnLayout().contentWidth
	data := buildStatsViewData(m.usageRecords, time.Now())
	var body strings.Builder
	body.WriteString(logoStyle.Render("◆ API Usage"))
	body.WriteString("\n\n")
	if data.empty {
		body.WriteString(mutedStyle.Render("No API usage recorded yet."))
	} else if width >= statsTwoColumnMinWidth {
		body.WriteString(renderWideStats(data, width))
	} else {
		body.WriteString(renderNarrowStats(data, width))
	}
	if data.costError != nil {
		body.WriteString("\n\n")
		body.WriteString(mutedStyle.Render("Pricing incomplete: costs are unavailable."))
	}
	body.WriteString("\n\n")
	body.WriteString(mutedStyle.Render("Esc volver"))
	column := lipgloss.NewStyle().Width(width).Render(body.String())
	return m.placeColumn(column)
}

func renderNarrowStats(data statsViewData, width int) string {
	blocks := make([]string, 0, len(data.periods))
	for index, period := range data.periods {
		blocks = append(blocks, renderStatsPeriod(period, width, index == 0, data.costAvailable))
	}
	return strings.Join(blocks, "\n\n")
}

func renderWideStats(data statsViewData, width int) string {
	columnWidth := (width - statsColumnGap) / 2
	gap := strings.Repeat(" ", statsColumnGap)
	blocks := make([]string, len(data.periods))
	for index, period := range data.periods {
		block := renderStatsPeriod(period, columnWidth, index == 0, data.costAvailable)
		blocks[index] = lipgloss.NewStyle().Width(columnWidth).Render(block)
	}
	firstRow := lipgloss.JoinHorizontal(lipgloss.Top, blocks[0], gap, blocks[1])
	secondRow := lipgloss.JoinHorizontal(lipgloss.Top, blocks[2], gap, blocks[3])
	return lipgloss.JoinVertical(lipgloss.Left, firstRow, "", secondRow)
}

func renderStatsPeriod(period statsPeriod, width int, details, costAvailable bool) string {
	cost := unavailableCost
	if costAvailable {
		cost = formatUSD(period.cost.Cost.TotalUSD)
	}
	rows := []string{
		lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(period.name),
		statRow("Cost", cost, width, true),
		statRow("Tokens", formatCount(period.tokens.TotalTokens), width, true),
		statRow("Requests", formatCount(period.tokens.Requests), width, true),
	}
	if details {
		rows = append(rows, "",
			statRow("Prompt", formatCount(period.tokens.PromptTokens), width, false),
			statRow("Completion", formatCount(period.tokens.CompletionTokens), width, false),
			statRow("Cache hit", formatCount(period.tokens.PromptCacheHitTokens), width, false),
			statRow("Cache miss", formatCount(period.tokens.PromptCacheMissTokens), width, false),
		)
	}
	return strings.Join(rows, "\n")
}

func statRow(label, value string, width int, primary bool) string {
	labelText := mutedStyle.Render(label)
	valueStyle := lipgloss.NewStyle().Foreground(ink).Bold(primary)
	valueText := valueStyle.Render(value)
	gap := width - lipgloss.Width(labelText) - lipgloss.Width(valueText)
	if gap < 1 {
		return labelText + "\n" + lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(valueText)
	}
	return labelText + strings.Repeat(" ", gap) + valueText
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
	return fmt.Sprintf("$%s", formatted)
}
