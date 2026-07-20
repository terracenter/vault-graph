package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config contiene la configuración global
type Config struct {
	DatabaseURL string
	OllamaURL   string
	VaultPath   string
}

// Load carga la configuración desde .env y variables de entorno
func Load() (*Config, error) {
	// Cargar .env si existe
	_ = loadDotenv(".env")

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		OllamaURL:   os.Getenv("OLLAMA_URL"),
		VaultPath:   os.Getenv("VAULT_PATH"),
	}

	// Validaciones
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL not set in environment")
	}

	if cfg.VaultPath == "" {
		// Fallback: asumir que el vault está en ~/Workspace/Obsidian
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.VaultPath = filepath.Join(home, "Workspace/Obsidian")
	}

	// Validar que el vault existe
	if _, err := os.Stat(cfg.VaultPath); err != nil {
		return nil, fmt.Errorf("vault path does not exist: %s", cfg.VaultPath)
	}

	return cfg, nil
}

// loadDotenv carga las variables de entorno desde un archivo .env
func loadDotenv(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		// .env es opcional, no error si no existe
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Ignorar comentarios y líneas vacías
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parsear KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remover quotes si existen
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}
