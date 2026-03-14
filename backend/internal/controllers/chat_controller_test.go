package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"workshop-backend/internal/controllers"
	"workshop-backend/internal/services"
)

func TestChatController_HandleChat_Success(t *testing.T) {
	// Stand up a fake Ollama server.
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := services.OllamaChatResponse{
			Model:   "llama3",
			Message: services.OllamaMessage{Role: "assistant", Content: "Sure, I can help!"},
			Done:    true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ollamaServer.Close()

	t.Setenv("OLLAMA_BASE_URL", ollamaServer.URL)

	ollama := services.NewOllamaService()
	ctrl := controllers.NewChatController(ollama)

	body, _ := json.Marshal(map[string]any{
		"model":    "llama3",
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	ctrl.HandleChat(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	msg, ok := resp["message"].(map[string]any)
	if !ok {
		t.Fatal("expected message field in response")
	}
	if msg["role"] != "assistant" {
		t.Errorf("expected assistant role, got %v", msg["role"])
	}
}

func TestChatController_HandleChat_WrongMethod(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://localhost:99999") // should not be called

	ctrl := controllers.NewChatController(services.NewOllamaService())
	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)
	rr := httptest.NewRecorder()

	ctrl.HandleChat(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestChatController_HandleChat_EmptyMessages(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://localhost:99999") // should not be called

	ctrl := controllers.NewChatController(services.NewOllamaService())

	body, _ := json.Marshal(map[string]any{
		"model":    "llama3",
		"messages": []map[string]string{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	ctrl.HandleChat(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestChatController_HandleChat_InvalidBody(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://localhost:99999") // should not be called

	ctrl := controllers.NewChatController(services.NewOllamaService())

	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	ctrl.HandleChat(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
