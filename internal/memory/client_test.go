package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/volcanic001/alice/internal/chat"
)

func TestSearchSendsAuthAndFiltersByUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v3/memories/search/" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Token secreto" {
			t.Errorf("autorización inesperada: %q", request.Header.Get("Authorization"))
		}
		var payload struct {
			Query   string            `json:"query"`
			Filters map[string]string `json:"filters"`
			TopK    int               `json:"top_k"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Query != "¿dónde vivo?" || payload.Filters["user_id"] != "usuario-1" || payload.TopK != 5 {
			t.Fatalf("payload inesperado: %+v", payload)
		}
		fmt.Fprint(response, `{"results":[{"memory":"vive en Caracas"},{"memory":"prefiere respuestas cortas"}]}`)
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", UserID: "usuario-1", BaseURL: server.URL}
	facts, err := client.Search(context.Background(), "¿dónde vivo?")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 || facts[0] != "vive en Caracas" || facts[1] != "prefiere respuestas cortas" {
		t.Fatalf("hechos inesperados: %v", facts)
	}
}

func TestSearchSkipsEmptyMemoryEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"results":[{"memory":""},{"memory":"único hecho válido"}]}`)
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", UserID: "usuario-1", BaseURL: server.URL}
	facts, err := client.Search(context.Background(), "consulta")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0] != "único hecho válido" {
		t.Fatalf("hechos inesperados: %v", facts)
	}
}

func TestSearchReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(response, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	client := Client{APIKey: "mala", UserID: "usuario-1", BaseURL: server.URL}
	if _, err := client.Search(context.Background(), "consulta"); err == nil {
		t.Fatal("se esperaba un error")
	}
}

func TestAddSendsMessagesAndUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v3/memories/add/" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		var payload struct {
			Messages []map[string]string `json:"messages"`
			UserID   string              `json:"user_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.UserID != "usuario-1" {
			t.Fatalf("user_id inesperado: %q", payload.UserID)
		}
		if len(payload.Messages) != 2 || payload.Messages[0]["role"] != "user" || payload.Messages[1]["role"] != "assistant" {
			t.Fatalf("mensajes inesperados: %+v", payload.Messages)
		}
		fmt.Fprint(response, `{"event_id":"abc","status":"PENDING"}`)
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", UserID: "usuario-1", BaseURL: server.URL}
	err := client.Add(context.Background(), []chat.Message{
		{Role: "user", Content: "me mudé a Caracas"},
		{Role: "assistant", Content: "anotado"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(response, "falla interna")
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", UserID: "usuario-1", BaseURL: server.URL}
	err := client.Add(context.Background(), []chat.Message{{Role: "user", Content: "hola"}})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error inesperado: %v", err)
	}
}
