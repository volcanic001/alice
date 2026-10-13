package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/volcanic001/alice/internal/chat"
)

// VeniceRecommendedModel corre en un TEE (Trusted Execution Environment) con
// cifrado de extremo a extremo: ni Venice puede leer el contenido en claro.
// Es la variante "uncensored" de Gemma 4 26B (MoE, 3.8B activos): sin el
// entrenamiento de seguridad estándar, a cambio de menos contexto (64K) y
// sin tool-calling.
const (
	VeniceRecommendedModel         = "e2ee-gemma-4-26b-a4b-uncensored-p"
	VeniceRecommendedContextWindow = 64_000
)

type Venice struct {
	APIKey            string
	BaseURL           string
	Client            *http.Client
	FirstTokenTimeout time.Duration
}

func (v Venice) Name() string { return "Venice" }

func (v Venice) Stream(ctx context.Context, request chat.Request) <-chan chat.Event {
	return streamOpenAICompatible(ctx, request, openAICompatible{
		Name: "Venice", APIKey: v.APIKey, BaseURL: v.BaseURL, DefaultBaseURL: "https://api.venice.ai/api/v1",
		Client: v.Client, FirstTokenTimeout: v.FirstTokenTimeout,
	})
}
