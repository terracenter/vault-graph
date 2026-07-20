package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config contiene la configuración global
type Config struct {
	DatabaseURL string
	OllamaURL   string
	VaultPath   string
}

// LoadConfig carga la configuración desde variables de entorno
// Busca .env en el directorio actual (cargar manualmente si es necesario)
func LoadConfig() (*Config, error) {
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
