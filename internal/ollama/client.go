package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client gestiona las llamadas a Ollama con batching paralelo
type Client struct {
	baseURL      string
	model        string
	httpClient   *http.Client
	maxWorkers   int
	requestTimeout time.Duration
}

// GenerateRequest es el request a /api/generate
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// GenerateResponse es la respuesta de /api/generate
type GenerateResponse struct {
	Response string `json:"response"`
}

// EnrichResult contiene el resultado del enriquecimiento de un nodo
type EnrichResult struct {
	Path      string
	Summary   string
	Success   bool
	Error     error
}

// NewClient crea un nuevo cliente Ollama
func NewClient(baseURL, model string, maxWorkers int) *Client {
	return &Client{
		baseURL:      baseURL,
		model:        model,
		maxWorkers:   maxWorkers,
		requestTimeout: 10 * time.Second,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CheckHealth verifica que el servidor Ollama está disponible
func (c *Client) CheckHealth(ctx context.Context) error {
	url := c.baseURL + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama server not reachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama server returned status %d", resp.StatusCode)
	}

	return nil
}

// GenerateSummary genera un resumen de un contenido usando Ollama
func (c *Client) GenerateSummary(ctx context.Context, content string) (string, error) {
	// Limitar el contenido a ~500 caracteres para no sobrecargar el prompt
	if len(content) > 500 {
		content = content[:500]
	}

	prompt := fmt.Sprintf(
		"Resume en una o dos líneas breves lo siguiente:\n\n%s\n\nResumen:",
		content,
	)

	reqBody := GenerateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to ollama failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var respBody GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return respBody.Response, nil
}

// EnrichBatch enriquece múltiples contenidos en paralelo con worker pool
// Retorna slice de resultados en el mismo orden que los inputs
func (c *Client) EnrichBatch(ctx context.Context, items []struct{ Path, Content string }) []EnrichResult {
	results := make([]EnrichResult, len(items))
	if len(items) == 0 {
		return results
	}

	// Canal de trabajos y resultados
	jobs := make(chan int, len(items))
	results_ch := make(chan EnrichResult, len(items))
	var wg sync.WaitGroup

	// Crear workers
	numWorkers := c.maxWorkers
	if numWorkers > len(items) {
		numWorkers = len(items)
	}

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for idx := range jobs {
				item := items[idx]
				summary, err := c.GenerateSummary(ctx, item.Content)
				results_ch <- EnrichResult{
					Path:    item.Path,
					Summary: summary,
					Success: err == nil,
					Error:   err,
				}
			}
		}()
	}

	// Enviar trabajos
	go func() {
		for i := 0; i < len(items); i++ {
			jobs <- i
		}
		close(jobs)
	}()

	// Recolectar resultados en gorutina separada
	go func() {
		wg.Wait()
		close(results_ch)
	}()

	// Mapear resultados al índice correcto
	resultMap := make(map[string]EnrichResult)
	for result := range results_ch {
		resultMap[result.Path] = result
	}

	for i, item := range items {
		if res, ok := resultMap[item.Path]; ok {
			results[i] = res
		} else {
			results[i] = EnrichResult{
				Path:    item.Path,
				Success: false,
				Error:   fmt.Errorf("no result returned"),
			}
		}
	}

	return results
}
