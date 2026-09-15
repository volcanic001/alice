package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

type ErrorKind string

const (
	AuthenticationError      ErrorKind = "authentication"
	InsufficientBalanceError ErrorKind = "insufficient_balance"
	RateLimitError           ErrorKind = "rate_limit"
	ServiceUnavailableError  ErrorKind = "service_unavailable"
	TimeoutError             ErrorKind = "timeout"
	NetworkError             ErrorKind = "network"
	UnknownError             ErrorKind = "unknown"
)

// APIError conserva datos técnicos para diagnóstico sin exponerlos en Error.
type APIError struct {
	Kind       ErrorKind
	StatusCode int
	Cause      error
}

func (e *APIError) Error() string {
	switch e.Kind {
	case AuthenticationError:
		return "⚠ Error de autenticación\nLa API key de DeepSeek no es válida."
	case InsufficientBalanceError:
		return "⚠ Saldo insuficiente\nRevisa el saldo de tu cuenta de DeepSeek."
	case RateLimitError:
		return "⚠ Demasiadas solicitudes\nEspera un momento e inténtalo de nuevo."
	case ServiceUnavailableError:
		return "⚠ DeepSeek no está disponible\nInténtalo de nuevo más tarde."
	case TimeoutError:
		return "⚠ La solicitud tardó demasiado\nInténtalo de nuevo."
	case NetworkError:
		return "⚠ No se pudo conectar con DeepSeek\nRevisa tu conexión a Internet."
	default:
		return "⚠ Ocurrió un error\nNo se pudo completar la respuesta."
	}
}

func (e *APIError) Unwrap() error { return e.Cause }

func (e *APIError) Technical() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Kind)
}

func apiError(kind ErrorKind, statusCode int, cause error) error {
	return &APIError{Kind: kind, StatusCode: statusCode, Cause: cause}
}

func classifyHTTPError(statusCode int, body []byte) error {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return apiError(AuthenticationError, statusCode, nil)
	case statusCode == http.StatusPaymentRequired || indicatesInsufficientBalance(body):
		return apiError(InsufficientBalanceError, statusCode, nil)
	case statusCode == http.StatusTooManyRequests:
		return apiError(RateLimitError, statusCode, nil)
	case statusCode >= http.StatusInternalServerError && statusCode < 600:
		return apiError(ServiceUnavailableError, statusCode, nil)
	default:
		return apiError(UnknownError, statusCode, nil)
	}
}

func indicatesInsufficientBalance(body []byte) bool {
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false
	}
	details := strings.ToLower(strings.Join([]string{response.Error.Code, response.Error.Message, response.Error.Type}, " "))
	return strings.Contains(details, "insufficient_balance") || strings.Contains(details, "insufficient balance") || strings.Contains(details, "insufficient credit") || strings.Contains(details, "insufficient quota")
}

func classifyTransportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return apiError(TimeoutError, 0, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return apiError(TimeoutError, 0, err)
	}
	return apiError(NetworkError, 0, err)
}

func classifyError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return apiError(TimeoutError, 0, err)
	}
	return apiError(UnknownError, 0, err)
}

func classifyStreamError(body []byte) error {
	var response struct {
		Error *struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil || response.Error == nil {
		return nil
	}
	details := strings.ToLower(response.Error.Code + " " + response.Error.Type)
	switch {
	case strings.Contains(details, "authentication") || strings.Contains(details, "invalid_api_key"):
		return apiError(AuthenticationError, 0, nil)
	case indicatesInsufficientBalance(body):
		return apiError(InsufficientBalanceError, 0, nil)
	case strings.Contains(details, "rate_limit"):
		return apiError(RateLimitError, 0, nil)
	case strings.Contains(details, "server") || strings.Contains(details, "service_unavailable"):
		return apiError(ServiceUnavailableError, 0, nil)
	default:
		return apiError(UnknownError, 0, nil)
	}
}

type DeepSeek struct {
	APIKey            string
	BaseURL           string
	Client            *http.Client
	FirstTokenTimeout time.Duration
}

func (d DeepSeek) Name() string { return "DeepSeek" }

type deepSeekUsage struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
}

