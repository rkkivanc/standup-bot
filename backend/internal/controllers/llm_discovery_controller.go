package controllers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
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

// HandleLLMDownload handles POST /api/llm/download.
// It runs "mlc_llm serve" as a subprocess and streams its stdout as
// Server-Sent Events (SSE) to the client, with each line wrapped as:
//   data: {"progress": "..."}
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

	cmd := exec.CommandContext(r.Context(), "mlc_llm", "serve")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeSSEError(w, flusher, fmt.Sprintf("failed to create stdout pipe: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		writeSSEError(w, flusher, fmt.Sprintf("failed to start mlc_llm: %v", err))
		return
	}

	writeSSEProgress(w, flusher, "Starting mlc_llm serve...")

	scanner := bufio.NewScanner(stdout)
	// Increase the scanner buffer in case lines are long.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		writeSSEProgress(w, flusher, line)
	}

	if err := scanner.Err(); err != nil {
		writeSSEError(w, flusher, fmt.Sprintf("stream error: %v", err))
	}

	// Wait for the command to exit.
	err = cmd.Wait()
	if err != nil {
		writeSSEError(w, flusher, fmt.Sprintf("mlc_llm exited with error: %v", err))
	} else {
		writeSSEProgress(w, flusher, "mlc_llm serve completed")
	}

	// Give clients a short window to receive the final events.
	time.Sleep(200 * time.Millisecond)
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

