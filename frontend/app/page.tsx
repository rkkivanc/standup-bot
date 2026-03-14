"use client";

import { useCallback, useMemo, useState } from "react";
import LocalAISelector from "../components/LocalAISelector";

const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

type StandupSection = string[];

type StandupResponse = {
  yesterday: StandupSection;
  today: StandupSection;
  blockers: StandupSection;
  raw_summary?: string;
  active_model?: {
    name: string;
    provider: string;
  };
};

type Message = {
  id: string;
  role: "user" | "assistant";
  content: string;
};

type ChatError = string | null;

const initialStandup: StandupResponse = {
  yesterday: [],
  today: [],
  blockers: [],
};

function buildPlainText(summary: StandupResponse): string {
  const sections: string[] = [];

  sections.push("Yesterday:");
  sections.push(
    summary.yesterday.length
      ? summary.yesterday.map((item) => `- ${item}`).join("\n")
      : "- (none)"
  );
  sections.push("");

  sections.push("Today:");
  sections.push(
    summary.today.length
      ? summary.today.map((item) => `- ${item}`).join("\n")
      : "- (none)"
  );
  sections.push("");

  sections.push("Blockers:");
  sections.push(
    summary.blockers.length
      ? summary.blockers.map((item) => `- ${item}`).join("\n")
      : "- (none)"
  );

  return sections.join("\n");
}

function buildMarkdown(summary: StandupResponse): string {
  const sections: string[] = [];

  sections.push("## Yesterday");
  sections.push(
    summary.yesterday.length
      ? summary.yesterday.map((item) => `- ${item}`).join("\n")
      : "- (none)"
  );
  sections.push("");

  sections.push("## Today");
  sections.push(
    summary.today.length
      ? summary.today.map((item) => `- ${item}`).join("\n")
      : "- (none)"
  );
  sections.push("");

  sections.push("## Blockers");
  sections.push(
    summary.blockers.length
      ? summary.blockers.map((item) => `- ${item}`).join("\n")
      : "- (none)"
  );

  return sections.join("\n");
}

async function copyToClipboard(text: string) {
  if (typeof navigator === "undefined" || !navigator.clipboard) return;

  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // ignore copy failures
  }
}

