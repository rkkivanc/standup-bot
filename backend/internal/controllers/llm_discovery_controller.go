package controllers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"workshop-backend/internal/services"
)

type llmProvidersResponse struct {
	Providers []services.LLMProvider `json:"providers"`
}

type llmConnectRequest struct {
	Endpoint string `json:"endpoint"`
}

type llmConnectResponse struct {
	ActiveEndpoint string `json:"active_endpoint"`
}

// HandleLLMProviders handles GET /api/llm/providers.
// It returns the list of detected local AI providers.
func HandleLLMProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	providers := services.DiscoverLLMProviders(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(llmProvidersResponse{
		Providers: providers,
	})
}

// HandleLLMConnect handles POST /api/llm/connect.
// It accepts a JSON body with an "endpoint" string and sets it as the active LLM.
func HandleLLMConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()

	var req llmConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}

	services.SetActiveLLMEndpoint(req.Endpoint)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(llmConnectResponse{
		ActiveEndpoint: req.Endpoint,
	})
}

// ollamaPullRequest is the request body for Ollama's /api/pull endpoint.
type ollamaPullRequest struct {
	Name   string `json:"name"`
	Stream bool   `json:"stream"`
}

// ollamaPullProgress represents a single progress line from Ollama's /api/pull.
type ollamaPullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

const defaultModel = "gemma3:1b"

// HandleLLMDownload handles POST /api/llm/download.
// It pulls the recommended model (gemma3:1b) in Ollama via its /api/pull
// endpoint and streams progress as SSE to the frontend.
func HandleLLMDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ollamaEndpoint := services.OllamaHost()

	// Step 1: Wait for Ollama to be ready
	writeSSEDownload(w, flusher, 5, "Waiting for Ollama to be ready...")

	ready := waitForOllama(r, ollamaEndpoint, w, flusher)
	if !ready {
		return
	}

	writeSSEDownload(w, flusher, 10, "Ollama is ready. Pulling model "+defaultModel+"...")

	// Step 2: Pull the model via Ollama /api/pull (streaming)
	pullURL := strings.TrimRight(ollamaEndpoint, "/") + "/api/pull"
	pullBody, _ := json.Marshal(ollamaPullRequest{
		Name:   defaultModel,
		Stream: true,
	})

	pullReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, pullURL, bytes.NewReader(pullBody))
	if err != nil {
		writeSSEDownload(w, flusher, 0, "Failed to create pull request: "+err.Error())
		return
	}
	pullReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Minute}
	pullResp, err := client.Do(pullReq)
	if err != nil {
		writeSSEDownload(w, flusher, 0, "Failed to connect to Ollama for pull: "+err.Error())
		return
	}
	defer pullResp.Body.Close()

	if pullResp.StatusCode != http.StatusOK {
		writeSSEDownload(w, flusher, 0, fmt.Sprintf("Ollama pull returned status %d", pullResp.StatusCode))
		return
	}

	// Read streaming NDJSON from Ollama and relay as SSE
	scanner := bufio.NewScanner(pullResp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			writeSSEDownload(w, flusher, 0, "Download cancelled.")
			return
		default:
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var progress ollamaPullProgress
		if err := json.Unmarshal([]byte(line), &progress); err != nil {
			continue
		}

		pct := 10
		msg := progress.Status
		if progress.Total > 0 && progress.Completed > 0 {
			pct = 10 + int(float64(progress.Completed)/float64(progress.Total)*85)
			msg = fmt.Sprintf("%s (%.0f MB / %.0f MB)",
				progress.Status,
				float64(progress.Completed)/1024/1024,
				float64(progress.Total)/1024/1024,
			)
		}

		writeSSEDownload(w, flusher, pct, msg)
	}

	// Step 3: Done - set active endpoint and connect
	writeSSEDownload(w, flusher, 98, "Model pulled successfully. Connecting...")

	services.SetActiveLLMEndpoint(ollamaEndpoint)

	writeSSEDownload(w, flusher, 100, "Ready")
	time.Sleep(200 * time.Millisecond)
}

// waitForOllama polls Ollama until it responds or the request is cancelled.
func waitForOllama(r *http.Request, ollamaEndpoint string, w http.ResponseWriter, flusher http.Flusher) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	tagsURL := strings.TrimRight(ollamaEndpoint, "/") + "/api/tags"

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Try immediately first
	if probeOllama(r.Context(), client, tagsURL) {
		return true
	}

	for i := 0; i < 30; i++ {
		select {
		case <-r.Context().Done():
			writeSSEDownload(w, flusher, 0, "Cancelled while waiting for Ollama.")
			return false
		case <-ticker.C:
			if probeOllama(r.Context(), client, tagsURL) {
				return true
			}
			writeSSEDownload(w, flusher, 5, fmt.Sprintf("Waiting for Ollama... (%ds)", (i+1)*2))
		}
	}

	writeSSEDownload(w, flusher, 0, "Timed out waiting for Ollama to start.")
	return false
}

func probeOllama(ctx context.Context, client *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type sseDownloadPayload struct {
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

func writeSSEDownload(w http.ResponseWriter, flusher http.Flusher, progress int, msg string) {
	payload, err := json.Marshal(sseDownloadPayload{Progress: progress, Message: msg})
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()
}

type sseProgress struct {
	Progress string `json:"progress"`
}

func writeSSEProgress(w http.ResponseWriter, flusher http.Flusher, msg string) {
	payload, err := json.Marshal(sseProgress{Progress: msg})
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()
}

func writeSSEError(w http.ResponseWriter, flusher http.Flusher, msg string) {
	writeSSEProgress(w, flusher, msg)
}
