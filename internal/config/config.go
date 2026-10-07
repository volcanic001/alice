package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/volcanic001/alice/internal/provider"
)

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float64
	DataPath    string
	UsagePath   string
	Mem0APIKey  string
	Mem0UserID  string // vacío cuando Mem0APIKey está vacío
}

func Load() (Config, error) {
	directory, err := Mem0Directory()
	if err != nil {
		return Config{}, err
	}
	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	mem0APIKey, err := loadMem0APIKey(directory)
	if err != nil {
		return Config{}, err
	}
	var mem0UserID string
	if mem0APIKey != "" {
		mem0UserID, err = loadOrCreateMem0UserID(directory)
		if err != nil {
			return Config{}, err
		}
	}
	return Config{
		APIKey: os.Getenv("DEEPSEEK_API_KEY"), BaseURL: baseURL,
		Model: provider.DeepSeekFlashModel, Temperature: 0.7,
		DataPath:   filepath.Join(directory, "alice.db"),
		UsagePath:  filepath.Join(directory, "usage.json"),
		Mem0APIKey: mem0APIKey, Mem0UserID: mem0UserID,
	}, nil
}

// Mem0Directory es el directorio de configuración de Alice
// (~/.config/alice), creado si hace falta. Lo usa tanto Load como el comando
// /memory de la TUI para guardar y leer la clave de mem0 sin pasar por
// variables de entorno.
func Mem0Directory() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	directory = filepath.Join(directory, "alice")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return directory, nil
}

const mem0APIKeyFile = "mem0_api_key"

// loadMem0APIKey lee la clave guardada por /memory. Si el archivo nunca se
// creó, cae a MEM0_API_KEY del entorno (compatibilidad con quien prefiera
// exportarla). Si el archivo existe —aunque esté vacío, como deja /memory
// clear— manda sobre el entorno: es la elección explícita del usuario desde
// dentro de Alice.
func loadMem0APIKey(directory string) (string, error) {
	content, err := os.ReadFile(filepath.Join(directory, mem0APIKeyFile))
	switch {
	case err == nil:
		return strings.TrimSpace(string(content)), nil
	case os.IsNotExist(err):
		return os.Getenv("MEM0_API_KEY"), nil
	default:
		return "", err
	}
}

// SaveMem0APIKey guarda la clave de mem0 y devuelve el user_id asociado
// (generándolo si es la primera vez). Usado por el comando /memory.
func SaveMem0APIKey(apiKey string) (userID string, err error) {
	directory, err := Mem0Directory()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(directory, mem0APIKeyFile), []byte(apiKey+"\n"), 0600); err != nil {
		return "", err
	}
	return loadOrCreateMem0UserID(directory)
}

// ClearMem0APIKey desactiva la memoria de forma explícita. Deja el archivo
// vacío en vez de borrarlo, para que una MEM0_API_KEY del entorno no la
// reactive sola en el siguiente arranque.
func ClearMem0APIKey() error {
	directory, err := Mem0Directory()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, mem0APIKeyFile), nil, 0600)
}

// loadOrCreateMem0UserID da a mem0 un identificador de usuario opaco y
// estable por instalación. No usa ningún dato personal (como un email): es
// un UUID generado una sola vez y guardado junto al resto de la config.
func loadOrCreateMem0UserID(directory string) (string, error) {
	path := filepath.Join(directory, "mem0_user_id")
	if existing, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(existing)); id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	id := uuid.NewString()
	if err := os.WriteFile(path, []byte(id+"\n"), 0600); err != nil {
		return "", err
	}
	return id, nil
}
