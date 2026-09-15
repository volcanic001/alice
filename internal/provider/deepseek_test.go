package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func firstEvent(client DeepSeek, ctx context.Context) chat.Event {
	return <-client.Stream(ctx, chat.Request{Model: "deepseek-chat"})
}

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

func TestDeepSeekHTTPErrorMessages(t *testing.T) {
	tests := []struct {
		name, body, expected string
		status               int
		kind                 ErrorKind
	}{
		{"401", `{"error":{"message":"api key: sk-secret-817e"}}`, "⚠ Error de autenticación\nLa API key de DeepSeek no es válida.", http.StatusUnauthorized, AuthenticationError},
		{"403", `{}`, "⚠ Error de autenticación\nLa API key de DeepSeek no es válida.", http.StatusForbidden, AuthenticationError},
		{"402", `{}`, "⚠ Saldo insuficiente\nRevisa el saldo de tu cuenta de DeepSeek.", http.StatusPaymentRequired, InsufficientBalanceError},
		{"saldo en respuesta", `{"error":{"code":"insufficient_balance"}}`, "⚠ Saldo insuficiente\nRevisa el saldo de tu cuenta de DeepSeek.", http.StatusBadRequest, InsufficientBalanceError},
		{"429", `{}`, "⚠ Demasiadas solicitudes\nEspera un momento e inténtalo de nuevo.", http.StatusTooManyRequests, RateLimitError},
		{"500", `{}`, "⚠ DeepSeek no está disponible\nInténtalo de nuevo más tarde.", http.StatusInternalServerError, ServiceUnavailableError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.WriteHeader(test.status)
				fmt.Fprint(response, test.body)
			}))
			defer server.Close()

			event := firstEvent(DeepSeek{APIKey: "secreto", BaseURL: server.URL}, context.Background())
			if event.Err == nil || event.Err.Error() != test.expected {
				t.Fatalf("mensaje=%q, se esperaba %q", event.Err, test.expected)
			}
			if strings.Contains(event.Err.Error(), "sk-secret-817e") {
				t.Fatalf("el mensaje público expuso la key: %q", event.Err)
			}
			var apiError *APIError
			if !errors.As(event.Err, &apiError) || apiError.Kind != test.kind || apiError.StatusCode != test.status {
				t.Fatalf("error estructurado inesperado: %#v", event.Err)
			}
		})
	}
}

func TestDeepSeekTimesOutWaitingForFirstToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		fmt.Fprint(response, ": keep-alive\n\n")
		response.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()

	client := DeepSeek{APIKey: "secreto", BaseURL: server.URL, FirstTokenTimeout: 50 * time.Millisecond}
	event := firstEvent(client, context.Background())
	if event.Err == nil || event.Err.Error() != "⚠ La solicitud tardó demasiado\nInténtalo de nuevo." {
		t.Fatalf("se esperaba timeout amigable, se recibió: %+v", event)
	}
}

func TestDeepSeekNetworkError(t *testing.T) {
	client := DeepSeek{
		APIKey:  "secreto",
		BaseURL: "https://deepseek.test",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("red no disponible")
		})},
	}
	event := firstEvent(client, context.Background())
	if event.Err == nil || event.Err.Error() != "⚠ No se pudo conectar con DeepSeek\nRevisa tu conexión a Internet." {
		t.Fatalf("se esperaba error de red amigable, se recibió: %+v", event)
	}
}

func TestDeepSeekUnknownError(t *testing.T) {
	err := classifyError(errors.New("fallo inesperado"))
	if err.Error() != "⚠ Ocurrió un error\nNo se pudo completar la respuesta." {
		t.Fatalf("mensaje inesperado: %q", err)
	}
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Kind != UnknownError {
		t.Fatalf("error estructurado inesperado: %#v", err)
	}
}

func TestDeepSeekUserCancellationIsNotTechnicalError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	event := firstEvent(DeepSeek{APIKey: "secreto", BaseURL: "https://deepseek.test"}, ctx)
	if !errors.Is(event.Err, context.Canceled) {
		t.Fatalf("se esperaba cancelación, se recibió: %#v", event.Err)
	}
}

func TestDeepSeekSSEError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(response, `data: {"error":{"code":"rate_limit_exceeded"}}`)
	}))
	defer server.Close()

	event := firstEvent(DeepSeek{APIKey: "secreto", BaseURL: server.URL}, context.Background())
	if event.Err == nil || event.Err.Error() != "⚠ Demasiadas solicitudes\nEspera un momento e inténtalo de nuevo." {
		t.Fatalf("error SSE inesperado: %+v", event)
	}
}

func TestDeepSeekStreamErrorAfterPartialText(t *testing.T) {
	client := DeepSeek{
		APIKey:  "secreto",
		BaseURL: "https://deepseek.test",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(&errorReader{data: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"parcial\"}}]}\n"), err: errors.New("conexión perdida")})}, nil
		})},
	}
	var text string
	var streamError error
	for event := range client.Stream(context.Background(), chat.Request{Model: "deepseek-chat"}) {
		text += event.Text
		streamError = event.Err
	}
	if text != "parcial" || streamError == nil || streamError.Error() != "⚠ No se pudo conectar con DeepSeek\nRevisa tu conexión a Internet." {
		t.Fatalf("stream parcial inesperado: texto=%q error=%v", text, streamError)
	}
}

type errorReader struct {
	data []byte
	err  error
}

func (r *errorReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
