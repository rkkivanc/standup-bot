package routes

import (
	"net/http"

	"workshop-backend/internal/controllers"
)

// RegisterRoutes wires all HTTP routes to their handlers.
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/commits", controllers.HandleCommits)
	mux.HandleFunc("/api/standup", controllers.HandleStandup)
	mux.HandleFunc("/api/llm/providers", controllers.HandleLLMProviders)
	mux.HandleFunc("/api/llm/connect", controllers.HandleLLMConnect)
	mux.HandleFunc("/api/llm/download", controllers.HandleLLMDownload)
	mux.HandleFunc("/api/chat", controllers.HandleChat)
}

