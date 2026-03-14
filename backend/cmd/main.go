package main

import (
	"log"
	"net/http"
	"os"

	"workshop-backend/internal/controllers"
	"workshop-backend/internal/services"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ollamaSvc := services.NewOllamaService()
	githubSvc := services.NewGitHubService()

	chatCtrl := controllers.NewChatController(ollamaSvc)
	commitsCtrl := controllers.NewCommitsController(githubSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", chatCtrl.HandleChat)
	mux.HandleFunc("/api/commits", commitsCtrl.HandleCommits)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	log.Printf("backend listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
