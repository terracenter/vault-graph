package parser

import (
	"strings"
	"gopkg.in/yaml.v3"
)

// ParseFrontmatter extrae el bloque YAML inicial (si existe) y retorna {frontmatter, body}
func ParseFrontmatter(content string) (map[string]any, string) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		// Sin frontmatter válido
		return map[string]any{}, content
	}

	// Buscar cierre del frontmatter
	var endIdx int
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == 0 {
		// No cerró el frontmatter
		return map[string]any{}, content
	}

	fmContent := strings.Join(lines[1:endIdx], "\n")
	body := strings.Join(lines[endIdx+1:], "\n")

	fm := make(map[string]any)
	if err := yaml.Unmarshal([]byte(fmContent), fm); err != nil {
		// Fallback si hay error en YAML
		return map[string]any{}, content
	}

	return fm, body
}

// GetString obtiene un valor string del frontmatter
func GetString(fm map[string]any, key string) string {
	if v, ok := fm[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetStringSlice obtiene un slice de strings del frontmatter
func GetStringSlice(fm map[string]any, key string) []string {
	if v, ok := fm[key]; ok {
		if slice, ok := v.([]any); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return []string{}
}
