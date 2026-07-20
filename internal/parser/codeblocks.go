package parser

import (
	"regexp"
	"strings"
)

// StripCode remueve tanto fences (``` ... ```) como inline code spans (` ... `)
// Reemplaza con espacio para no pegar palabras adyacentes
func StripCode(body string) string {
	// Remover fences ``` ... ```
	// Estrategia: split por ``` y tomar solo los índices pares (contenido, no código)
	parts := strings.Split(body, "```")
	var result []string
	for i := 0; i < len(parts); i += 2 {
		result = append(result, parts[i])
	}
	stripped := strings.Join(result, " ")

	// Remover inline code spans ` ... `
	// Regex: backtick, uno o más caracteres no-backtick, backtick
	codeSpanRegex := regexp.MustCompile("`[^`]+`")
	stripped = codeSpanRegex.ReplaceAllString(stripped, " ")

	return stripped
}
