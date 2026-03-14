package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const defaultStandupModel = "gemma3:1b"

const standupSystemPrompt = "You are a developer standup assistant. Given a list of git commit messages, extract and return ONLY a JSON object with keys: yesterday, today, blockers. Each value is an array of concise strings. No explanation. No markdown."

// StandupSummary represents the structured standup output.
type StandupSummary struct {
	Yesterday  []string `json:"yesterday"`
	Today      []string `json:"today"`
	Blockers   []string `json:"blockers"`
	RawSummary string   `json:"raw_summary"`
}

type standupJSON struct {
	Yesterday []string `json:"yesterday"`
	Today     []string `json:"today"`
	Blockers  []string `json:"blockers"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// GenerateStandupSummary takes a slice of commits and returns a structured
// standup summary. It first attempts to call the local mlc-llm endpoint; if the
// response is malformed JSON or the call fails/times out, it falls back to a
// simple keyword-based parser.
func GenerateStandupSummary(ctx context.Context, commits []Commit) StandupSummary {
	if len(commits) == 0 {
		return StandupSummary{
			Yesterday:  []string{},
			Today:      []string{},
			Blockers:   []string{},
			RawSummary: "",
		}
	}

	summary, ok := generateViaLLM(ctx, commits)
	if ok {
		return summary
	}

	return generateViaKeywords(commits)
}

func generateViaLLM(ctx context.Context, commits []Commit) (StandupSummary, bool) {
	activeEndpoint := GetActiveLLMEndpoint()
	if strings.TrimSpace(activeEndpoint) == "" {
		// Fallback to Ollama host if no active endpoint set
		activeEndpoint = OllamaHost()
	}

	messages := make([]string, 0, len(commits))
	for _, c := range commits {
		if strings.TrimSpace(c.Message) == "" {
			continue
		}
		messages = append(messages, "- "+c.Message)
	}

	if len(messages) == 0 {
		return StandupSummary{
			Yesterday:  []string{},
			Today:      []string{},
			Blockers:   []string{},
			RawSummary: "",
		}, true
	}

	userContent := strings.Join(messages, "\n")

	reqBody := chatRequest{
		Model: defaultStandupModel,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: standupSystemPrompt,
			},
			{
				Role:    "user",
				Content: userContent,
			},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return StandupSummary{}, false
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	endpoint := strings.TrimRight(activeEndpoint, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return StandupSummary{}, false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return StandupSummary{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return StandupSummary{}, false
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return StandupSummary{}, false
	}

	if len(cr.Choices) == 0 {
		return StandupSummary{}, false
	}

	content := strings.TrimSpace(cr.Choices[0].Message.Content)
	if content == "" {
		return StandupSummary{}, false
	}

	var parsed standupJSON
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return StandupSummary{}, false
	}

	// Ensure slices are non-nil for JSON responses.
	if parsed.Yesterday == nil {
		parsed.Yesterday = []string{}
	}
	if parsed.Today == nil {
		parsed.Today = []string{}
	}
	if parsed.Blockers == nil {
		parsed.Blockers = []string{}
	}

	return StandupSummary{
		Yesterday:  parsed.Yesterday,
		Today:      parsed.Today,
		Blockers:   parsed.Blockers,
		RawSummary: content,
	}, true
}

func generateViaKeywords(commits []Commit) StandupSummary {
	yesterday := make([]string, 0, len(commits))
	today := make([]string, 0, len(commits))
	blockers := make([]string, 0, len(commits))

	blockerKeywords := []string{"fix", "bug", "broken", "fail", "revert", "hotfix", "todo", "wip"}

	for _, c := range commits {
		msg := strings.TrimSpace(c.Message)
		if msg == "" {
			continue
		}

		lower := strings.ToLower(msg)

		isToday := strings.Contains(lower, "wip") || strings.Contains(lower, "todo")

		isBlocker := false
		for _, kw := range blockerKeywords {
			if strings.Contains(lower, kw) {
				isBlocker = true
				break
			}
		}

		if isBlocker {
			blockers = append(blockers, msg)
		}

		if isToday {
			today = append(today, msg)
		} else if !isBlocker {
			yesterday = append(yesterday, msg)
		}
	}

	return StandupSummary{
		Yesterday:  yesterday,
		Today:      today,
		Blockers:   blockers,
		RawSummary: "",
	}
}

