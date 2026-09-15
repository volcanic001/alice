package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

type DeepSeek struct {
	APIKey            string
	BaseURL           string
	Client            *http.Client
	FirstTokenTimeout time.Duration
}

func (d DeepSeek) Name() string { return "DeepSeek" }

func (d DeepSeek) Stream(ctx context.Context, request chat.Request) <-chan chat.Event {
	out := make(chan chat.Event)
	go func() {
		defer close(out)
		if d.APIKey == "" {
			out <- chat.Event{Err: errors.New("falta DEEPSEEK_API_KEY")}
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
		})
		if err != nil {
			out <- chat.Event{Err: err}
			return
		}
		baseURL := strings.TrimRight(d.BaseURL, "/")
		if baseURL == "" {
			baseURL = "https://api.deepseek.com"
		}
		req, err := http.NewRequestWithContext(requestContext, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			out <- chat.Event{Err: err}
			return
		}
		req.Header.Set("Authorization", "Bearer "+d.APIKey)
		req.Header.Set("Content-Type", "application/json")
		client := d.Client
		if client == nil {
			client = http.DefaultClient
		}
		response, err := client.Do(req)
		if err != nil {
			select {
			case <-timedOut:
				out <- chat.Event{Err: fmt.Errorf("DeepSeek no envió ningún token en %s", timeout)}
			default:
				out <- chat.Event{Err: err}
			}
			return
		}
		defer response.Body.Close()
		if response.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			out <- chat.Event{Err: fmt.Errorf("DeepSeek respondió %s: %s", response.Status, strings.TrimSpace(string(body)))}
			return
		}
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				out <- chat.Event{Done: true}
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(data), &chunk) == nil && len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				timer.Stop()
				out <- chat.Event{Text: chunk.Choices[0].Delta.Content}
			}
		}
		select {
		case <-timedOut:
			out <- chat.Event{Err: fmt.Errorf("DeepSeek no envió ningún token en %s", timeout)}
			return
		default:
		}
		if err := scanner.Err(); err != nil {
			out <- chat.Event{Err: err}
			return
		}
		out <- chat.Event{Done: true}
	}()
	return out
}
