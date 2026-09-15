package chat

import (
	"context"
	"time"
)

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

// UsageRecord is the token usage reported by a provider for one successful request.
// It intentionally lives in memory only; storage and presentation are future work.
type UsageRecord struct {
	Model                 string
	PromptTokens          int
	CompletionTokens      int
	TotalTokens           int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
	RequestedAt           time.Time
}

type Event struct {
	Text  string
	Usage *UsageRecord
	Err   error
	Done  bool
}

type Provider interface {
	Name() string
	Stream(context.Context, Request) <-chan Event
}
