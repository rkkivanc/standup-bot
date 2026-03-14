# Standup Bot — stories.md

## Project Overview
A privacy-first developer standup assistant. Fetches GitHub commits locally,
summarizes them using an on-device LLM (mlc-llm), and provides a chat interface
for follow-up questions — all without sending private data to any external service.

**Stack:** Go (backend) · Next.js (frontend) · mlc-llm (local LLM) · Docker · PostgreSQL

---

## Epic 1: GitHub Data Pipeline

### Story 1: Fetch Last 24h Commits from GitHub API
**As a** developer,
**I want to** enter my GitHub token and repo name,
**so that** the system fetches my last 24 hours of commits without storing them externally.

**Acceptance Criteria:**
- [ ] User provides: `github_token`, `owner`, `repo` via POST `/api/commits`
- [ ] Backend calls `https://api.github.com/repos/{owner}/{repo}/commits`
       with `since` param set to 24 hours ago (UTC)
- [ ] Response includes: `sha`, `message`, `author.name`, `commit.author.date`
- [ ] If token is invalid or repo not found → return structured error `{ "error": "...", "code": 401 | 404 }`
- [ ] GitHub token is NEVER logged or persisted — used only in-memory per request

**Technical Notes:**
- Handler: `backend/internal/controllers/commits_controller.go`
- Service: `backend/internal/services/github_service.go`
- Use `net/http` with `Authorization: Bearer {token}` header
- Filter commits in Go, not in the GitHub API query, to keep the logic testable

---

### Story 2: Parse Commits into Standup Format
**As a** developer,
**I want** my raw commit messages automatically categorized,
**so that** I get a structured Yesterday / Today / Blockers standup summary.

**Acceptance Criteria:**
- [ ] POST `/api/standup` accepts the commit array from Story 1
- [ ] Passes commits to local mlc-llm with a strict system prompt (see Technical Notes)
- [ ] LLM response is parsed into exactly 3 sections:
  - `yesterday`: list of completed work items
  - today`: list of planned/in-progress items (inferred from WIP commits)
  - `blockers`: extracted from commit messages containing keywords:
     `fix`, `bug`, `broken`, `fail`, `revert`, `hotfix`, `todo`, `wip`
- [ ] If LLM returns malformed JSON → fallback to keyword-based parser in Go
- [ ] Response schema:
```json
{
  "yesterday": ["string"],
  "today": ["string"],
  "blockers": ["string"],
  "raw_summary": "string"
}
```

**Technical Notes:**
- Service: `backend/internal/services/standup_service.go`
- mlc-llm runs locally at `http://localhost:8080/v1/chat/completions`
  (OpenAI-compatible endpoint)
- System prompt to inject:
  > "You are a developer standup assistant. Given a list of git commit messages,
  > extract and return ONLY a JSON object with keys: yesterday, today, blockers.
  > Each value is an array of concise strings. No explanation. No markdown."
- Model: `Llama-3.1-8B-Instruct-q4f32_1` (or any mlc-llm served model)
- Timeout: 30s — if exceeded, return keyword-based fallback silently

---

## Epic 2: Privacy-First Local LLM Integration

### Story 3a: Local AI Model Discovery & Selection UI
**As a** developer,
**I want to** see and select from locally available AI models before connecting,
**so that** I don't have to download a new model if I already have one running.

**Acceptance Criteria:**
- [ ] A "Local AI" modal/panel opens before the standup is generated
- [ ] Top section — **Recommended Model (mlc-llm):**
  - Displays: model name `Llama-3.1-8B-Instruct-q4f32_1`, size `~4.5GB`, description
  - "Download & Run" button → triggers `mlc_llm.serve` via backend shell command
  - Download progress shown as a progress bar (SSE stream from backend)
  - Button disabled + spinner shown while download/boot is in progress
  - On success → model auto-selected and modal moves to "connected" state
- [ ] Bottom section — **Detected Local Models:**
  - Backend probes known local AI endpoints on app startup and on manual refresh:
    - `http://localhost:8080` → mlc-llm
    - `http://localhost:11434` → Ollama
    - `http://localhost:1234` → LM Studio
    - `http://localhost:8081` → LocalAI
  - Each detected service shown as a card: name, endpoint, status badge
    (`● Running` in green / `○ Not found` in gray)
  - "Connect" button on each running model → sets it as active LLM for the session
- [ ] Active model shown in top bar with a green dot: `● Llama-3.1-8B (Ollama)`
- [ ] If no model is running and mlc-llm not downloaded → warning banner:
  `"No local AI detected. Download the recommended model or start a local server."`
- [ ] "Refresh" button re-probes all endpoints without page reload

**Technical Notes:**
- Handler: `backend/internal/controllers/llm_discovery_controller.go`
- Service: `backend/internal/services/llm_discovery_service.go`
- Probe logic: send `GET /health` or `GET /v1/models` to each known port,
  timeout 800ms per probe, run all probes concurrently with `goroutine + WaitGroup`
- Probe response schema:
```json
{
  "providers": [
    {
      "name": "mlc-llm",
      "endpoint": "http://localhost:8080",
      "status": "running" | "not_found",
      "models": ["Llama-3.1-8B-Instruct-q4f32_1"],
      "recommended": true
    },
    {
      "name": "Ollama",
      "endpoint": "http://localhost:11434",
      "status": "running" | "not_found",
      "models": ["llama3", "mistral"],
      "recommended": false
    }
  ]
}
```
- Download trigger: POST `/api/llm/download` → runs `mlc_llm serve` as subprocess
  in Go using `exec.Command`, streams stdout as SSE to frontend
- Active model stored in backend session (in-memory, per-user context) and
  returned in every `/api/standup` and `/api/chat` call automatically
- Frontend: modal built as a React component `components/LocalAISelector.tsx`
- Status badges use Tailwind: `text-green-400` for running, `text-gray-400` for not found
- Auto-probe on page load — user should never have to open the modal manually
  if a model is already running

---

## Epic 3: Frontend UI

### Story 4: Single-Page Standup Dashboard
**As a** developer,
**I want** a single clean page that shows my standup and lets me chat with it,
**so that** I can review, copy, and explore my daily summary without switching tools.

**Acceptance Criteria:**
- [ ] Single page layout with two panels:
  - **Left panel:** Standup Summary Card
    - Three sections rendered: Yesterday / Today / Blockers
    - Each section is a styled list (not a wall of text)
    - "Copy as Text" button → copies plain-text standup to clipboard
    - "Copy as Markdown" button → copies `## Yesterday\n- ...` format
  - **Right panel:** Chat Window
    - Input field at the bottom, message bubbles above
    - Streamed tokens appear in real-time (no waiting for full response)
    - "You" vs "Bot" messages visually distinct
- [ ] Top bar: repo input field + GitHub token input (masked) + "Generate Standup" button
- [ ] Loading state shown during commit fetch and LLM processing
- [ ] Error states handled: invalid token, empty commits, LLM timeout

**Technical Notes:**
- Framework: Next.js App Router (`app/page.tsx`)
- Styling: Tailwind CSS — keep it minimal, monospace font preferred for dev aesthetic
- No external UI library needed — custom components only
- API calls go to Go backend via env var `NEXT_PUBLIC_API_URL=http://localhost:3001`
- Token input: `type="password"` field, value never sent to any analytics or logging

---

## Technical Constraints (Global)
- GitHub token → in-memory only, never written to DB or logs
- All LLM inference → local mlc-llm, no OpenAI/Anthropic API calls
- Commit data → processed and discarded per request, no persistence
- Docker Compose must spin up: Go backend, Next.js frontend, mlc-llm server