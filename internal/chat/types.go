package chat

import "context"

type Message struct {
	ID             int64
	ConversationID int64
	Role           string
	Content        string
}

type Conversation struct {
	ID        int64
	Title     string
	UpdatedAt string
}

type Request struct {
	Model       string
	Messages    []Message
	Temperature float64
}

type Event struct {
	Text string
	Err  error
	Done bool
}

type Provider interface {
	Name() string
	Stream(context.Context, Request) <-chan Event
}
