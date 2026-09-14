package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/volcanic001/alice/internal/chat"
)

func TestDeepSeekStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secreto" {
			t.Errorf("autorización inesperada: %q", request.Header.Get("Authorization"))
		}
		response.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(response, `data: {"choices":[{"delta":{"content":"Hola"}}]}`)
		fmt.Fprintln(response)
		fmt.Fprintln(response, `data: {"choices":[{"delta":{"content":" mundo"}}]}`)
		fmt.Fprintln(response)
		fmt.Fprintln(response, "data: [DONE]")
	}))
	defer server.Close()

	client := DeepSeek{APIKey: "secreto", BaseURL: server.URL}
	var received string
	var done bool
	for event := range client.Stream(context.Background(), chat.Request{Model: "deepseek-chat"}) {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		received += event.Text
		done = done || event.Done
	}
	if received != "Hola mundo" || !done {
		t.Fatalf("stream inesperado: texto=%q done=%v", received, done)
	}
}
