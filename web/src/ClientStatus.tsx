import { useState, useEffect, useCallback, useRef } from "react";
import type { StatusResponse, ClientStatus } from "./types";
import { fetchJSON } from "./utils";

export function ClientStatus({
  authed,
  refreshSignal,
  onWake,
  onShutdown,
  onOnline,
  onOffline,
}: {
  authed: boolean;
  refreshSignal: { key: number; silent: boolean };
  onWake?: () => void;
  onShutdown?: () => void;
  onOnline?: () => void;
  onOffline?: () => void;
}) {
  const [client, setClient] = useState<ClientStatus | null>(null);
  const [busy, setBusy] = useState(false);

  const prevStatusRef = useRef<string | null>(null);

  const fetchStatus = useCallback(async () => {
    if (!authed) return;
    try {
      const data = await fetchJSON<StatusResponse>("/api/status");
      if (data.client) {
        setClient(data.client);

        // Handle callbacks based on status change
        if (prevStatusRef.current !== data.client.status) {
          if (data.client.status === "online") onOnline?.();
          if (data.client.status === "offline") onOffline?.();
          prevStatusRef.current = data.client.status;
        }
      }
    } catch {
      // Ignore errors
    }
  }, [authed, onOnline, onOffline]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus, refreshSignal]);

  // Poll status
  useEffect(() => {
    if (!authed) return;
    const id = setInterval(() => fetchStatus(), 3000);
    return () => clearInterval(id);
  }, [authed, fetchStatus]);

  const wake = async () => {
    setBusy(true);
    try {
      const res = await fetch("/api/wake", { method: "POST" });
      const data = await res.json();
      if (data.success) {
        onWake?.();
      } else {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send wake packet");
    } finally {
      setBusy(false);
    }
  };

  const shutdown = async () => {
    if (!confirm("Are you sure you want to shut down?")) return;
    setBusy(true);
    try {
      const res = await fetch("/api/shutdown", { method: "POST" });
      const data = await res.json();
      if (data.success) {
        onShutdown?.();
      } else {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send shutdown command");
    } finally {
      setBusy(false);
    }
  };

  if (!client) return null;

  const isOnline = client.status === "online";
  const isOffline = client.status === "offline";
  const isWaking = client.status === "waking";
  const isShuttingDown = client.status === "shutting_down";
  const isBusy = busy || isWaking || isShuttingDown;

  const formatStatus = (s: string) => {
    if (s === "waking") return "Waking";
    if (s === "shutting_down") return "Shutting down";
    return s.charAt(0).toUpperCase() + s.slice(1);
  };

  return (
    <section className="mb-6">
      <div className="flex items-center justify-between p-4 rounded-xl border border-zinc-700/50 bg-zinc-800/50">
        <div>
          <div className="flex items-center gap-2">
            <span className={`font-medium ${isOnline ? "text-green-400" : isOffline ? "text-red-400" : "text-zinc-300"}`}>
              {isOnline ? "Online" : isOffline ? "Offline" : formatStatus(client.status)}
            </span>
            {isBusy && (
              <svg className="animate-spin h-3.5 w-3.5 text-zinc-400" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
            )}
          </div>
          <div className="text-xs text-zinc-500 font-mono mt-1">{client.ip_address}</div>
          <div className="text-[10px] text-zinc-600 font-mono">{client.mac_address}</div>
        </div>

        <div>
          {isOffline && (
            <button
              onClick={wake}
              disabled={isBusy}
              className="px-3 py-1.5 rounded-lg bg-zinc-700 hover:bg-zinc-600 text-zinc-200 text-sm font-medium transition-colors disabled:opacity-50 cursor-pointer"
            >
              Wake
            </button>
          )}
          {isOnline && (
            <button
              onClick={shutdown}
              disabled={isBusy}
              className="px-3 py-1.5 rounded-lg bg-red-900/30 hover:bg-red-900/50 text-red-200 text-sm font-medium transition-colors disabled:opacity-50 cursor-pointer"
            >
              Shutdown
            </button>
          )}
        </div>
      </div>
    </section>
  );
}