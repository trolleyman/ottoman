import { useState, useEffect, useCallback } from "react";
import type { MonitorInfo } from "./types";
import { fetchJSON, sortedMonitors } from "./utils";

function Row({ label, value }: { label: string; value: string }) {
  return (
    <>
      <span className="text-zinc-500">{label}</span>
      <span className="text-zinc-300 font-mono text-xs leading-5 truncate">
        {value}
      </span>
    </>
  );
}

function MonitorCard({ monitor }: { monitor: MonitorInfo }) {
  const a = monitor.active;
  return (
    <div className={`rounded-xl border p-5 flex flex-col gap-3 ${a
      ? "border-zinc-700/50 bg-zinc-800/50"
      : "border-zinc-800/50 bg-zinc-900/50 opacity-60"
      }`}>
      <div className="flex items-center justify-between">
        <h3 className={`font-semibold truncate ${a ? "text-zinc-100" : "text-zinc-400"}`}>
          {monitor.name || monitor.port || "Unknown"}
        </h3>
        <div className="flex gap-2">
          {!a && (
            <span className="text-xs font-medium bg-zinc-700/30 text-zinc-500 px-2 py-0.5 rounded-full">
              Inactive
            </span>
          )}
          {a?.primary && (
            <span className="text-xs font-medium bg-blue-500/20 text-blue-400 px-2 py-0.5 rounded-full">
              Primary
            </span>
          )}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm">
        {a && (
          <>
            <Row label="Resolution" value={`${a.width}x${a.height}`} />
            <Row
              label="Refresh"
              value={`${Number.isInteger(a.refresh_rate) ? a.refresh_rate : a.refresh_rate.toFixed(1)} Hz`}
            />
            <Row label="Position" value={`${a.position_x}, ${a.position_y}`} />
          </>
        )}
        {monitor.port && <Row label="Port" value={monitor.port} />}
        {monitor.edid && <Row label="EDID" value={monitor.edid} />}
        {monitor.manufacturer && (
          <Row label="Manufacturer" value={monitor.manufacturer} />
        )}
      </div>
    </div>
  );
}

export function Monitors({
  authed,
  refreshSignal,
}: {
  authed: boolean;
  refreshSignal: { key: number; silent: boolean };
}) {
  const [monitors, setMonitors] = useState<MonitorInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchMonitors = useCallback(async (silent: boolean) => {
    if (!authed) return;
    if (!silent) setLoading(true);
    try {
      const monitorsData = await fetchJSON<MonitorInfo[]>("/api/monitors");
      setMonitors(sortedMonitors(monitorsData));
      setError(null);
    } catch (e) {
      setError("Failed to load monitors");
    } finally {
      setLoading(false);
    }
  }, [authed]);

  useEffect(() => {
    fetchMonitors(refreshSignal.silent);
  }, [fetchMonitors, refreshSignal]);

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200">Monitors</h2>
        <span className="text-xs text-zinc-500">
          {monitors.filter((m) => m.active).length} active / {monitors.length} total
        </span>
      </div>
      {loading && monitors.length === 0 ? (
        <div className="text-zinc-500 text-sm">Loading monitors...</div>
      ) : error ? (
        <div className="text-red-400 text-sm">{error}</div>
      ) : monitors.length === 0 ? (
        <p className="text-zinc-500 text-sm">No monitors detected.</p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {monitors.map((m, i) => (
            <MonitorCard key={m.port || m.edid || i} monitor={m} />
          ))}
        </div>
      )}
    </section>
  );
}
