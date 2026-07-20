package parser

import (
	"regexp"
	"strings"
)

var inlineTagRegex = regexp.MustCompile(`#([a-zA-Z0-9_-]+)`)

// ExtractTags extrae tags del frontmatter (array tags:) e inline (#tag)
// Devuelve lista deduplicated de tags
func ExtractTags(fm map[string]any, body string) []string {
	tagSet := make(map[string]bool)

	// Tags del frontmatter
	fmTags := GetStringSlice(fm, "tags")
	for _, tag := range fmTags {
		tagSet[tag] = true
	}

	// Tags inline (fuera de bloques de código)
	inlineTags := extractInlineTags(body)
	for _, tag := range inlineTags {
		tagSet[tag] = true
	}

	// Convertir a slice
	result := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		result = append(result, tag)
	}
	return result
}

// extractInlineTags extrae tags inline (#tag) excluyendo bloques de código
func extractInlineTags(body string) []string {
	// Splits por bloques ``` ... ```
	parts := strings.Split(body, "```")
	var tags []string

	// Procesar solo las partes no-código (índices pares)
	for i := 0; i < len(parts); i += 2 {
		matches := inlineTagRegex.FindAllStringSubmatch(parts[i], -1)
		for _, match := range matches {
			// match[1] = el tag sin #
			tags = append(tags, match[1])
		}
	}

	return tags
}
