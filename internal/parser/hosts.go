package parser

import (
	"regexp"
	"strings"
)

// Regex idéntico a extract_wiki.py:18
// Captura dominios con TLD allowlist dentro de backticks
var hostRegex = regexp.MustCompile("`((?:[a-z0-9-]+\\.)+(?:net|com|org|io|dev|local|lan|internal))`")

// ExtractHosts extrae hosts/dominios del cuerpo (dentro de backticks)
// Devuelve lista deduplicated de hosts
func ExtractHosts(body string) []string {
	matches := hostRegex.FindAllStringSubmatch(body, -1)
	hostSet := make(map[string]bool)

	for _, match := range matches {
		// match[1] = el dominio sin backticks
		host := strings.ToLower(strings.TrimSpace(match[1]))
		if host != "" {
			hostSet[host] = true
		}
	}

	// Convertir a slice
	result := make([]string, 0, len(hostSet))
	for host := range hostSet {
		result = append(result, host)
	}
	return result
}
