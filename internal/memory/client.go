// Package memory habla con la API en la nube de mem0 (https://mem0.ai) para
// que Alice recuerde hechos del usuario entre conversaciones. mem0 no tiene
// SDK en Go, así que este cliente llama directamente su API REST.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/volcanic001/alice/internal/chat"
)

const defaultBaseURL = "https://api.mem0.ai"

// Client es un cliente REST mínimo para las partes de la API de mem0 que usa
// Alice: buscar hechos relevantes y guardar un intercambio para que mem0 los
// extraiga. No depende de ningún vector store local; todo vive en mem0 Cloud.
type Client struct {
	APIKey     string
	UserID     string
	BaseURL    string // opcional; por defecto api.mem0.ai
	HTTPClient *http.Client
}

// New crea un cliente contra mem0 Cloud para un usuario concreto.
func New(apiKey, userID string) *Client {
	return &Client{APIKey: apiKey, UserID: userID}
}

func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return defaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

func (c *Client) do(ctx context.Context, path string, body any) (*http.Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Token "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")
	return c.httpClient().Do(request)
}

func readError(path string, response *http.Response) error {
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	return fmt.Errorf("mem0: %s devolvió %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
}

type searchResponse struct {
	Results []struct {
		Memory string `json:"memory"`
	} `json:"results"`
}

// Search devuelve hasta cinco hechos relevantes que mem0 tiene guardados
// sobre el usuario, del más al menos relevante. Una búsqueda vacía o fallida
// no es un error que deba llegar al usuario: quien llama debe tratarla como
// "sin memorias disponibles" y seguir sin ellas.
func (c *Client) Search(ctx context.Context, query string) ([]string, error) {
	response, err := c.do(ctx, "/v3/memories/search/", map[string]any{
		"query":   query,
		"filters": map[string]string{"user_id": c.UserID},
		"top_k":   5,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		return nil, readError("search", response)
	}
	defer response.Body.Close()
	var parsed searchResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	facts := make([]string, 0, len(parsed.Results))
	for _, result := range parsed.Results {
		if result.Memory != "" {
			facts = append(facts, result.Memory)
		}
	}
	return facts, nil
}

// Add manda un intercambio a mem0 para que extraiga y guarde hechos nuevos.
// El procesamiento en mem0 es asíncrono: Add solo confirma que lo aceptaron,
// no que la extracción ya terminó.
func (c *Client) Add(ctx context.Context, messages []chat.Message) error {
	payload := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		payload = append(payload, map[string]string{"role": message.Role, "content": message.Content})
	}
	response, err := c.do(ctx, "/v3/memories/add/", map[string]any{
		"messages": payload,
		"user_id":  c.UserID,
	})
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 {
		return readError("add", response)
	}
	defer response.Body.Close()
	return nil
}
