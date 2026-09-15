package config

import (
	"testing"

	"github.com/volcanic001/alice/internal/provider"
)

func TestLoadUsesEffectiveDeepSeekFlashConfiguration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_BASE_URL", "")
	t.Setenv("DEEPSEEK_API_KEY", "test")

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
}
