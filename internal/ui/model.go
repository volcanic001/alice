package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/volcanic001/alice/internal/chat"
	"github.com/volcanic001/alice/internal/store"
)

var (
	coral      = lipgloss.Color("#FF7A90")
	lavender   = lipgloss.Color("#A995FF")
	ink        = lipgloss.Color("#E8E6F0")
	muted      = lipgloss.Color("#777184")
	surface    = lipgloss.Color("#25222C")
	userStyle  = lipgloss.NewStyle().Foreground(ink).Background(lipgloss.Color("#34303D")).Padding(0, 1).MarginTop(1)
	aliceStyle = lipgloss.NewStyle().Foreground(ink).BorderLeft(true).BorderForeground(lavender).PaddingLeft(1).MarginTop(1)
	logoStyle  = lipgloss.NewStyle().Bold(true).Foreground(coral)
	mutedStyle = lipgloss.NewStyle().Foreground(muted)
)

type streamMsg chat.Event
type errMsg struct{ err error }

type screen int

const (
	chatScreen screen = iota
	historyScreen
)

const (
	horizontalMargin = 2
	maxColumnWidth   = 84
	helpText         = "Enter enviar · Shift+Enter salto · Ctrl+N nuevo · Ctrl+H historial · PgUp/PgDn páginas · Inicio/Fin · Esc cancelar"
)

type columnLayout struct {
	contentWidth int
	leftMargin   int
	rightMargin  int
}
type Model struct {
	store         *store.Store
	provider      chat.Provider
	conversations []chat.Conversation
	conversation  chat.Conversation
	messages      []chat.Message
	viewport      viewport.Model
	input         textarea.Model
	width, height int
	stream        <-chan chat.Event
	cancel        context.CancelFunc
	draft         string
	busy          bool
	status        string
	markdown      *glamour.TermRenderer
	markdownWidth int
	markdownCache map[string]string
	pages         []string
	currentPage   int
	totalPages    int
	screen        screen
	historyIndex  int
	deleteConfirm bool
	renameActive  bool
	renameInput   textarea.Model
	searchActive  bool
	searchInput   textarea.Model
}

func New(database *store.Store, provider chat.Provider) (Model, error) {
	conversations, err := database.Conversations()
	if err != nil {
		return Model{}, err
	}
	if len(conversations) == 0 {
		conversation, err := database.CreateConversation()
		if err != nil {
			return Model{}, err
		}
		conversations = []chat.Conversation{conversation}
	}
	messages, err := database.Messages(conversations[0].ID)
	if err != nil {
		return Model{}, err
	}
	input := textarea.New()
	input.Placeholder = "Escribe un mensaje…"
	input.Prompt = "› "
	input.CharLimit = 16000
	input.SetHeight(1)
	input.ShowLineNumbers = false
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(coral).Bold(true)
	input.KeyMap.InsertNewline.SetKeys("shift+enter", "ctrl+j")
	input.Focus()
	view := viewport.New(1, 1)
	model := Model{store: database, provider: provider, conversations: conversations, conversation: conversations[0], messages: messages, viewport: view, input: input, markdownCache: make(map[string]string)}
	model.refresh()
	return model, nil
}

func (m Model) Init() tea.Cmd { return textarea.Blink }

