package services

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"
)

// LLMProvider represents a local AI provider and its status.
type LLMProvider struct {
	Name        string   `json:"name"`
	Endpoint    string   `json:"endpoint"`
	Status      string   `json:"status"` // "running" | "not_found"
	Models      []string `json:"models"`
	Recommended bool     `json:"recommended"`
}

const (
	statusRunning  = "running"
	statusNotFound = "not_found"
)

// activeLLMEndpoint holds the currently selected local LLM endpoint in-memory.
// It is process-wide and not persisted.
var (
	activeLLMEndpoint string
	activeMu          sync.RWMutex
)

// SetActiveLLMEndpoint sets the currently active local LLM endpoint.
func SetActiveLLMEndpoint(endpoint string) {
	activeMu.Lock()
	defer activeMu.Unlock()
	activeLLMEndpoint = endpoint
}

// GetActiveLLMEndpoint returns the currently active local LLM endpoint.
func GetActiveLLMEndpoint() string {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeLLMEndpoint
}

// OllamaHost returns the Ollama base URL. Inside Docker this is
// http://ollama:11434 (set via OLLAMA_HOST env var); outside Docker
// it falls back to http://localhost:11434.
func OllamaHost() string {
	if h := os.Getenv("OLLAMA_HOST"); h != "" {
		return h
	}
	return "http://localhost:11434"
}

// DiscoverLLMProviders probes a fixed set of known local AI endpoints concurrently
// and returns their status. Each probe has an 800ms timeout.
func DiscoverLLMProviders(ctx context.Context) []LLMProvider {
	oHost := OllamaHost()

	providers := []LLMProvider{
		{
			Name:        "Ollama",
			Endpoint:    oHost,
			Status:      statusNotFound,
			Models:      []string{},
			Recommended: true,
		},
		{
			Name:        "LM Studio",
			Endpoint:    "http://localhost:1234",
			Status:      statusNotFound,
			Models:      []string{},
			Recommended: false,
		},
		{
			Name:        "LocalAI",
			Endpoint:    "http://localhost:8081",
			Status:      statusNotFound,
			Models:      []string{},
			Recommended: false,
		},
	}

	type probeConfig struct {
		index int
		url   string
		kind  string
	}

	probes := []probeConfig{
		{index: 0, url: oHost + "/api/tags", kind: "ollama"},
		{index: 1, url: "http://localhost:1234/v1/models", kind: "openai"},
		{index: 2, url: "http://localhost:8081/v1/models", kind: "openai"},
	}

	var wg sync.WaitGroup
	client := &http.Client{
		Timeout: 800 * time.Millisecond,
	}

	for _, p := range probes {
		wg.Add(1)
		go func(p probeConfig) {
			defer wg.Done()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
			if err != nil {
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return
			}

			providers[p.index].Status = statusRunning

			switch p.kind {
			case "ollama":
				var o ollamaTagsResponse
				if err := json.NewDecoder(resp.Body).Decode(&o); err == nil {
					models := make([]string, 0, len(o.Models))
					for _, m := range o.Models {
						if m.Name != "" {
							models = append(models, m.Name)
						}
					}
					providers[p.index].Models = models
				}
			case "openai":
				var m modelsResponse
				if err := json.NewDecoder(resp.Body).Decode(&m); err == nil {
					models := make([]string, 0, len(m.Data))
					for _, md := range m.Data {
						if md.ID != "" {
							models = append(models, md.ID)
						}
					}
					providers[p.index].Models = models
				}
			}
		}(p)
	}

	wg.Wait()

	return providers
}

// modelsResponse describes a minimal OpenAI-compatible /v1/models response.
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ollamaTagsResponse describes the shape of Ollama's /api/tags response.
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}
