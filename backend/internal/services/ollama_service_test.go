package services_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"workshop-backend/internal/services"
)

func TestOllamaService_Chat_Success(t *testing.T) {
	// Simulate an Ollama /api/chat response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Verify the request body matches the Ollama /api/chat format.
		var req services.OllamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "llama3" {
			t.Errorf("expected model llama3, got %s", req.Model)
		}
		if req.Stream {
			t.Error("stream should be false for non-streaming chat")
		}
		if len(req.Messages) == 0 {
			t.Error("expected at least one message")
		}

		// Return a valid Ollama response.
		resp := services.OllamaChatResponse{
			Model: "llama3",
			Message: services.OllamaMessage{
				Role:    "assistant",
				Content: "Hello! How can I help you today?",
			},
			Done: true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("OLLAMA_BASE_URL", server.URL)

	svc := services.NewOllamaService()
	result, err := svc.Chat(context.Background(), "llama3", []services.OllamaMessage{
		{Role: "user", Content: "Hello!"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message.Role != "assistant" {
		t.Errorf("expected assistant role, got %s", result.Message.Role)
	}
	if result.Message.Content == "" {
		t.Error("expected non-empty response content")
	}
}

func TestOllamaService_Chat_UsesDefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req services.OllamaChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Model != "llama3" {
			t.Errorf("expected default model llama3, got %s", req.Model)
		}

		resp := services.OllamaChatResponse{
			Model:   "llama3",
			Message: services.OllamaMessage{Role: "assistant", Content: "ok"},
			Done:    true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("OLLAMA_BASE_URL", server.URL)

	svc := services.NewOllamaService()
	// Pass empty model to trigger the default.
	_, err := svc.Chat(context.Background(), "", []services.OllamaMessage{
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOllamaService_Chat_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("OLLAMA_BASE_URL", server.URL)

	svc := services.NewOllamaService()
	_, err := svc.Chat(context.Background(), "unknown-model", []services.OllamaMessage{
		{Role: "user", Content: "hi"},
	})
	if err == nil {
		t.Fatal("expected an error for non-200 status, got nil")
	}
}