type deepSeekStreamChunk struct {
	Model   string         `json:"model"`
	Usage   *deepSeekUsage `json:"usage"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func usageRecord(model string, usage *deepSeekUsage, requestedAt time.Time) *chat.UsageRecord {
	if usage == nil {
		return nil
	}
	return &chat.UsageRecord{
		Model:                 model,
		PromptTokens:          usage.PromptTokens,
		CompletionTokens:      usage.CompletionTokens,
		TotalTokens:           usage.TotalTokens,
		PromptCacheHitTokens:  usage.PromptCacheHitTokens,
		PromptCacheMissTokens: usage.PromptCacheMissTokens,
		RequestedAt:           requestedAt,
	}
}

func (d DeepSeek) Stream(ctx context.Context, request chat.Request) <-chan chat.Event {
	out := make(chan chat.Event)
	go func() {
		defer close(out)
		if d.APIKey == "" {
			out <- chat.Event{Err: apiError(AuthenticationError, 0, errors.New("missing API key"))}
			return
		}
		timeout := d.FirstTokenTimeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		requestContext, cancelRequest := context.WithCancel(ctx)
		defer cancelRequest()
		timedOut := make(chan struct{})
		timer := time.AfterFunc(timeout, func() {
			close(timedOut)
			cancelRequest()
		})
		defer timer.Stop()

		messages := make([]map[string]string, 0, len(request.Messages))
		for _, message := range request.Messages {
			messages = append(messages, map[string]string{"role": message.Role, "content": message.Content})
		}
		payload, err := json.Marshal(map[string]any{
			"model": request.Model, "messages": messages,
			"temperature": request.Temperature, "stream": true,
			"stream_options": map[string]bool{"include_usage": true},
		})
		if err != nil {
			out <- chat.Event{Err: classifyError(err)}
			return
		}
		baseURL := strings.TrimRight(d.BaseURL, "/")
		if baseURL == "" {
			baseURL = "https://api.deepseek.com"
		}
		req, err := http.NewRequestWithContext(requestContext, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			out <- chat.Event{Err: classifyError(err)}
			return
		}
		req.Header.Set("Authorization", "Bearer "+d.APIKey)
		req.Header.Set("Content-Type", "application/json")
		client := d.Client
		if client == nil {
			client = http.DefaultClient
		}
		requestedAt := time.Now()
		response, err := client.Do(req)
		if err != nil {
			select {
			case <-timedOut:
				out <- chat.Event{Err: apiError(TimeoutError, 0, context.DeadlineExceeded)}
			default:
				out <- chat.Event{Err: classifyTransportError(err)}
			}
			return
		}
		defer response.Body.Close()
		if response.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			out <- chat.Event{Err: classifyHTTPError(response.StatusCode, body)}
			return
		}
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				out <- chat.Event{Done: true}
				return
			}
			if streamError := classifyStreamError([]byte(data)); streamError != nil {
				out <- chat.Event{Err: streamError}
				return
			}
			var chunk deepSeekStreamChunk
			if json.Unmarshal([]byte(data), &chunk) != nil {
				continue
			}
			if usage := usageRecord(chunk.Model, chunk.Usage, requestedAt); usage != nil {
				timer.Stop()
				out <- chat.Event{Usage: usage}
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				timer.Stop()
				out <- chat.Event{Text: chunk.Choices[0].Delta.Content}
			}
		}
		select {
		case <-timedOut:
			out <- chat.Event{Err: apiError(TimeoutError, 0, context.DeadlineExceeded)}
			return
		default:
		}
		if err := scanner.Err(); err != nil {
			out <- chat.Event{Err: classifyTransportError(err)}
			return
		}
		out <- chat.Event{Done: true}
	}()
	return out
}
