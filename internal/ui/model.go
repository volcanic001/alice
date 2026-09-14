package ui

import (
	"context"
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
	view.MouseWheelEnabled = true
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
		switch message.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "esc":
			if m.busy && m.cancel != nil {
				m.cancel()
				m.busy = false
				m.status = "Respuesta cancelada"
				return m, nil
			}
		case "ctrl+n":
			if !m.busy {
				conversation, err := m.store.CreateConversation()
				if err != nil {
					m.status = err.Error()
					return m, nil
				}
				m.conversations = append([]chat.Conversation{conversation}, m.conversations...)
				m.conversation, m.messages = conversation, nil
				m.refresh()
			}
			return m, nil
		case "enter":
			if !m.busy && strings.TrimSpace(m.input.Value()) != "" {
				return m.send()
			}
		case "pgup":
			m.viewport.HalfViewUp()
			return m, nil
		case "pgdown":
			m.viewport.HalfViewDown()
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
			m.status = event.Err.Error()
			m.cancel = nil
			return m, nil
		}
		if event.Text != "" {
			m.draft += event.Text
			m.refresh()
			m.viewport.GotoBottom()
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
	m.viewport, command = m.viewport.Update(message)
	commands = append(commands, command)
	if !m.busy {
		m.input, command = m.input.Update(message)
		commands = append(commands, command)
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
	m.viewport.GotoBottom()
	return m, waitForEvent(m.stream)
}

func (m Model) openRelative(delta int) (tea.Model, tea.Cmd) {
	index := 0
	for i, conversation := range m.conversations {
		if conversation.ID == m.conversation.ID {
			index = i
			break
		}
	}
	index += delta
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
	m.refresh()
	m.viewport.GotoBottom()
	return m, nil
}

func (m *Model) resize() {
	footer := 7
	sidebar := 0
	if m.width >= 90 {
		sidebar = min(28, m.width/3)
	}
	m.viewport.Width = max(20, m.width-sidebar-4)
	m.viewport.Height = max(4, m.height-footer)
	m.input.SetWidth(max(20, m.width-sidebar-4))
	m.refresh()
}

func (m *Model) refresh() {
	width := max(20, m.viewport.Width-2)
	messageWidth := max(10, width-4)
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
	m.viewport.SetContent(body.String())
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
	header := logoStyle.Render("◆ ALICE") + "  " + mutedStyle.Render(m.conversation.Title)
	main := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Width(m.viewport.Width).Padding(0, 1).Render(header),
		m.viewport.View(),
		lipgloss.NewStyle().Foreground(muted).Render(m.status),
		lipgloss.NewStyle().BorderTop(true).BorderForeground(surface).Render(m.input.View()),
		mutedStyle.Render("Enter enviar · Shift+Enter salto · Ctrl+N nuevo · PgUp/PgDn scroll · Esc cancelar"),
	)
	if m.width < 90 {
		return main
	}
	sideWidth := min(28, m.width/3)
	var side strings.Builder
	side.WriteString(logoStyle.Render("Conversaciones") + "\n\n")
	for _, conversation := range m.conversations {
		prefix := "  "
		style := mutedStyle
		if conversation.ID == m.conversation.ID {
			prefix = "● "
			style = lipgloss.NewStyle().Foreground(lavender).Bold(true)
		}
		title := conversation.Title
		if len([]rune(title)) > sideWidth-4 {
			title = string([]rune(title)[:sideWidth-5]) + "…"
		}
		side.WriteString(style.Render(prefix+title) + "\n")
	}
	sidebar := lipgloss.NewStyle().Width(sideWidth).Height(m.height - 1).BorderRight(true).BorderForeground(surface).Padding(1).Render(side.String())
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
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
