package routes

import (
	"net/http"

	"workshop-backend/internal/controllers"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RegisterRoutes(mux *http.ServeMux) http.Handler {
	mux.HandleFunc("/api/commits", controllers.HandleCommits)
	mux.HandleFunc("/api/standup", controllers.HandleStandup)
	mux.HandleFunc("/api/llm/providers", controllers.HandleLLMProviders)
	mux.HandleFunc("/api/llm/connect", controllers.HandleLLMConnect)
	mux.HandleFunc("/api/llm/download", controllers.HandleLLMDownload)
	mux.HandleFunc("/api/chat", controllers.HandleChat)
	return corsMiddleware(mux)
}