func waitForEvent(events <-chan chat.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return streamMsg(chat.Event{Done: true})
		}
		return streamMsg(event)
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
		m.resize()
	case tea.KeyMsg:
		key := message.String()
		if key == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if m.screen == historyScreen && m.renameActive {
			return m.updateRename(message, key)
		}
		if m.screen == historyScreen && m.searchActive {
			return m.updateHistorySearch(message, key)
		}
		if m.screen == historyScreen && m.deleteConfirm {
			return m.updateHistory(key)
		}
		if key == "ctrl+n" {
			if !m.busy {
				return m.newConversation()
			}
			return m, nil
		}
		if m.screen == historyScreen {
			return m.updateHistory(key)
		}
		switch key {
		case "esc":
			if m.busy && m.cancel != nil {
				m.cancel()
				m.busy = false
				m.status = "Respuesta cancelada"
				return m, nil
			}
		case "ctrl+h":
			m.screen = historyScreen
			m.historyIndex = m.currentConversationIndex()
			return m, nil
		case "enter":
			if !m.busy && strings.TrimSpace(m.input.Value()) != "" {
				return m.send()
			}
		case "pgup":
			m.goToPage(m.currentPage - 1)
			return m, nil
		case "pgdown":
			m.goToPage(m.currentPage + 1)
			return m, nil
		case "home":
			m.goToPage(0)
			return m, nil
		case "end":
			m.goToPage(m.totalPages - 1)
			return m, nil
		case "alt+up":
			return m.openRelative(-1)
		case "alt+down":
			return m.openRelative(1)
		}
	case streamMsg:
		event := chat.Event(message)
		if event.Err != nil {
			m.busy = false
			m.cancel = nil
			if errors.Is(event.Err, context.Canceled) {
				m.status = "Respuesta cancelada"
			} else {
				m.status = event.Err.Error()
			}
			return m, nil
		}
		if event.Text != "" {
			m.draft += event.Text
			m.refresh()
			m.goToPage(m.totalPages - 1)
		}
		if event.Done {
			m.busy = false
			m.cancel = nil
			if m.draft != "" {
				if err := m.store.AddMessage(m.conversation.ID, "assistant", m.draft); err != nil {
					m.status = err.Error()
				}
				m.messages = append(m.messages, chat.Message{ConversationID: m.conversation.ID, Role: "assistant", Content: m.draft})
			}
			m.draft = ""
			m.status = "Listo"
			m.refresh()
			return m, nil
		}
		return m, waitForEvent(m.stream)
	case errMsg:
		m.status = message.err.Error()
	}

	var command tea.Cmd
	if !m.busy {
		inputHeight := m.input.Height()
		m.input, command = m.input.Update(message)
		commands = append(commands, command)
		if m.input.Height() != inputHeight {
			m.resize()
		}
	}
	return m, tea.Batch(commands...)
}

func (m Model) send() (tea.Model, tea.Cmd) {
	content := strings.TrimSpace(m.input.Value())
	if err := m.store.AddMessage(m.conversation.ID, "user", content); err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.messages = append(m.messages, chat.Message{ConversationID: m.conversation.ID, Role: "user", Content: content})
	m.input.Reset()
	m.busy = true
	m.status = "Alice está pensando…"
	m.draft = ""
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.stream = m.provider.Stream(ctx, chat.Request{Model: "deepseek-chat", Messages: m.messages, Temperature: .7})
	m.refresh()
	m.goToPage(m.totalPages - 1)
	return m, waitForEvent(m.stream)
}

func (m Model) openRelative(delta int) (tea.Model, tea.Cmd) {
	return m.openConversation(m.currentConversationIndex() + delta)
}

func (m Model) currentConversationIndex() int {
	for i, conversation := range m.conversations {
		if conversation.ID == m.conversation.ID {
			return i
		}
	}
	return 0
}

func (m Model) openConversation(index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(m.conversations) {
		return m, nil
	}
	m.conversation = m.conversations[index]
	messages, err := m.store.Messages(m.conversation.ID)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.messages = messages
	m.screen = chatScreen
	if m.searchActive {
		m.searchActive = false
		m.searchInput.Reset()
	}
	m.refresh()
	m.goToPage(m.totalPages - 1)
	return m, m.input.Focus()
}

func (m Model) newConversation() (tea.Model, tea.Cmd) {
	conversation, err := m.store.CreateConversation()
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.conversations = append([]chat.Conversation{conversation}, m.conversations...)
	m.conversation, m.messages = conversation, nil
	m.screen = chatScreen
	m.refresh()
	return m, m.input.Focus()
}

func (m Model) updateHistory(key string) (tea.Model, tea.Cmd) {
	if m.deleteConfirm {
		switch key {
		case "y", "Y":
			return m.deleteSelectedConversation()
		case "n", "N", "esc":
			m.deleteConfirm = false
		}
		return m, nil
	}

	switch key {
	case "ctrl+h", "esc":
		m.screen = chatScreen
		return m, m.input.Focus()
	case "/":
		m.searchInput = newHistorySearchInput(m.viewport.Width)
		m.searchActive = true
		m.historyIndex = 0
		return m, m.searchInput.Focus()
	case "d", "delete", "backspace":
		if m.selectedHistoryConversationIndex() >= 0 {
			m.deleteConfirm = true
		}
	case "r":
		if index := m.selectedHistoryConversationIndex(); index >= 0 {
			m.renameInput = newRenameInput(m.conversations[index].Title, m.viewport.Width)
			m.renameActive = true
			return m, m.renameInput.Focus()
		}
	case "up":
		if m.historyIndex > 0 {
			m.historyIndex--
		}
	case "down":
		if m.historyIndex < len(m.historyResults())-1 {
			m.historyIndex++
		}
	case "enter":
		if index := m.selectedHistoryConversationIndex(); index >= 0 {
			return m.openConversation(index)
		}
	}
	return m, nil
}

