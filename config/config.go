package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config contiene la configuración global.
// Desde 2026-08-12 el CLI es 100% Kuzu, así que solo se requieren
// KuzuPath, OllamaURL y VaultPath. DatabaseURL se eliminó.
type Config struct {
	OllamaURL   string
	VaultPath   string
	KuzuPath    string
	Backend     string
	DatabaseURL string
}

// Load carga la configuración desde .env y variables de entorno.
func Load() (*Config, error) {
	_ = loadDotenv(".env")

	cfg := &Config{
		OllamaURL:   os.Getenv("OLLAMA_URL"),
		VaultPath:   os.Getenv("VAULT_PATH"),
		KuzuPath:    os.Getenv("KUZU_PATH"),
		Backend:     os.Getenv("VG_BACKEND"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	if cfg.Backend == "" {
		cfg.Backend = "kuzu"
	}

	if cfg.Backend == "age" && cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("backend age requiere DATABASE_URL")
	}
	if cfg.Backend == "kuzu" && cfg.KuzuPath == "" {
		return nil, fmt.Errorf("backend kuzu requiere KUZU_PATH")
	}


	if cfg.VaultPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.VaultPath = filepath.Join(home, "Workspace/Obsidian")
	}

	if _, err := os.Stat(cfg.VaultPath); err != nil {
		return nil, fmt.Errorf("vault path does not exist: %s", cfg.VaultPath)
	}

	return cfg, nil
}

func loadDotenv(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}