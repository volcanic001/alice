package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContextSendsAuthAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/llm/context" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		if request.Header.Get("X-Subscription-Token") != "secreto" {
			t.Errorf("header de autorización inesperado: %q", request.Header.Get("X-Subscription-Token"))
		}
		if request.URL.Query().Get("q") != "clima en San Salvador" {
			t.Errorf("query inesperada: %q", request.URL.Query().Get("q"))
		}
		if request.URL.Query().Get("maximum_number_of_urls") != "4" {
			t.Errorf("maximum_number_of_urls inesperado: %q", request.URL.Query().Get("maximum_number_of_urls"))
		}
		fmt.Fprint(response, `{
			"grounding": {"generic": [
				{"url":"https://example.com/clima","title":"Clima El Salvador","snippets":["Soleado, 30°C","Humedad 60%"]},
				{"url":"","title":"","snippets":["entrada vacía, debe descartarse"]}
			]},
			"sources": {"https://example.com/clima": {"age":["2026-10-07T12:00:00Z","hoy"]}}
		}`)
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", BaseURL: server.URL}
	results, err := client.Context(context.Background(), "clima en San Salvador")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "Clima El Salvador" || results[0].URL != "https://example.com/clima" {
		t.Fatalf("resultados inesperados: %#v", results)
	}
	if len(results[0].Snippets) != 2 || results[0].Snippets[0] != "Soleado, 30°C" {
		t.Fatalf("snippets inesperados: %#v", results[0].Snippets)
	}
	if len(results[0].Age) != 2 || results[0].Age[0] != "2026-10-07T12:00:00Z" {
		t.Fatalf("age inesperada: %#v", results[0].Age)
	}
}

func TestContextReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(response, `{"error":"invalid token"}`)
	}))
	defer server.Close()

	client := Client{APIKey: "mala", BaseURL: server.URL}
	_, err := client.Context(context.Background(), "consulta")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestContextHandlesEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"grounding":{"generic":[]},"sources":{}}`)
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", BaseURL: server.URL}
	results, err := client.Context(context.Background(), "algo muy raro")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("se esperaban cero resultados: %#v", results)
	}
}

func TestNewsSendsFreshnessPastDay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/news/search" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		if request.Header.Get("X-Subscription-Token") != "secreto" {
			t.Errorf("header de autorización inesperado: %q", request.Header.Get("X-Subscription-Token"))
		}
		if request.URL.Query().Get("q") != "elecciones" {
			t.Errorf("query inesperada: %q", request.URL.Query().Get("q"))
		}
		if request.URL.Query().Get("freshness") != "pd" {
			t.Errorf("freshness inesperado: %q, se esperaba 'pd' (últimas 24h)", request.URL.Query().Get("freshness"))
		}
		fmt.Fprint(response, `{"type":"news","query":{"original":"elecciones"},"results":[
			{"type":"news_result","title":"Resultado de ayer","url":"https://example.com/a","description":"D1","age":"2 hours ago","breaking":true},
			{"type":"news_result","title":"","url":"","description":"entrada vacía, debe descartarse"}
		]}`)
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", BaseURL: server.URL}
	results, err := client.News(context.Background(), "elecciones", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "Resultado de ayer" || results[0].Age != "2 hours ago" || !results[0].Breaking {
		t.Fatalf("resultados inesperados: %#v", results)
	}
}

func TestNewsReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := Client{APIKey: "secreto", BaseURL: server.URL}
	_, err := client.News(context.Background(), "algo", 5)
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("error inesperado: %v", err)
	}
}