func newHistorySearchInput(width int) textarea.Model {
	input := textarea.New()
	input.Prompt = "> "
	input.SetHeight(1)
	input.SetWidth(max(1, width))
	input.ShowLineNumbers = false
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(coral).Bold(true)
	return input
}

// historyResults derives the visible list from the conversations already held
// by the UI. It deliberately does not read from storage.
func (m Model) historyResults() []int {
	query := strings.ToLower(strings.TrimSpace(m.searchInput.Value()))
	results := make([]int, 0, len(m.conversations))
	for index, conversation := range m.conversations {
		if !m.searchActive || strings.Contains(strings.ToLower(conversation.Title), query) {
			results = append(results, index)
		}
	}
	return results
}

func (m Model) selectedHistoryConversationIndex() int {
	results := m.historyResults()
	if m.historyIndex < 0 || m.historyIndex >= len(results) {
		return -1
	}
	return results[m.historyIndex]
}

func (m *Model) normalizeHistorySelection() {
	results := m.historyResults()
	if len(results) == 0 {
		m.historyIndex = 0
		return
	}
	m.historyIndex = min(max(0, m.historyIndex), len(results)-1)
}

func (m Model) updateHistorySearch(message tea.Msg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.searchActive = false
		m.searchInput.Reset()
		m.historyIndex = m.currentConversationIndex()
		m.normalizeHistorySelection()
		return m, nil
	case "up":
		if m.historyIndex > 0 {
			m.historyIndex--
		}
		return m, nil
	case "down":
		if m.historyIndex < len(m.historyResults())-1 {
			m.historyIndex++
		}
		return m, nil
	case "enter":
		if index := m.selectedHistoryConversationIndex(); index >= 0 {
			return m.openConversation(index)
		}
		return m, nil
	}

	selectedID := int64(0)
	if index := m.selectedHistoryConversationIndex(); index >= 0 {
		selectedID = m.conversations[index].ID
	}
	var command tea.Cmd
	m.searchInput, command = m.searchInput.Update(message)
	if selectedID != 0 {
		for resultIndex, conversationIndex := range m.historyResults() {
			if m.conversations[conversationIndex].ID == selectedID {
				m.historyIndex = resultIndex
				return m, command
			}
		}
	}
	m.normalizeHistorySelection()
	return m, command
}

func newRenameInput(title string, width int) textarea.Model {
	input := textarea.New()
	input.Prompt = "> "
	input.SetValue(title)
	input.CursorEnd()
	input.SetHeight(1)
	input.SetWidth(max(1, width))
	input.ShowLineNumbers = false
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(coral).Bold(true)
	return input
}

func (m Model) updateRename(message tea.Msg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.renameActive = false
		return m, nil
	case "enter":
		title := strings.TrimSpace(m.renameInput.Value())
		m.renameActive = false
		if title == "" {
			m.status = "El título no puede estar vacío"
			return m, nil
		}
		index := m.selectedHistoryConversationIndex()
		if index < 0 {
			return m, nil
		}
		selected := &m.conversations[index]
		if err := m.store.RenameConversation(selected.ID, title); err != nil {
			m.status = err.Error()
			return m, nil
		}
		selected.Title = title
		if selected.ID == m.conversation.ID {
			m.conversation.Title = title
		}
		m.normalizeHistorySelection()
		return m, nil
	}
	var command tea.Cmd
	m.renameInput, command = m.renameInput.Update(message)
	return m, command
}

func (m Model) deleteSelectedConversation() (tea.Model, tea.Cmd) {
	selectedIndex := m.selectedHistoryConversationIndex()
	if selectedIndex < 0 {
		m.deleteConfirm = false
		return m, nil
	}

	deleted := m.conversations[selectedIndex]
	if err := m.store.DeleteConversation(deleted.ID); err != nil {
		m.deleteConfirm = false
		m.status = err.Error()
		return m, nil
	}

	m.conversations = append(m.conversations[:selectedIndex], m.conversations[selectedIndex+1:]...)
	m.normalizeHistorySelection()
	m.deleteConfirm = false

	if deleted.ID != m.conversation.ID {
		return m, nil
	}
	if len(m.conversations) > 0 {
		selectedIndex = m.selectedHistoryConversationIndex()
		if selectedIndex < 0 {
			selectedIndex = 0
		}
		m.conversation = m.conversations[selectedIndex]
		messages, err := m.store.Messages(m.conversation.ID)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.messages = messages
		m.refresh()
		return m, nil
	}

	conversation, err := m.store.CreateConversation()
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.conversations = []chat.Conversation{conversation}
	m.conversation, m.messages = conversation, nil
	m.historyIndex = 0
	m.normalizeHistorySelection()
	m.refresh()
	return m, nil
}

