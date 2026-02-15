import { useState } from "react";
import { useStore } from "./store";

export function ClientStatus() {
  const clientStatus = useStore((s) => s.agentStatus);
  const clientInfo = useStore((s) => s.agentInfo);
  const loading = useStore((s) => s.agentLoading);
  const storeWake = useStore((s) => s.wake);
  const storeShutdown = useStore((s) => s.shutdown);

  const [busy, setBusy] = useState(false);

  const wake = async () => {
    setBusy(true);
    try {
      await storeWake();
    } finally {
      setBusy(false);
    }
  };

  const shutdown = async () => {
    setBusy(true);
    try {
      await storeShutdown();
    } finally {
      setBusy(false);
    }
  };

  const isOnline = clientStatus === "online";
  const isOffline = clientStatus === "offline";
  const isWaking = clientStatus === "waking";
  const isShuttingDown = clientStatus === "shutting_down";
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
              {isOnline ? "Online" : isOffline ? "Offline" : formatStatus(clientStatus)}
            </span>
            {(isBusy || loading) && (
              <svg className="animate-spin h-3.5 w-3.5 text-zinc-400" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
            )}
          </div>
          {clientInfo && (
            <>
              <div className="text-xs text-zinc-500 font-mono mt-1">{clientInfo.hostname}</div>
              <div className="text-[10px] text-zinc-600 font-mono">{clientInfo.ip_address}</div>
            </>
          )}
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
