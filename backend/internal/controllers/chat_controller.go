package controllers

import (
	"encoding/json"
	"net/http"

	"workshop-backend/internal/services"
)

// ChatController handles POST /api/chat requests by forwarding them
// to the locally running Ollama instance using the Ollama /api/chat format.
type ChatController struct {
	ollama *services.OllamaService
}

// NewChatController creates a ChatController with the given OllamaService.
func NewChatController(ollama *services.OllamaService) *ChatController {
	return &ChatController{ollama: ollama}
}

// chatRequest is the JSON body expected from the frontend.
type chatRequest struct {
	Model    string                  `json:"model"`
	Messages []services.OllamaMessage `json:"messages"`
}

// chatResponse is the JSON body returned to the frontend.
type chatResponse struct {
	Message services.OllamaMessage `json:"message"`
	Model   string                 `json:"model"`
}

// HandleChat is the HTTP handler for POST /api/chat.
func (c *ChatController) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages must not be empty")
		return
	}

	result, err := c.ollama.Chat(r.Context(), req.Model, req.Messages)
	if err != nil {
		writeError(w, http.StatusBadGateway, "ollama request failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Message: result.Message,
		Model:   result.Model,
	})
}
