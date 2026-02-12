import { useState, useEffect, useCallback } from "react";
import type { WakeTarget } from "./types";

export function WakeTargets({
  authed,
  refreshSignal,
  onWake,
  onShutdown,
}: {
  authed: boolean;
  refreshSignal: { key: number; silent: boolean };
  onWake?: () => void;
  onShutdown?: () => void;
}) {
  const [targets, setTargets] = useState<WakeTarget[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [wakingTargets, setWakingTargets] = useState<Set<string>>(new Set());
  const [shuttingDownTargets, setShuttingDownTargets] = useState<Set<string>>(new Set());

  const fetchWakeTargets = useCallback(async (silent: boolean) => {
    if (!authed) return;
    if (!silent) setLoading(true);
    try {
      const res = await fetch("/api/wake/targets");
      if (res.ok) {
        const data = await res.json();
        setTargets(data);
        setError(null);
      }
    } catch {
      setError("Failed to load wake targets");
    } finally {
      setLoading(false);
    }
  }, [authed]);

  useEffect(() => {
    fetchWakeTargets(refreshSignal.silent);
  }, [fetchWakeTargets, refreshSignal]);

  // Poll wake targets while any target is waking or shutting down
  useEffect(() => {
    if (wakingTargets.size === 0 && shuttingDownTargets.size === 0) return;

    const poll = async () => {
      try {
        const res = await fetch("/api/wake/targets");
        if (res.ok) {
          const data: WakeTarget[] = await res.json();
          setTargets(data);

          // Clear waking state for targets that came online
          const nowOnline = data.filter((t) => t.status === "online").map((t) => t.name);
          setWakingTargets((prev) => {
            const next = new Set(prev);
            for (const name of nowOnline) next.delete(name);
            return next;
          });

          // Clear shutting down state for targets that went offline
          const nowOffline = data.filter((t) => t.status === "offline").map((t) => t.name);
          setShuttingDownTargets((prev) => {
            const next = new Set(prev);
            for (const name of nowOffline) next.delete(name);
            return next;
          });
        }
      } catch { /* ignore polling errors */ }
    };

    const id = setInterval(poll, 3000);
    poll(); // immediate first poll
    return () => clearInterval(id);
  }, [wakingTargets, shuttingDownTargets]);

  const wake = async (target: string) => {
    setWakingTargets((prev) => new Set(prev).add(target));
    try {
      const res = await fetch("/api/wake", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ target }),
      });
      const data = await res.json();
      if (data.success) {
        onWake?.();
      } else {
        alert("Failed: " + data.message);
        setWakingTargets((prev) => { const next = new Set(prev); next.delete(target); return next; });
      }
    } catch {
      alert("Failed to send wake packet");
      setWakingTargets((prev) => { const next = new Set(prev); next.delete(target); return next; });
    }
  };

  const shutdown = async (target: string) => {
    if (!confirm(`Are you sure you want to shut down ${target}?`)) return;
    setShuttingDownTargets((prev) => new Set(prev).add(target));
    try {
      const res = await fetch("/api/shutdown", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
      });
      const data = await res.json();
      if (data.success) {
        onShutdown?.();
      } else {
        alert("Failed: " + data.message);
        setShuttingDownTargets((prev) => { const next = new Set(prev); next.delete(target); return next; });
      }
    } catch {
      alert("Failed to send shutdown command");
      setShuttingDownTargets((prev) => { const next = new Set(prev); next.delete(target); return next; });
    }
  };

  return (
    <section>
      <h2 className="text-lg font-semibold text-zinc-200 mb-4">Wake on LAN</h2>
      {loading && targets.length === 0 && !error ? (
        <div className="text-zinc-500 text-sm">Loading targets...</div>
      ) : error ? (
        <div className="text-red-400 text-sm">{error}</div>
      ) : targets.length === 0 ? (
        <div className="text-zinc-500 text-sm">No wake targets configured.</div>
      ) : (
        <div className="flex flex-wrap gap-3">
          {targets.map((target) => {
            const isWaking = wakingTargets.has(target.name);
            const isShuttingDown = shuttingDownTargets.has(target.name);
            const isOnline = target.status === "online";
            const isOffline = target.status === "offline";
            const isBusy = isWaking || isShuttingDown;

            return (
              <button
                key={target.mac_address}
                onClick={() => {
                  if (isBusy) return;
                  if (isOffline) wake(target.name);
                  if (isOnline) shutdown(target.name);
                }}
                className={`rounded-xl border p-4 transition-colors text-left min-w-[140px] cursor-pointer ${isOnline
                    ? "border-green-500/30 bg-green-500/10 hover:bg-green-500/20"
                    : isBusy
                      ? "border-zinc-500/30 bg-zinc-500/10 hover:bg-zinc-500/20"
                      : isOffline
                        ? "border-red-500/30 bg-red-500/10 hover:bg-red-500/20"
                        : "border-zinc-700/50 bg-zinc-800/50 hover:bg-zinc-800"
                  }`}
              >
                <div className="flex items-center gap-2">
                  <span className={`font-medium ${isOnline ? "text-green-400" : isBusy ? "text-zinc-300" : isOffline ? "text-red-400" : "text-zinc-200"
                    }`}>
                    {target.name}
                  </span>
                  {isBusy && (
                    <svg className="animate-spin h-3.5 w-3.5 text-zinc-400" viewBox="0 0 24 24" fill="none">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                  )}
                </div>
                <div className="text-xs text-zinc-500 font-mono mt-1">{target.ip_address}</div>
                <div className="text-[10px] text-zinc-600 font-mono">{target.mac_address}</div>
              </button>
            );
          })}
        </div>
      )}
    </section>
  );
}
