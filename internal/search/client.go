// Package search habla con la Brave Search API
// (https://brave.com/search/api/) para que Alice pueda traer resultados
// actuales de internet cuando DeepSeek, por su cuenta, no puede saberlo.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const defaultBaseURL = "https://api.search.brave.com/res/v1"

// Client es un cliente REST mínimo para /web/search y /news/search de Brave.
// Ambos usan la misma API key — no hay credenciales separadas por endpoint.
type Client struct {
	APIKey     string
	BaseURL    string // opcional; por defecto api.search.brave.com/res/v1
	HTTPClient *http.Client
}

// New crea un cliente de Brave Search con una API key.
func New(apiKey string) *Client {
	return &Client{APIKey: apiKey}
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

// get hace el GET autenticado común a ambos endpoints y devuelve el cuerpo ya
// validado (2xx). El llamador decodifica el JSON según su propio shape.
func (c *Client) get(ctx context.Context, path string, params url.Values) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Subscription-Token", c.APIKey)
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("brave: %s devolvió %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response.Body, nil
}

// ContextSource es una página de /llm/context: a diferencia de un resultado
// de /web/search, Snippets trae fragmentos del contenido real de la página
// (texto, tablas, a veces datos en JSON), no solo el título y una
// descripción corta. Age es la antigüedad que Brave le asigna a la página
// —de cuándo la rastreó, no de cuándo ocurrió lo que dice el texto—: un
// snippet puede "parecer" en vivo (p. ej. un reloj) y aun así venir de un
// rastreo de hace meses, así que Age hay que tratarla con sospecha, nunca
// como un hecho confirmado.
type ContextSource struct {
	Title    string
	URL      string
	Snippets []string
	Age      []string
}

type llmContextResponse struct {
	Grounding struct {
		Generic []struct {
			URL      string   `json:"url"`
			Title    string   `json:"title"`
			Snippets []string `json:"snippets"`
		} `json:"generic"`
	} `json:"grounding"`
	Sources map[string]struct {
		Age []string `json:"age"`
	} `json:"sources"`
}

// Valores por defecto para /llm/context: suficientes páginas y tokens para
// que DeepSeek tenga con qué responder, sin que una sola búsqueda dispare un
// bloque de contexto enorme.
const (
	contextMaxURLs  = 4
	contextMaxToken = 3000
)

// Context trae contenido real extraído de hasta contextMaxURLs páginas
// relevantes para la consulta — el reemplazo de Search (/web/search) para
// /search: en vez de título+descripción, da fragmentos de la página misma,
// listos para que el modelo razone sobre ellos directamente.
func (c *Client) Context(ctx context.Context, query string) ([]ContextSource, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("maximum_number_of_urls", strconv.Itoa(contextMaxURLs))
	params.Set("maximum_number_of_tokens", strconv.Itoa(contextMaxToken))
	body, err := c.get(ctx, "/llm/context", params)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var parsed llmContextResponse
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return nil, err
	}
	sources := make([]ContextSource, 0, len(parsed.Grounding.Generic))
	for _, item := range parsed.Grounding.Generic {
		if item.Title == "" && item.URL == "" {
			continue
		}
		sources = append(sources, ContextSource{
			Title: item.Title, URL: item.URL, Snippets: item.Snippets,
			Age: parsed.Sources[item.URL].Age,
		})
	}
	return sources, nil
}

// NewsResult es un artículo de /news/search. A diferencia de Result, trae Age
// —la antigüedad legible que da Brave (p. ej. "2 hours ago")— que es
// precisamente lo que falta en /web/search y lo que permite no mezclar
// noticias viejas con nuevas.
type NewsResult struct {
	Title       string
	URL         string
	Description string
	Age         string
	Breaking    bool
}

type newsSearchResponse struct {
	Results []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Age         string `json:"age"`
		Breaking    bool   `json:"breaking"`
	} `json:"results"`
}

// newsFreshness limita /news/search a lo descubierto en las últimas 24
// horas (valor "pd" de Brave). Es justo lo que separa este endpoint de
// /web/search: aquí preferimos quedarnos sin resultados a devolver una
// noticia de hace meses mezclada con las de hoy.
const newsFreshness = "pd"

// News devuelve hasta `count` artículos recientes (últimas 24 horas) para la
// consulta dada.
func (c *Client) News(ctx context.Context, query string, count int) ([]NewsResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("freshness", newsFreshness)
	if count > 0 {
		params.Set("count", strconv.Itoa(count))
	}
	body, err := c.get(ctx, "/news/search", params)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var parsed newsSearchResponse
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return nil, err
	}
	results := make([]NewsResult, 0, len(parsed.Results))
	for _, item := range parsed.Results {
		if item.Title == "" && item.URL == "" {
			continue
		}
		results = append(results, NewsResult{
			Title: item.Title, URL: item.URL, Description: item.Description,
			Age: item.Age, Breaking: item.Breaking,
		})
	}
	return results, nil
}
