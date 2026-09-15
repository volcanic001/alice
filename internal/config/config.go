package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float64
	DataPath    string
	UsagePath   string
}

func Load() (Config, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	directory = filepath.Join(directory, "alice")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return Config{}, err
	}
	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	return Config{
		APIKey: os.Getenv("DEEPSEEK_API_KEY"), BaseURL: baseURL,
		Model: "deepseek-chat", Temperature: 0.7,
		DataPath:  filepath.Join(directory, "alice.db"),
		UsagePath: filepath.Join(directory, "usage.json"),
	}, nil
}
