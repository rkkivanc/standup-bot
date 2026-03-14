package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"workshop-backend/internal/services"
)

const chatSystemPrompt = "You are a helpful assistant. You have access to the developer's standup summary provided below. Answer questions only based on this context. Be concise."

type chatContext struct {
	Yesterday []string `json:"yesterday"`
	Today     []string `json:"today"`
	Blockers  []string `json:"blockers"`
}

type chatRequestBody struct {
	Message string      `json:"message"`
	Context chatContext `json:"context"`
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type sseToken struct {
	Token string `json:"token"`
}

const defaultChatModel = "gemma3:1b"

// chatModelsResponse describes a minimal OpenAI-compatible /v1/models response.
type chatModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// HandleChat handles POST /api/chat.
// It accepts a message and standup context, calls the active local LLM endpoint,
// and streams the response back as Server-Sent Events (SSE) with one token per event.
func HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	activeEndpoint := services.GetActiveLLMEndpoint()
	if strings.TrimSpace(activeEndpoint) == "" {
		writeError(w, http.StatusBadRequest, "no active LLM endpoint configured")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	defer r.Body.Close()

	var reqBody chatRequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if strings.TrimSpace(reqBody.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Build the system message with the standup context embedded.
	systemContent := buildSystemContent(reqBody.Context)

	// Resolve the concrete model ID from the LLM's /v1/models endpoint.
	modelID := resolveChatModel(r.Context(), client, activeEndpoint)

	llmReq := chatCompletionRequest{
		Model: modelID,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: systemContent,
			},
			{
				Role:    "user",
				Content: reqBody.Message,
			},
		},
		Stream: false,
	}

	data, err := json.Marshal(llmReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode LLM request")
		return
	}

	url := strings.TrimRight(activeEndpoint, "/") + "/v1/chat/completions"

	llmHTTPReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create LLM request")
		return
	}
	llmHTTPReq.Header.Set("Content-Type", "application/json")

	llmResp, err := client.Do(llmHTTPReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to call LLM: %v", err))
		return
	}
	defer llmResp.Body.Close()

	if llmResp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("LLM returned status %d", llmResp.StatusCode))
		return
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(llmResp.Body).Decode(&completion); err != nil {
		writeError(w, http.StatusBadGateway, "failed to decode LLM response")
		return
	}

	if len(completion.Choices) == 0 {
		writeError(w, http.StatusBadGateway, "LLM returned no choices")
		return
	}

	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		writeError(w, http.StatusBadGateway, "LLM returned empty content")
		return
	}

	// Start SSE streaming of the content as tokens.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	streamTokensAsSSE(w, flusher, content)
}

func buildSystemContent(ctx chatContext) string {
	var b strings.Builder
	b.WriteString(chatSystemPrompt)
	b.WriteString("\n\nStandup summary:\n")

	if len(ctx.Yesterday) > 0 {
		b.WriteString("Yesterday:\n")
		for _, item := range ctx.Yesterday {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}

	if len(ctx.Today) > 0 {
		b.WriteString("Today:\n")
		for _, item := range ctx.Today {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}

	if len(ctx.Blockers) > 0 {
		b.WriteString("Blockers:\n")
		for _, item := range ctx.Blockers {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// resolveChatModel queries the LLM's /v1/models endpoint and returns the first
// available model ID. If anything fails, it falls back to defaultChatModel.
func resolveChatModel(ctx context.Context, client *http.Client, baseEndpoint string) string {
	url := strings.TrimRight(baseEndpoint, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return defaultChatModel
	}

	resp, err := client.Do(req)
	if err != nil {
		return defaultChatModel
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return defaultChatModel
	}

	var mr chatModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return defaultChatModel
	}

	if len(mr.Data) == 0 || strings.TrimSpace(mr.Data[0].ID) == "" {
		return defaultChatModel
	}

	return mr.Data[0].ID
}

func streamTokensAsSSE(w http.ResponseWriter, flusher http.Flusher, content string) {
	tokens := splitContentIntoTokens(content)
	for _, t := range tokens {
		payload, err := json.Marshal(sseToken{Token: t})
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func splitContentIntoTokens(content string) []string {
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return nil
	}

	// Re-add spaces between tokens so the client can reconstruct the text easily.
	tokens := make([]string, 0, len(parts))
	for i, p := range parts {
		if i == len(parts)-1 {
			tokens = append(tokens, p)
		} else {
			tokens = append(tokens, p+" ")
		}
	}
	return tokens
}

