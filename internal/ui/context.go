package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/volcanic001/alice/internal/provider"
)

const (
	maxContextSegments     = 14
	contextPercentMaxWidth = 5
)

var (
	contextEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B3742"))
	contextGradient   = [][3]uint8{
		{0xA8, 0x3F, 0x60},
		{0xFF, 0x7A, 0x90},
		{0xFF, 0xB0, 0xBE},
	}
)

func contextWindow(model string) (int, bool) {
	switch model {
	case provider.DeepSeekFlashModel:
		return provider.DeepSeekFlashContextWindow, true
	default:
		return 0, false
	}
}

func contextPercent(totalTokens, window int) (float64, bool) {
	if totalTokens < 0 || window <= 0 {
		return 0, false
	}
	percent := float64(totalTokens) / float64(window) * 100
	return math.Min(percent, 100), true
}

func formatContextPercent(percent float64) string {
	if percent < 1 {
		return fmt.Sprintf("%.2f%%", percent)
	}
	if percent < 10 {
		return fmt.Sprintf("%.1f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

func contextFilledSegments(percent float64, segments int) int {
	if segments <= 0 {
		return 0
	}
	filled := int(math.Floor(percent / 100 * float64(segments)))
	return max(0, min(segments, filled))
}

func contextPointStyle(index, count int) lipgloss.Style {
	if count <= 1 {
		return lipgloss.NewStyle().Foreground(coral)
	}
	position := float64(index) / float64(count-1)
	scaled := position * float64(len(contextGradient)-1)
	left := min(int(math.Floor(scaled)), len(contextGradient)-2)
	fraction := scaled - float64(left)
	color := interpolateContextColor(contextGradient[left], contextGradient[left+1], fraction)
	return lipgloss.NewStyle().Foreground(color)
}

func interpolateContextColor(from, to [3]uint8, fraction float64) lipgloss.Color {
	channel := func(index int) uint8 {
		value := float64(from[index]) + (float64(to[index])-float64(from[index]))*fraction
		return uint8(math.Round(value))
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", channel(0), channel(1), channel(2)))
}

func contextMeter(percent float64, known bool, segments int) string {
	if segments <= 0 {
		return ""
	}
	filled := 0
	if known {
		filled = contextFilledSegments(percent, segments)
	}
	points := make([]string, 0, segments)
	for index := 0; index < segments; index++ {
		if index < filled {
			points = append(points, contextPointStyle(index, filled).Render("●"))
		} else {
			points = append(points, contextEmptyStyle.Render("○"))
		}
	}
	return strings.Join(points, "")
}

func (m Model) confirmedContextPercent() (float64, bool) {
	for index := len(m.usageRecords) - 1; index >= 0; index-- {
		record := m.usageRecords[index]
		if record.ConversationID != m.conversation.ID {
			continue
		}
		window, ok := contextWindow(record.Model)
		if !ok {
			return 0, false
		}
		return contextPercent(record.TotalTokens, window)
	}
	return 0, false
}

func (m Model) chatHeader(width int) string {
	if width <= 0 {
		return ""
	}
	logo := logoStyle.Render("◆ ALICE")
	percent, known := m.confirmedContextPercent()
	percentText := "--%"
	if known {
		percentText = formatContextPercent(percent)
	}
	percentRendered := mutedStyle.Render(percentText)

	const (
		groupGap   = "  "
		minimumGap = 2
	)
	logoWidth := lipgloss.Width(logo)
	percentWidth := lipgloss.Width(percentText)
	labelText := "Context"
	fullGroupWidth := lipgloss.Width(labelText) + 2*lipgloss.Width(groupGap) + maxContextSegments + contextPercentMaxWidth
	if logoWidth+minimumGap+fullGroupWidth > width {
		labelText = "CTX"
	}
	label := mutedStyle.Render(labelText)
	fixedWidth := logoWidth + minimumGap + lipgloss.Width(label) + 2*lipgloss.Width(groupGap) + percentWidth
	segments := min(maxContextSegments, max(0, width-fixedWidth))

	if segments > 0 {
		right := label + groupGap + contextMeter(percent, known, segments) + groupGap + percentRendered
		return alignHeaderGroups(logo, right, width)
	}

	right := label + groupGap + percentRendered
	if logoWidth+minimumGap+lipgloss.Width(right) <= width {
		return alignHeaderGroups(logo, right, width)
	}
	if logoWidth+1+percentWidth <= width {
		return alignHeaderGroups(logo, percentRendered, width)
	}
	if percentWidth <= width {
		return strings.Repeat(" ", width-percentWidth) + percentRendered
	}
	return ""
}

func alignHeaderGroups(left, right string, width int) string {
	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}