func (m *Model) resize() {
	layout := m.columnLayout()
	m.viewport.Width = layout.contentWidth
	m.input.SetWidth(layout.contentWidth)
	if m.renameActive {
		m.renameInput.SetWidth(layout.contentWidth)
	}
	if m.searchActive {
		m.searchInput.SetWidth(layout.contentWidth)
	}
	m.viewport.Height = m.conversationHeight()
	m.refresh()
}

func (m Model) columnLayout() columnLayout {
	contentWidth := min(m.width-horizontalMargin*2, maxColumnWidth)
	if contentWidth < 1 {
		contentWidth = 1
	}
	remaining := m.width - contentWidth
	if remaining < 0 {
		remaining = 0
	}
	leftMargin := remaining / 2
	return columnLayout{
		contentWidth: contentWidth,
		leftMargin:   leftMargin,
		rightMargin:  remaining - leftMargin,
	}
}

func (m Model) placeColumn(column string) string {
	indent := strings.Repeat(" ", m.columnLayout().leftMargin)
	return indent + strings.ReplaceAll(column, "\n", "\n"+indent)
}

func (m Model) conversationHeight() int {
	header := logoStyle.Render("◆ ALICE") + "  " + mutedStyle.Render(m.conversation.Title)
	page := mutedStyle.Render(fmt.Sprintf("pág %d/%d", m.currentPage+1, m.totalPages))
	status := m.status + "  " + page
	headerLines := lipgloss.Height(lipgloss.NewStyle().Width(m.viewport.Width).Render(header))
	statusLines := lipgloss.Height(lipgloss.NewStyle().Width(m.viewport.Width).Render(status))
	inputLines := lipgloss.Height(lipgloss.NewStyle().Width(m.viewport.Width).BorderTop(true).Render(m.input.View()))
	helpLines := lipgloss.Height(lipgloss.NewStyle().Width(m.viewport.Width).Render(helpText))
	return max(4, m.height-headerLines-statusLines-inputLines-helpLines)
}

func (m *Model) refresh() {
	messageWidth := max(10, m.viewport.Width)
	markdownWidth := max(10, messageWidth-2)
	var body strings.Builder
	if len(m.messages) == 0 && m.draft == "" {
		body.WriteString("\n" + logoStyle.Render("Hola, soy Alice.") + "\n" + mutedStyle.Render("¿En qué puedo ayudarte hoy?"))
	}
	for _, message := range m.messages {
		if message.Role == "user" {
			body.WriteString(userStyle.Width(messageWidth).Render("Tú\n" + message.Content))
		} else {
			body.WriteString(aliceStyle.Width(messageWidth).Render("Alice\n" + m.renderMarkdown(message.Content, markdownWidth)))
		}
		body.WriteString("\n")
	}
	if m.draft != "" {
		body.WriteString(aliceStyle.Width(messageWidth).Render("Alice\n" + m.draft + " ▌"))
	}
	m.setPages(body.String())
}

func (m *Model) setPages(content string) {
	lines := strings.Split(content, "\n")
	linesPerPage := max(1, m.viewport.Height)
	m.pages = make([]string, 0, (len(lines)+linesPerPage-1)/linesPerPage)
	for start := 0; start < len(lines); start += linesPerPage {
		end := min(start+linesPerPage, len(lines))
		m.pages = append(m.pages, strings.Join(lines[start:end], "\n"))
	}
	if len(m.pages) == 0 {
		m.pages = []string{""}
	}
	m.totalPages = len(m.pages)
	if m.currentPage >= m.totalPages {
		m.currentPage = m.totalPages - 1
	}
	if m.currentPage < 0 {
		m.currentPage = 0
	}
	m.showCurrentPage()
}

func (m *Model) goToPage(page int) {
	if page < 0 || page >= m.totalPages || page == m.currentPage {
		return
	}
	m.currentPage = page
	m.showCurrentPage()
}

func (m *Model) showCurrentPage() {
	if m.totalPages == 0 {
		return
	}
	m.viewport.SetContent(m.pages[m.currentPage])
	m.viewport.GotoTop()
}

func (m *Model) renderMarkdown(source string, width int) string {
	if m.markdown == nil || m.markdownWidth != width {
		style := markdownStyle()
		renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(width))
		if err != nil {
			return source
		}
		m.markdown = renderer
		m.markdownWidth = width
		m.markdownCache = make(map[string]string)
	}
	if rendered, ok := m.markdownCache[source]; ok {
		return rendered
	}
	rendered, err := m.markdown.Render(source)
	if err != nil {
		return source
	}
	rendered = strings.Trim(rendered, "\n")
	m.markdownCache[source] = rendered
	return rendered
}

func markdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	zero := uint(0)
	one := uint(1)
	bold := true
	italic := true
	text := "#E8E6F0"
	heading := "#A995FF"
	accent := "#FF7A90"
	mutedText := "#777184"
	quote := "│ "

	style.Document.Margin = &zero
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""
	style.Document.Color = &text
	style.BlockQuote.Indent = &one
	style.BlockQuote.IndentToken = &quote
	style.BlockQuote.Color = &mutedText
	style.List.LevelIndent = 2
	style.Heading.BlockSuffix = "\n"
	style.Heading.Color = &heading
	style.Heading.Bold = &bold
	style.H1.Prefix, style.H1.Suffix = "", ""
	style.H1.BackgroundColor = nil
	style.H1.Color = &accent
	style.H2.Prefix, style.H3.Prefix = "", ""
	style.H2.Color, style.H3.Color = &heading, &heading
	style.H4.Prefix, style.H5.Prefix, style.H6.Prefix = "", "", ""
	style.H4.Color, style.H5.Color, style.H6.Color = &heading, &heading, &mutedText
	style.H6.Bold = &bold
	style.Strong.Bold = &bold
	style.Emph.Italic = &italic
	style.Item.BlockPrefix = "• "
	style.Code.Color = &accent
	style.CodeBlock.Margin = &zero
	style.CodeBlock.Indent = &one
	style.Link.Color = &heading
	style.LinkText.Color = &heading
	return style
}

func (m Model) View() string {
	if m.width == 0 {
		return "Iniciando Alice…"
	}
	if m.screen == historyScreen {
		return m.historyView()
	}
	header := logoStyle.Render("◆ ALICE") + "  " + mutedStyle.Render(m.conversation.Title)
	page := mutedStyle.Render(fmt.Sprintf("pág %d/%d", m.currentPage+1, m.totalPages))
	main := lipgloss.NewStyle().Width(m.viewport.Width).Render(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Width(m.viewport.Width).Render(header),
		m.viewport.View(),
		lipgloss.NewStyle().Foreground(muted).Render(m.status+"  "+page),
		lipgloss.NewStyle().BorderTop(true).BorderForeground(surface).Render(m.input.View()),
		mutedStyle.Render(helpText),
	))
	return m.placeColumn(main)
}

func (m Model) historyView() string {
	var body strings.Builder
	body.WriteString(logoStyle.Render("◆ ALICE") + " · " + mutedStyle.Render("HISTORIAL") + "\n\n")
	if m.searchActive {
		body.WriteString("Buscar:\n" + m.searchInput.View() + "\n\n")
	}
	results := m.historyResults()
	if len(m.conversations) == 0 {
		body.WriteString(mutedStyle.Render("Sin conversaciones") + "\n")
	} else if len(results) == 0 {
		body.WriteString(mutedStyle.Render("Sin resultados") + "\n")
	} else {
		for resultIndex, conversationIndex := range results {
			conversation := m.conversations[conversationIndex]
			prefix := "  "
			style := mutedStyle
			if resultIndex == m.historyIndex {
				prefix = "● "
				style = lipgloss.NewStyle().Foreground(lavender).Bold(true)
			}
			title := conversation.Title
			available := max(1, m.viewport.Width-4)
			if len([]rune(title)) > available {
				title = string([]rune(title)[:max(0, available-1)]) + "…"
			}
			body.WriteString(style.Render(prefix+title) + "\n")
		}
	}
	if m.deleteConfirm {
		if index := m.selectedHistoryConversationIndex(); index >= 0 {
			body.WriteString("\n" + lipgloss.NewStyle().Foreground(coral).Render(fmt.Sprintf("Borrar %q? y/n", m.conversations[index].Title)) + "\n")
		}
	}
	if m.renameActive {
		body.WriteString("\nRenombrar:\n" + m.renameInput.View() + "\n")
	}
	help := "↑/↓ seleccionar · Enter abrir · / buscar · r renombrar · d borrar · Esc volver"
	if m.searchActive {
		help = "↑/↓ seleccionar · Enter abrir · Esc limpiar"
	}
	body.WriteString("\n" + mutedStyle.Render(help))
	column := lipgloss.NewStyle().Width(m.viewport.Width).Render(body.String())
	return m.placeColumn(column)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) Debug() string { return fmt.Sprintf("%dx%d", m.width, m.height) }
