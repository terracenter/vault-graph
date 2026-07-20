package parser

import (
	"regexp"
	"strings"
)

type RawLink struct {
	Target string // el enlace crudo [[...]]
	Alias  string // opcional, texto después de |
}

var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// ExtractWikilinks extrae todos los wikilinks [[...]] de un texto
// Retorna una lista de RawLink con target y alias opcional
func ExtractWikilinks(body string) []RawLink {
	matches := wikiLinkRegex.FindAllStringSubmatch(body, -1)
	links := make([]RawLink, 0)

	for _, match := range matches {
		// match[0] = [[...]] completo
		// match[1] = target
		// match[2] = alias (puede estar vacío)
		target := strings.TrimSpace(match[1])
		alias := ""
		if len(match) > 2 && match[2] != "" {
			alias = strings.TrimSpace(match[2])
		}
		links = append(links, RawLink{
			Target: target,
			Alias:  alias,
		})
	}
	return links
}
