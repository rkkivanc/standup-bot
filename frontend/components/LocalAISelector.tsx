"use client";

import { useEffect, useMemo, useState } from "react";

const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

type ProviderStatus = "running" | "not_found";

type Provider = {
  name: string;
  endpoint: string;
  status: ProviderStatus;
  models?: string[];
  recommended?: boolean;
};

type ProvidersResponse = {
  providers: Provider[];
  active_model?: {
    name: string;
    provider: string;
  };
};

type LocalAISelectorProps = {
  open: boolean;
  onClose: () => void;
  onConnected: (modelName: string, provider: string) => void;
};

type DownloadState = {
  inProgress: boolean;
  progress: number | null;
  message?: string;
};

export default function LocalAISelector({
  open,
  onClose,
  onConnected,
}: LocalAISelectorProps) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loadingProviders, setLoadingProviders] = useState(false);
  const [providersError, setProvidersError] = useState<string | null>(null);

  const [download, setDownload] = useState<DownloadState>({
    inProgress: false,
    progress: null,
  });

  const [connectingProvider, setConnectingProvider] = useState<string | null>(
    null
  );

  const anyRunning = useMemo(
    () => providers.some((p) => p.status === "running"),
    [providers]
  );

  const recommendedProvider = useMemo(
    () => providers.find((p) => p.recommended),
    [providers]
  );

  useEffect(() => {
    if (!open) return;
    void fetchProviders();
  }, [open]);

  async function fetchProviders() {
    setLoadingProviders(true);
    setProvidersError(null);
    try {
      const res = await fetch(`${API_BASE}/api/llm/providers`, {
        method: "GET",
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        setProvidersError(
          data?.error ?? "Failed to load local AI providers."
        );
        setProviders([]);
        return;
      }
      const data = (await res.json()) as ProvidersResponse;
      setProviders(data.providers ?? []);
      if (data.active_model) {
        onConnected(data.active_model.name, data.active_model.provider);
      }
    } catch {
      setProvidersError(
        "Error while probing local AI providers. Ensure the backend is running."
      );
      setProviders([]);
    } finally {
      setLoadingProviders(false);
    }
  }

  async function handleDownloadAndRun() {
    if (download.inProgress) return;

    setDownload({ inProgress: true, progress: 0, message: "Starting..." });

    try {
      const res = await fetch(`${API_BASE}/api/llm/download`, {
        method: "POST",
      });

      if (!res.body) {
        setDownload({
          inProgress: false,
          progress: null,
          message: "No download stream received.",
        });
        return;
      }

      const reader = res.body.getReader();
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

              let progress = null as number | null;
              let message: string | undefined;

              try {
                const parsed = JSON.parse(data);
                if (typeof parsed?.progress === "number") {
                  progress = parsed.progress;
                }
                if (typeof parsed?.message === "string") {
                  message = parsed.message;
                } else if (typeof parsed?.progress === "string") {
                  message = parsed.progress;
                }
              } catch {
                message = data;
              }

              setDownload((prev) => ({
                inProgress: true,
                progress: progress ?? prev.progress,
                message: message ?? prev.message,
              }));
            }
          }
        }
      }

      // After download completes, refresh providers and attempt to connect to recommended
      await fetchProviders();
      const modelName =
        recommendedProvider?.models?.[0] ??
        "gemma3:1b";
      const providerName = recommendedProvider?.name ?? "Ollama";
      onConnected(modelName, providerName);
      onClose();
      setDownload({ inProgress: false, progress: 100, message: "Ready" });
    } catch {
      setDownload({
        inProgress: false,
        progress: null,
        message: "Download failed. Check logs and try again.",
      });
    }
  }

  async function handleConnect(provider: Provider) {
    if (provider.status !== "running") return;
    setConnectingProvider(provider.name);
    try {
      const res = await fetch(`${API_BASE}/api/llm/connect`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          name: provider.name,
          endpoint: provider.endpoint,
        }),
      });

      if (!res.ok) {
        setProvidersError(
          "Failed to connect to selected model. Please try again."
        );
        return;
      }

      const data = await res.json().catch(() => null);
      const modelName =
        data?.active_model?.name ??
        provider.models?.[0] ??
        "gemma3:1b";

      onConnected(modelName, provider.name);
      onClose();
    } catch {
      setProvidersError(
        "Error while connecting to provider. Ensure the server is reachable."
      );
    } finally {
      setConnectingProvider(null);
    }
  }

  if (!open) return null;

  const showWarning = !loadingProviders && !anyRunning && !download.inProgress;

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/70 backdrop-blur-sm">
      <div className="w-full max-w-xl rounded-lg border border-zinc-800 bg-zinc-950 p-4 shadow-xl">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <h2 className="text-sm font-semibold tracking-wide text-zinc-50">
              Local AI
            </h2>
            <p className="text-xs text-zinc-500">
              Choose a local model before generating your standup.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={fetchProviders}
              disabled={loadingProviders}
              className="inline-flex h-7 items-center justify-center rounded border border-zinc-700 bg-zinc-900 px-2 text-[10px] font-semibold uppercase tracking-wide text-zinc-200 transition hover:bg-zinc-800 disabled:cursor-not-allowed disabled:text-zinc-500"
            >
              {loadingProviders ? "Refreshing…" : "Refresh"}
            </button>
            <button
              type="button"
              onClick={onClose}
              className="inline-flex h-7 items-center justify-center rounded border border-zinc-700 bg-zinc-900 px-2 text-[10px] font-semibold uppercase tracking-wide text-zinc-400 transition hover:bg-zinc-800"
            >
              Skip
            </button>
          </div>
        </div>

        {providersError && (
          <div className="mb-3 rounded-md border border-red-500/40 bg-red-950/40 px-3 py-2 text-[11px] text-red-200">
            {providersError}
          </div>
        )}

        {showWarning && (
          <div className="mb-3 rounded-md border border-amber-500/40 bg-amber-950/40 px-3 py-2 text-[11px] text-amber-100">
            No local AI detected. Download the recommended model or start a
            local server.
          </div>
        )}

        {/* Recommended model */}
        <div className="mb-3 rounded-md border border-zinc-800 bg-zinc-900/80 p-3">
          <div className="mb-2 flex items-center justify-between">
            <div>
              <h3 className="text-xs font-semibold uppercase tracking-wide text-zinc-300">
                Recommended Model (Ollama)
              </h3>
              <p className="text-xs text-zinc-500">
                gemma3:1b · ~1GB
              </p>
            </div>
            <button
              type="button"
              onClick={handleDownloadAndRun}
              disabled={download.inProgress}
              className="inline-flex h-8 items-center justify-center rounded-md border border-zinc-600 bg-zinc-100 px-3 text-[11px] font-semibold uppercase tracking-wide text-zinc-900 transition hover:bg-white disabled:cursor-not-allowed disabled:border-zinc-700 disabled:bg-zinc-800 disabled:text-zinc-400"
            >
              {download.inProgress ? (
                <span className="inline-flex items-center gap-2">
                  <span className="h-3 w-3 animate-spin rounded-full border border-zinc-400 border-t-transparent" />
                  <span>Downloading…</span>
                </span>
              ) : (
                "Download & Run"
              )}
            </button>
          </div>

          {download.inProgress || download.progress !== null ? (
            <div className="space-y-1">
              <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-800">
                <div
                  className="h-full rounded-full bg-green-400 transition-all"
                  style={{
                    width: `${Math.min(
                      100,
                      Math.max(download.progress ?? 5, 5)
                    )}%`,
                  }}
                />
              </div>
              {download.message && (
                <p className="text-[10px] text-zinc-400">{download.message}</p>
              )}
            </div>
          ) : (
            <p className="text-[11px] text-zinc-500">
              Downloads and runs the recommended Ollama model locally on your
              machine.
            </p>
          )}
        </div>

        {/* Providers list */}
        <div className="rounded-md border border-zinc-800 bg-zinc-900/80 p-3">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-zinc-300">
              Detected Local Models
            </h3>
          </div>

          <div className="space-y-2 max-h-56 overflow-y-auto">
            {loadingProviders && (
              <div className="text-[11px] text-zinc-500">Probing ports…</div>
            )}

            {!loadingProviders && providers.length === 0 && (
              <div className="text-[11px] text-zinc-500">
                No providers discovered yet.
              </div>
            )}

            {providers.map((provider) => {
              const isRunning = provider.status === "running";
              const isConnecting = connectingProvider === provider.name;
              const dotClass = isRunning
                ? "text-green-400"
                : "text-zinc-500";

              return (
                <div
                  key={provider.name + provider.endpoint}
                  className="flex items-center justify-between rounded border border-zinc-800 bg-zinc-950/60 px-3 py-2"
                >
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-semibold text-zinc-100">
                        {provider.name}
                      </span>
                      <span
                        className={`text-[11px] font-semibold ${dotClass}`}
                      >
                        {isRunning ? "● Running" : "○ Not found"}
                      </span>
                    </div>
                    <p className="text-[11px] text-zinc-500">
                      {provider.endpoint}
                    </p>
                    {provider.models && provider.models.length > 0 && (
                      <p className="text-[11px] text-zinc-500">
                        Models: {provider.models.join(", ")}
                      </p>
                    )}
                  </div>
                  <button
                    type="button"
                    disabled={!isRunning || isConnecting}
                    onClick={() => handleConnect(provider)}
                    className="inline-flex h-8 items-center justify-center rounded-md border border-zinc-600 bg-zinc-100 px-3 text-[11px] font-semibold uppercase tracking-wide text-zinc-900 transition hover:bg-white disabled:cursor-not-allowed disabled:border-zinc-700 disabled:bg-zinc-800 disabled:text-zinc-400"
                  >
                    {isConnecting ? "Connecting…" : "Connect"}
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