export default function Home() {
  const [repoInput, setRepoInput] = useState("");
  const [tokenInput, setTokenInput] = useState("");
  const [isGenerating, setIsGenerating] = useState(false);
  const [standup, setStandup] = useState<StandupResponse>(initialStandup);
  const [standupError, setStandupError] = useState<string | null>(null);

  const [messages, setMessages] = useState<Message[]>([]);
  const [chatInput, setChatInput] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [chatError, setChatError] = useState<ChatError>(null);

  const [activeModel, setActiveModel] = useState<{
    name: string;
    provider: string;
  } | null>(null);
  const [showLocalAIModal, setShowLocalAIModal] = useState(true);

  const hasStandup =
    standup.yesterday.length > 0 ||
    standup.today.length > 0 ||
    standup.blockers.length > 0;

  const standupContext = useMemo(
    () => (hasStandup ? buildPlainText(standup) : ""),
    [standup, hasStandup]
  );

  const handleGenerateStandup = useCallback(async () => {
    if (!repoInput.trim() || !tokenInput.trim()) {
      setStandupError("Please enter both a repo and GitHub token.");
      return;
    }

    setIsGenerating(true);
    setStandupError(null);

    // Parse owner/repo from single repo input field
    const [ownerRaw, repoRaw] = repoInput.split("/");
    const owner = (ownerRaw ?? "").trim();
    const repo = (repoRaw ?? "").trim();

    if (!owner || !repo) {
      setIsGenerating(false);
      setStandupError("Repo must be in the form owner/repo.");
      return;
    }

    try {
      const commitsRes = await fetch(`${API_BASE}/api/commits`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          github_token: tokenInput,
          owner,
          repo,
        }),
      });

      if (!commitsRes.ok) {
        const data = await commitsRes.json().catch(() => null);
        const code = data?.code;
        if (code === 401) {
          setStandupError("Invalid GitHub token. Please double-check it.");
        } else if (code === 404) {
          setStandupError("Repository not found. Check the owner/repo name.");
        } else {
          setStandupError(
            data?.error ??
              "Failed to fetch commits. Please try again in a moment."
          );
        }
        setIsGenerating(false);
        return;
      }

      const commitsData = await commitsRes.json().catch(() => null);
      if (!Array.isArray(commitsData) || commitsData.length === 0) {
        setStandupError(
          "No commits found in the last 24 hours for this repository."
        );
        setIsGenerating(false);
        return;
      }

      const standupRes = await fetch(`${API_BASE}/api/standup`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(commitsData),
      });

      if (!standupRes.ok) {
        const data = await standupRes.json().catch(() => null);
        const code = data?.code;
        if (code === "timeout") {
          setStandupError(
            "Standup generation timed out. Please try again; the LLM may be warming up."
          );
        } else {
          setStandupError(
            data?.error ??
              "Failed to generate standup summary. Please try again."
          );
        }
        setIsGenerating(false);
        return;
      }

      const summary = (await standupRes.json()) as StandupResponse;
      setStandup({
        yesterday: summary.yesterday ?? [],
        today: summary.today ?? [],
        blockers: summary.blockers ?? [],
        raw_summary: summary.raw_summary,
        active_model: summary.active_model,
      });

      if (summary.active_model) {
        setActiveModel(summary.active_model);
      }
    } catch {
      setStandupError(
        "Unexpected error while generating standup. Please check that the backend is running."
      );
    } finally {
      setIsGenerating(false);
    }
  }, [repoInput, tokenInput]);

  const handleChatSubmit = useCallback(
    async (event: React.FormEvent) => {
      event.preventDefault();
      if (!chatInput.trim()) return;
      if (!hasStandup) {
        setChatError("Generate a standup first, then chat about it.");
        return;
      }

      setChatError(null);

      const userMessage: Message = {
        id: crypto.randomUUID(),
        role: "user",
        content: chatInput.trim(),
      };

      const assistantMessage: Message = {
        id: crypto.randomUUID(),
        role: "assistant",
        content: "",
      };

      setMessages((prev) => [...prev, userMessage, assistantMessage]);
      setChatInput("");
      setIsStreaming(true);

      try {
        const response = await fetch(`${API_BASE}/api/chat`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            message: userMessage.content,
            context: standupContext,
          }),
        });

        if (!response.body) {
          setChatError("No response body received from chat endpoint.");
          setIsStreaming(false);
          return;
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder("utf-8");
        let done = false;
        let buffer = "";

        while (!done) {
          const result = await reader.read();
          done = result.done;
          if (result.value) {
            buffer += decoder.decode(result.value, { stream: !done });

            const events = buffer.split("\n\n");
            buffer = events.pop() ?? "";

            for (const event of events) {
              const lines = event
                .split("\n")
                .map((line) => line.trim())
                .filter(Boolean);

              for (const line of lines) {
                if (!line.startsWith("data:")) continue;

                const data = line.slice(5).trim();
                if (!data || data === "[DONE]") {
                  done = true;
                  break;
                }

                let token = data;
                try {
                  const parsed = JSON.parse(data);
                  if (typeof parsed?.token === "string") {
                    token = parsed.token;
                  } else if (typeof parsed?.content === "string") {
                    token = parsed.content;
                  }
                } catch {
                  // treat as plain token
                }

                setMessages((prev) =>
                  prev.map((msg) =>
                    msg.id === assistantMessage.id
                      ? { ...msg, content: msg.content + token }
                      : msg
                  )
                );
              }
            }
          }
        }
      } catch {
        setChatError(
          "Error while streaming chat response. Ensure the local AI is running."
        );
      } finally {
        setIsStreaming(false);
      }
    },
    [chatInput, hasStandup, standupContext]
  );

  const handleConnectedModel = useCallback(
    (modelName: string, provider: string) => {
      setActiveModel({ name: modelName, provider });
      setShowLocalAIModal(false);
    },
    []
  );

  const handleCopyText = useCallback(() => {
    if (!hasStandup) return;
    void copyToClipboard(buildPlainText(standup));
  }, [standup, hasStandup]);

  const handleCopyMarkdown = useCallback(() => {
    if (!hasStandup) return;
    void copyToClipboard(buildMarkdown(standup));
  }, [standup, hasStandup]);

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50 font-mono">
      <div className="mx-auto flex min-h-screen max-w-6xl flex-col px-4 py-6 gap-4">
        {/* Top bar */}
        <header className="flex flex-col gap-3 rounded-lg border border-zinc-800 bg-zinc-900/80 px-4 py-3 shadow-sm backdrop-blur md:flex-row md:items-center md:justify-between">
          <div className="flex flex-1 flex-col gap-2 md:flex-row md:items-center md:gap-3">
            <div className="flex flex-1 items-center gap-2">
              <label
                htmlFor="repo"
                className="whitespace-nowrap text-xs uppercase tracking-wide text-zinc-400"
              >
                Repo
              </label>
              <input
                id="repo"
                type="text"
                placeholder="owner/repo"
                className="h-9 w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 text-sm outline-none ring-0 focus:border-zinc-400"
                value={repoInput}
                onChange={(e) => setRepoInput(e.target.value)}
              />
            </div>
            <div className="flex flex-1 items-center gap-2">
              <label
                htmlFor="token"
                className="whitespace-nowrap text-xs uppercase tracking-wide text-zinc-400"
              >
                GitHub Token
              </label>
              <input
                id="token"
                type="password"
                placeholder="••••••••••••"
                className="h-9 w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 text-sm outline-none ring-0 focus:border-zinc-400"
                value={tokenInput}
                onChange={(e) => setTokenInput(e.target.value)}
              />
            </div>
          </div>

          <div className="mt-2 flex items-center justify-between gap-3 md:mt-0 md:justify-end">
            <div className="text-xs text-zinc-400">
              {activeModel ? (
                <span className="inline-flex items-center gap-1">
                  <span className="text-green-400">●</span>
                  <span>
                    {activeModel.name} ({activeModel.provider})
                  </span>
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 text-zinc-500">
                  <span className="text-zinc-500">○</span>
                  <span>No model connected</span>
                </span>
              )}
            </div>
            <button
              type="button"
              onClick={handleGenerateStandup}
              disabled={isGenerating}
              className="inline-flex h-9 items-center justify-center rounded-md border border-zinc-600 bg-zinc-100 text-zinc-900 px-3 text-xs font-semibold uppercase tracking-wide transition hover:bg-white disabled:cursor-not-allowed disabled:border-zinc-700 disabled:bg-zinc-800 disabled:text-zinc-400"
            >
              {isGenerating ? "Generating…" : "Generate Standup"}
            </button>
          </div>
        </header>

        {standupError && (
          <div className="rounded-md border border-red-500/40 bg-red-950/40 px-3 py-2 text-xs text-red-200">
            {standupError}
          </div>
        )}

        {/* Main panels */}
        <main className="grid flex-1 gap-4 md:grid-cols-2">
          {/* Standup panel */}
          <section className="flex flex-col rounded-lg border border-zinc-800 bg-zinc-900/80 p-4 shadow-sm">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <h2 className="text-sm font-semibold tracking-wide text-zinc-100">
                  Standup Summary
                </h2>
                <p className="text-xs text-zinc-500">
                  Yesterday / Today / Blockers
                </p>
              </div>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={handleCopyText}
                  disabled={!hasStandup}
                  className="inline-flex h-7 items-center justify-center rounded border border-zinc-700 px-2 text-[10px] font-semibold uppercase tracking-wide text-zinc-200 transition hover:bg-zinc-800 disabled:cursor-not-allowed disabled:border-zinc-800 disabled:text-zinc-500"
                >
                  Copy as Text
                </button>
                <button
                  type="button"
                  onClick={handleCopyMarkdown}
                  disabled={!hasStandup}
                  className="inline-flex h-7 items-center justify-center rounded border border-zinc-700 px-2 text-[10px] font-semibold uppercase tracking-wide text-zinc-200 transition hover:bg-zinc-800 disabled:cursor-not-allowed disabled:border-zinc-800 disabled:text-zinc-500"
                >
                  Copy as Markdown
                </button>
              </div>
            </div>

            <div className="relative flex-1 overflow-hidden rounded-md border border-zinc-800 bg-zinc-950/60 px-3 py-3 text-sm">
              {isGenerating && (
                <div className="absolute inset-0 flex flex-col gap-3 bg-zinc-950/80 p-3">
                  <div className="h-3 w-1/2 animate-pulse rounded bg-zinc-800" />
                  <div className="h-3 w-2/3 animate-pulse rounded bg-zinc-800" />
                  <div className="h-3 w-1/3 animate-pulse rounded bg-zinc-800" />
                  <div className="mt-2 flex-1 space-y-2">
                    <div className="h-3 w-full animate-pulse rounded bg-zinc-800" />
                    <div className="h-3 w-5/6 animate-pulse rounded bg-zinc-800" />
                    <div className="h-3 w-3/4 animate-pulse rounded bg-zinc-800" />
                  </div>
                </div>
              )}

              {!hasStandup && !isGenerating && (
                <p className="text-xs text-zinc-500">
                  Generate a standup to see your summary here.
                </p>
              )}

              {hasStandup && !isGenerating && (
                <div className="flex flex-col gap-4">
                  <div>
                    <h3 className="text-xs font-semibold uppercase tracking-wide text-zinc-400">
                      Yesterday
                    </h3>
                    <ul className="mt-1 list-disc space-y-1 pl-4 text-xs text-zinc-100">
                      {standup.yesterday.length ? (
                        standup.yesterday.map((item, idx) => (
                          <li key={`y-${idx}`}>{item}</li>
                        ))
                      ) : (
                        <li className="text-zinc-500">(none)</li>
                      )}
                    </ul>
                  </div>

                  <div>
                    <h3 className="text-xs font-semibold uppercase tracking-wide text-zinc-400">
                      Today
                    </h3>
                    <ul className="mt-1 list-disc space-y-1 pl-4 text-xs text-zinc-100">
                      {standup.today.length ? (
                        standup.today.map((item, idx) => (
                          <li key={`t-${idx}`}>{item}</li>
                        ))
                      ) : (
                        <li className="text-zinc-500">(none)</li>
                      )}
                    </ul>
                  </div>

                  <div>
                    <h3 className="text-xs font-semibold uppercase tracking-wide text-zinc-400">
                      Blockers
                    </h3>
                    <ul className="mt-1 list-disc space-y-1 pl-4 text-xs text-zinc-100">
                      {standup.blockers.length ? (
                        standup.blockers.map((item, idx) => (
                          <li key={`b-${idx}`}>{item}</li>
                        ))
                      ) : (
                        <li className="text-zinc-500">(none)</li>
                      )}
                    </ul>
                  </div>
                </div>
              )}
            </div>
          </section>

          {/* Chat panel */}
          <section className="flex flex-col rounded-lg border border-zinc-800 bg-zinc-900/80 p-4 shadow-sm">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <h2 className="text-sm font-semibold tracking-wide text-zinc-100">
                  Chat with your Standup
                </h2>
                <p className="text-xs text-zinc-500">
                  Ask follow-up questions about today&apos;s work.
                </p>
              </div>
            </div>

            <div className="flex-1 overflow-hidden rounded-md border border-zinc-800 bg-zinc-950/60">
              <div className="flex h-full flex-col">
                <div className="flex-1 space-y-3 overflow-y-auto p-3 text-sm">
                  {messages.length === 0 && (
                    <p className="text-xs text-zinc-500">
                      Once you generate a standup, ask anything about it here.
                    </p>
                  )}

                  {messages.map((message) => (
                    <div
                      key={message.id}
                      className={`flex ${
                        message.role === "user"
                          ? "justify-end"
                          : "justify-start"
                      }`}
                    >
                      <div
                        className={`max-w-[80%] rounded-md px-3 py-2 text-xs leading-relaxed ${
                          message.role === "user"
                            ? "bg-zinc-100 text-zinc-900"
                            : "bg-zinc-800 text-zinc-50"
                        }`}
                      >
                        <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">
                          {message.role === "user" ? "You" : "Bot"}
                        </div>
                        <div className="whitespace-pre-wrap">
                          {message.content || (isStreaming && "…")}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>

                {chatError && (
                  <div className="border-t border-red-500/40 bg-red-950/40 px-3 py-2 text-[11px] text-red-200">
                    {chatError}
                  </div>
                )}

                <form
                  onSubmit={handleChatSubmit}
                  className="flex items-center gap-2 border-t border-zinc-800 bg-zinc-950/80 px-3 py-2"
                >
                  <input
                    type="text"
                    placeholder="Ask about your standup…"
                    className="h-8 flex-1 rounded-md border border-zinc-700 bg-zinc-950 px-2 text-xs outline-none ring-0 focus:border-zinc-400"
                    value={chatInput}
                    onChange={(e) => setChatInput(e.target.value)}
                    disabled={isStreaming}
                  />
                  <button
                    type="submit"
                    disabled={isStreaming || !chatInput.trim()}
                    className="inline-flex h-8 items-center justify-center rounded-md border border-zinc-600 bg-zinc-100 px-3 text-[11px] font-semibold uppercase tracking-wide text-zinc-900 transition hover:bg-white disabled:cursor-not-allowed disabled:border-zinc-700 disabled:bg-zinc-800 disabled:text-zinc-400"
                  >
                    {isStreaming ? "Streaming…" : "Send"}
                  </button>
                </form>
              </div>
            </div>
          </section>
        </main>
      </div>

      <LocalAISelector
        open={showLocalAIModal}
        onClose={() => setShowLocalAIModal(false)}
        onConnected={handleConnectedModel}
      />
    </div>
  );
}

