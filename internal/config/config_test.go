package config

import (
	"testing"

	"github.com/volcanic001/alice/internal/provider"
)

func TestLoadUsesEffectiveDeepSeekFlashConfiguration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_BASE_URL", "")
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("MEM0_API_KEY", "")
	t.Setenv("BRAVE_API_KEY", "")
	t.Setenv("ALICE_SYSTEM_PROMPT", "")

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Model != provider.DeepSeekFlashModel {
		t.Fatalf("modelo = %q, se esperaba %q", configuration.Model, provider.DeepSeekFlashModel)
	}
	if configuration.Temperature != 0.7 {
		t.Fatalf("temperature = %v, se esperaba 0.7", configuration.Temperature)
	}
	if configuration.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("base URL inesperada: %q", configuration.BaseURL)
	}
	if configuration.Mem0APIKey != "" || configuration.Mem0UserID != "" {
		t.Fatalf("mem0 debería quedar desactivado sin MEM0_API_KEY: %+v", configuration)
	}
	if configuration.BraveAPIKey != "" {
		t.Fatalf("la búsqueda web debería quedar desactivada sin BRAVE_API_KEY: %+v", configuration)
	}
	if configuration.SystemPrompt != "" {
		t.Fatalf("no debería haber prompt de sistema sin configurar: %+v", configuration)
	}
}

func TestLoadGeneratesAndPersistsMem0UserID(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("MEM0_API_KEY", "clave-mem0")

	first, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if first.Mem0APIKey != "clave-mem0" {
		t.Fatalf("Mem0APIKey = %q, se esperaba clave-mem0", first.Mem0APIKey)
	}
	if first.Mem0UserID == "" {
		t.Fatal("se esperaba un Mem0UserID generado")
	}

	second, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if second.Mem0UserID != first.Mem0UserID {
		t.Fatalf("el user id de mem0 cambió entre cargas: %q != %q", second.Mem0UserID, first.Mem0UserID)
	}
}

func TestSaveMem0APIKeyActivatesMemoryWithoutEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("MEM0_API_KEY", "")

	userID, err := SaveMem0APIKey("clave-desde-la-app")
	if err != nil {
		t.Fatal(err)
	}
	if userID == "" {
		t.Fatal("se esperaba un user id generado al guardar la clave")
	}

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Mem0APIKey != "clave-desde-la-app" || configuration.Mem0UserID != userID {
		t.Fatalf("Load no recogió la clave guardada por /memory: %+v", configuration)
	}
}

func TestClearMem0APIKeyWinsOverStaleEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("MEM0_API_KEY", "clave-vieja-del-entorno")

	if _, err := SaveMem0APIKey("clave-desde-la-app"); err != nil {
		t.Fatal(err)
	}
	if err := ClearMem0APIKey(); err != nil {
		t.Fatal(err)
	}

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Mem0APIKey != "" || configuration.Mem0UserID != "" {
		t.Fatalf("/memory clear debe ganarle a una MEM0_API_KEY del entorno: %+v", configuration)
	}
}

func TestSaveBraveAPIKeyActivatesSearchWithoutEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("BRAVE_API_KEY", "")

	if err := SaveBraveAPIKey("clave-brave"); err != nil {
		t.Fatal(err)
	}

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.BraveAPIKey != "clave-brave" {
		t.Fatalf("Load no recogió la clave guardada por /search-key: %+v", configuration)
	}
}

func TestClearBraveAPIKeyWinsOverStaleEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("BRAVE_API_KEY", "clave-vieja-del-entorno")

	if err := SaveBraveAPIKey("clave-desde-la-app"); err != nil {
		t.Fatal(err)
	}
	if err := ClearBraveAPIKey(); err != nil {
		t.Fatal(err)
	}

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.BraveAPIKey != "" {
		t.Fatalf("/search-key clear debe ganarle a una BRAVE_API_KEY del entorno: %+v", configuration)
	}
}

func TestLoadFallsBackToSystemPromptEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("ALICE_SYSTEM_PROMPT", "Eres un asistente pirata")

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SystemPrompt != "Eres un asistente pirata" {
		t.Fatalf("SystemPrompt = %q, se esperaba el valor de ALICE_SYSTEM_PROMPT", configuration.SystemPrompt)
	}
}

func TestSaveSystemPromptActivatesItWithoutEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("ALICE_SYSTEM_PROMPT", "")

	if err := SaveSystemPrompt("Responde siempre en español neutro"); err != nil {
		t.Fatal(err)
	}

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SystemPrompt != "Responde siempre en español neutro" {
		t.Fatalf("Load no recogió el prompt guardado por /prompt: %+v", configuration)
	}
}

func TestClearSystemPromptWinsOverStaleEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test")
	t.Setenv("ALICE_SYSTEM_PROMPT", "prompt viejo del entorno")

	if err := SaveSystemPrompt("prompt desde la app"); err != nil {
		t.Fatal(err)
	}
	if err := ClearSystemPrompt(); err != nil {
		t.Fatal(err)
	}

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SystemPrompt != "" {
		t.Fatalf("/prompt clear debe ganarle a un ALICE_SYSTEM_PROMPT del entorno: %+v", configuration)
	}
}
