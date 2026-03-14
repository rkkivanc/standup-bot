package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const defaultOllamaBaseURL = "http://localhost:11434"

// OllamaMessage represents a single message in the Ollama chat format.
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaChatRequest is the request body sent to Ollama's /api/chat endpoint.
type OllamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// OllamaChatResponse is the response body returned by Ollama's /api/chat endpoint.
type OllamaChatResponse struct {
	Model     string        `json:"model"`
	CreatedAt string        `json:"created_at"`
	Message   OllamaMessage `json:"message"`
	Done      bool          `json:"done"`
}

// OllamaService handles communication with a locally running Ollama instance.
type OllamaService struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllamaService creates a new OllamaService. It reads OLLAMA_BASE_URL from
// the environment and falls back to http://localhost:11434 when it is not set.
func NewOllamaService() *OllamaService {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	return &OllamaService{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Chat sends a chat request to Ollama and returns the assistant's reply.
// It uses the Ollama /api/chat endpoint (not the OpenAI-compatible endpoint).
func (s *OllamaService) Chat(ctx context.Context, model string, messages []OllamaMessage) (*OllamaChatResponse, error) {
	if model == "" {
		model = "llama3"
	}

	payload := OllamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(raw))
	}

	var result OllamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	return &result, nil
}
