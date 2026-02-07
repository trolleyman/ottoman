import { useState, useEffect, useCallback } from "react";

// --- Types matching Go API responses ---

interface MonitorInfo {
  edid: string;
  port: string;
  name: string;
  manufacturer: string;
  model: string;
  width: number;
  height: number;
  refresh_rate: number;
  position_x: number;
  position_y: number;
  primary: boolean;
  connected: boolean;
}

interface LayoutMonitor {
  edid: string;
  port: string;
  name?: string;
  width: number;
  height: number;
  refresh_rate: number;
  position_x: number;
  position_y: number;
  primary: boolean;
  enabled: boolean;
}

interface Layout {
  id: string;
  name: string;
  emoji?: string;
  aliases?: string[];
  monitors: LayoutMonitor[];
}

interface LayoutsResponse {
  layouts: Layout[];
  current_layout: string;
}

interface SwitchResponse {
  success: boolean;
  current_layout: string;
  message: string;
}

interface WakeTarget {
  name: string;
  mac_address: string;
  ip_address: string;
  status?: string;
}

// --- API helpers ---

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

// --- Components ---

function MonitorCard({ monitor }: { monitor: MonitorInfo }) {
  return (
    <div className="rounded-xl border border-zinc-700/50 bg-zinc-800/50 p-5 flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h3 className="font-semibold text-zinc-100 truncate">
          {monitor.name || monitor.model || monitor.port || "Unknown"}
        </h3>
        {monitor.primary && (
          <span className="text-xs font-medium bg-blue-500/20 text-blue-400 px-2 py-0.5 rounded-full">
            Primary
          </span>
        )}
      </div>

      <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm">
        <Row label="Resolution" value={`${monitor.width}x${monitor.height}`} />
        <Row
          label="Refresh"
          value={`${Number.isInteger(monitor.refresh_rate) ? monitor.refresh_rate : monitor.refresh_rate.toFixed(1)} Hz`}
        />
        {monitor.port && <Row label="Port" value={monitor.port} />}
        {monitor.edid && <Row label="EDID" value={monitor.edid} />}
        <Row
          label="Position"
          value={`${monitor.position_x}, ${monitor.position_y}`}
        />
        {monitor.manufacturer && (
          <Row label="Manufacturer" value={monitor.manufacturer} />
        )}
      </div>
    </div>
  );
}

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

// --- Mini layout preview ---

/** Compute a uniform scale that fits all layouts into the same coordinate space */
function computeUniformScale(layouts: Layout[], maxW: number, maxH: number): number {
  let globalMaxW = 0;
  let globalMaxH = 0;
  for (const layout of layouts) {
    const enabled = (layout.monitors ?? []).filter((m) => m.enabled);
    if (enabled.length === 0) continue;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const m of enabled) {
      minX = Math.min(minX, m.position_x);
      minY = Math.min(minY, m.position_y);
      maxX = Math.max(maxX, m.position_x + m.width);
      maxY = Math.max(maxY, m.position_y + m.height);
    }
    globalMaxW = Math.max(globalMaxW, maxX - minX);
    globalMaxH = Math.max(globalMaxH, maxY - minY);
  }
  if (globalMaxW <= 0 || globalMaxH <= 0) return 1;
  return Math.min(maxW / globalMaxW, maxH / globalMaxH);
}

function MiniLayoutPreview({ monitors, scale }: { monitors: LayoutMonitor[]; scale: number }) {
  const enabled = monitors.filter((m) => m.enabled);
  if (enabled.length === 0) return null;

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const m of enabled) {
    minX = Math.min(minX, m.position_x);
    minY = Math.min(minY, m.position_y);
    maxX = Math.max(maxX, m.position_x + m.width);
    maxY = Math.max(maxY, m.position_y + m.height);
  }

  const totalW = maxX - minX;
  const totalH = maxY - minY;
  if (totalW <= 0 || totalH <= 0) return null;

  const scaledW = totalW * scale;
  const scaledH = totalH * scale;

  return (
    <div
      className="relative mx-auto"
      style={{ width: scaledW, height: scaledH }}
    >
      {enabled.map((m, i) => {
        const x = (m.position_x - minX) * scale;
        const y = (m.position_y - minY) * scale;
        const w = m.width * scale;
        const h = m.height * scale;

        return (
          <div
            key={m.edid || m.port || i}
            className={`absolute rounded border ${m.primary ? `bg-blue-500/30 border-blue-500/40` : `border-zinc-600 bg-zinc-700/60`} overflow-hidden`}
            style={{ left: x, top: y, width: w, height: h }}
          >
            {m.primary && <>
              <span className="absolute top-0.5 right-1 leading-none text-blue-400 text-[7pt]">
                primary
              </span>
            </>}
            <span className="absolute top-0.5 left-1 text-zinc-400 font-mono leading-none text-[7pt]">
              {m.position_x},{m.position_y}
            </span>
            <span className="absolute bottom-0.5 left-1 text-zinc-400 font-mono leading-none text-[7pt]">
              {m.width}x{m.height}
            </span>
            <span className="absolute bottom-0.5 right-1 text-zinc-500 leading-none text-[7pt]">
              {m.edid}
            </span>
            <span className="absolute inset-0 flex items-center justify-center text-zinc-200 font-medium leading-none text-[10pt]">
              {m.name}
            </span>
          </div>
        );
      })}
    </div>
  );
}

/** Sort monitors: primary first, then left-to-right, top-to-bottom */
function sortedMonitors<T extends { primary?: boolean; position_x: number; position_y: number }>(monitors: T[]): T[] {
  return [...monitors].sort((a, b) => {
    // if (a.primary !== b.primary) return a.primary ? -1 : 1;
    if (a.position_x !== b.position_x) return a.position_x - b.position_x;
    return a.position_y - b.position_y;
  });
}

function LayoutCard({
  layout,
  isCurrent,
  disabled,
  scale,
  onClick,
  onDelete,
}: {
  layout: Layout;
  isCurrent: boolean;
  disabled: boolean;
  scale: number;
  onClick: () => void;
  onDelete?: () => void;
}) {
  const enabled = sortedMonitors((layout.monitors ?? []).filter((m) => m.enabled));
  const idAliases = [layout.id, ...(layout.aliases ?? [])].join(" \u00b7 ");

  return (
    <div className="relative group mb-auto flex-grow">
      {onDelete && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className="absolute -top-2 -right-2 w-6 h-6 rounded-full bg-zinc-800 border border-zinc-600 text-zinc-400 hover:text-red-400 hover:border-red-400 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-all z-20 shadow-lg cursor-pointer"
          title="Delete layout"
        >
          ×
        </button>
      )}
      <button
        onClick={onClick}
        disabled={disabled}
        className={`
          relative overflow-hidden rounded-xl text-sm font-medium transition-all cursor-pointer p-4 flex flex-col w-full gap-1.5 min-w-[180px] text-left
          ${isCurrent
            ? "bg-gradient-to-br from-blue-500/20 via-blue-500/10 to-indigo-500/20 text-blue-400 border border-blue-500/40 ring-1 ring-blue-500/20"
            : "bg-gradient-to-br from-zinc-800/80 via-zinc-800/50 to-zinc-900/80 text-zinc-300 border border-zinc-700/50 hover:border-zinc-600 hover:from-zinc-800 hover:to-zinc-800/80"
          }
          disabled:opacity-50 disabled:cursor-wait
        `}
      >
        {/* Header row: name left, emoji right */}
        <span className="flex items-start justify-between gap-3 w-full">
          <span className="flex flex-col gap-0.5">
            <span className="flex items-center gap-2">
              {isCurrent && (
                <span className="inline-block w-2 h-2 rounded-full bg-blue-400 shrink-0" />
              )}
              <span className="font-semibold">{layout.name}</span>
            </span>
            <span className="text-[10px] text-zinc-500 font-normal">{idAliases}</span>
          </span>
          {layout.emoji && <span className="text-lg leading-none">{layout.emoji}</span>}
        </span>

        {/* Monitor list */}
        {enabled.length > 0 && (
          <div className="flex flex-col gap-1 mt-2 w-full">
            {enabled.map((m, i) => (
              <div
                key={m.port || m.edid || i}
                className={`grid grid-cols-[auto_1fr_auto] items-center gap-2 text-[11px] px-2 py-1.5 rounded border ${m.primary
                    ? "bg-blue-500/10 border-blue-500/20"
                    : "bg-zinc-900/40 border-transparent"
                  }`}
              >
                <span className="truncate text-zinc-300 font-medium" title={m.name || m.port}>
                  {m.name || m.port || "Unknown"}
                </span>
                <span className="font-mono text-zinc-500">{m.width}x{m.height}</span>
                <span className="font-mono text-zinc-600 text-[10px]">{m.edid}</span>
              </div>
            ))}
          </div>
        )}
      </button>

      {/* Hover preview — below card with notch */}
      {enabled.length > 0 && (
        <div className="hidden sm:block absolute left-1/2 -translate-x-1/2 top-full mt-2 z-10 opacity-0 group-hover:opacity-100 pointer-events-none group-hover:pointer-events-auto transition-opacity duration-150">
          {/* Notch */}
          <div className="mx-auto w-0 h-0 border-l-[6px] border-l-transparent border-r-[6px] border-r-transparent border-b-[6px] border-b-zinc-700" />
          <div className="rounded-xl border border-zinc-700 bg-gradient-to-b from-zinc-900 to-zinc-950 p-3 shadow-2xl">
            <MiniLayoutPreview monitors={layout.monitors} scale={scale} />
          </div>
        </div>
      )}
    </div>
  );
}

// --- Login ---

function LoginForm({
  onSuccess,
}: {
  onSuccess: () => void;
}) {
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim() || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const res = await fetch("/api/auth", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: token.trim() }),
      });
      const data = await res.json();
      if (data.success) {
        onSuccess();
      } else {
        setError(data.message || "Authentication failed");
      }
    } catch {
      setError("Connection failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-zinc-950 flex items-center justify-center">
      <form onSubmit={submit} className="w-full max-w-sm px-6">
        <h1 className="text-3xl font-bold font-serif tracking-tight text-zinc-100">
          Ottoman
        </h1>
        <p className="text-zinc-500 text-sm mb-6">
          Enter your auth token to continue.
        </p>

        {error && (
          <div className="mb-4 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-sm px-4 py-3">
            {error}
          </div>
        )}

        <input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Auth token"
          autoFocus
          className="w-full rounded-lg border border-zinc-700 bg-zinc-800 px-4 py-2.5 text-sm text-zinc-100 placeholder-zinc-500 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        />

        <button
          type="submit"
          disabled={submitting || !token.trim()}
          className="mt-4 w-full rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
        >
          {submitting ? "Authenticating..." : "Log in"}
        </button>
      </form>
    </div>
  );
}

// --- App ---

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [monitors, setMonitors] = useState<MonitorInfo[]>([]);
  const [layouts, setLayouts] = useState<Layout[]>([]);
  const [currentLayout, setCurrentLayout] = useState("");

  const [switching, setSwitching] = useState(false);
  const [showSaveForm, setShowSaveForm] = useState(false);
  const [newLayoutName, setNewLayoutName] = useState("");
  const [newLayoutEmoji, setNewLayoutEmoji] = useState("");

  const [monitorsLoading, setMonitorsLoading] = useState(false);
  const [monitorsError, setMonitorsError] = useState<string | null>(null);
  const [layoutsLoading, setLayoutsLoading] = useState(false);
  const [layoutsError, setLayoutsError] = useState<string | null>(null);
  const [wakeTargets, setWakeTargets] = useState<WakeTarget[]>([]);
  const [wakeTargetsLoading, setWakeTargetsLoading] = useState(false);
  const [wakeTargetsError, setWakeTargetsError] = useState<string | null>(null);

  // Check auth on mount
  useEffect(() => {
    fetch("/api/auth/check")
      .then((res) => setAuthed(res.ok))
      .catch(() => setAuthed(false));
  }, []);

  const fetchWakeTargets = useCallback(async () => {
    setWakeTargetsLoading(true);
    try {
      const res = await fetch("/api/wake/targets");
      if (res.ok) {
        const targets = await res.json();
        setWakeTargets(targets);
        setWakeTargetsError(null);
      }
    } catch {
      setWakeTargetsError("Failed to load wake targets");
    } finally {
      setWakeTargetsLoading(false);
    }
  }, []);

  const fetchMonitors = useCallback(async () => {
    setMonitorsLoading(true);
    try {
      const monitorsData = await fetchJSON<MonitorInfo[]>("/api/monitors");
      setMonitors(sortedMonitors(monitorsData.filter((m) => m.connected)));
      setMonitorsError(null);
    } catch (e) {
      setMonitorsError("Failed to load monitors");
    } finally {
      setMonitorsLoading(false);
    }
  }, []);

  const fetchLayouts = useCallback(async () => {
    setLayoutsLoading(true);
    try {
      const layoutsData = await fetchJSON<LayoutsResponse>("/api/layouts");
      setLayouts(layoutsData.layouts ?? []);
      setCurrentLayout(layoutsData.current_layout ?? "");
      setLayoutsError(null);
    } catch (e) {
      setLayoutsError("Failed to load layouts");
    } finally {
      setLayoutsLoading(false);
    }
  }, []);

  const refresh = useCallback(() => {
    fetchWakeTargets();
    fetchMonitors();
    fetchLayouts();
  }, [fetchWakeTargets, fetchMonitors, fetchLayouts]);

  useEffect(() => {
    if (!authed) return;
    refresh();
    // Silent refresh every 10s
    const id = setInterval(() => {
      // We could implement silent refresh here, but for now we rely on manual refresh or initial load
      // to avoid flickering loading states.
    }, 10_000);
    return () => clearInterval(id);
  }, [authed, refresh]);

  const switchLayout = async (name: string) => {
    if (switching || name === currentLayout) return;
    setSwitching(true);
    try {
      const res = await fetch("/api/layouts/switch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ layout: name }),
      });
      const data: SwitchResponse = await res.json();
      if (data.success) {
        setCurrentLayout(data.current_layout);
        setTimeout(refresh, 1000);
      } else {
        alert(data.message || "Switch failed");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Switch failed");
    } finally {
      setSwitching(false);
    }
  };

  const saveCurrentLayout = async (name: string, emoji: string) => {
    try {
      const res = await fetch("/api/layouts/save-current", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, emoji }),
      });
      const data = await res.json();
      if (data.success) {
        setShowSaveForm(false);
        setNewLayoutName("");
        setNewLayoutEmoji("");
        refresh();
      } else {
        alert(data.message || "Failed to save layout");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to save layout");
    }
  };

  const removeLayout = async (id: string) => {
    if (!confirm("Are you sure you want to delete this layout?")) return;
    try {
      const res = await fetch("/api/layouts/remove", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ layout: id }),
      });
      const data = await res.json();
      if (data.success) {
        refresh();
      } else {
        alert(data.message || "Failed to remove layout");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to remove layout");
    }
  };

  const wake = async (target: string) => {
    try {
      const res = await fetch("/api/wake", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ target }),
      });
      const data = await res.json();
      if (!data.success) {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send wake packet");
    }
  };

  const logout = async () => {
    await fetch("/api/auth/logout", { method: "POST" });
    setAuthed(false);
    setMonitors([]);
    setLayouts([]);
    setCurrentLayout("");
  };

  // Auth check pending
  if (authed === null) {
    return (
      <div className="min-h-screen bg-zinc-950 flex items-center justify-center">
        <div className="text-zinc-500 text-sm">Loading...</div>
      </div>
    );
  }

  // Not authenticated — show login
  if (!authed) {
    return <LoginForm onSuccess={() => setAuthed(true)} />;
  }

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 overflow-x-hidden">
      <div className="max-w-4xl mx-auto px-6 py-10">
        {/* Header */}
        <header className="mb-10 flex items-center justify-between">
          <div className="flex items-center gap-4">
            <img src="/ottoman_logo.png" alt="Ottoman" className="h-14 w-auto" />
            <div>
              <h1 className="text-3xl font-bold font-serif tracking-tight">Ottoman</h1>
              <p className="text-zinc-500 italic text-sm">Display Management</p>
            </div>
          </div>
          <div className="flex items-center gap-4">
            <button
              onClick={refresh}
              className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
            >
              Refresh
            </button>
            <button
              onClick={logout}
              className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
            >
              Log out
            </button>
          </div>
        </header>

        {/* Wake on LAN */}
        <section className="mb-10">
          <h2 className="text-lg font-semibold text-zinc-200 mb-4">Wake on LAN</h2>
          {wakeTargetsLoading && wakeTargets.length === 0 ? (
            <div className="text-zinc-500 text-sm">Loading targets...</div>
          ) : wakeTargetsError ? (
            <div className="text-red-400 text-sm">{wakeTargetsError}</div>
          ) : wakeTargets.length === 0 ? (
            <div className="text-zinc-500 text-sm">No wake targets configured.</div>
          ) : (
            <div className="flex flex-wrap gap-3">
              {wakeTargets.map((target) => (
                <button
                  key={target.mac_address}
                  onClick={() => wake(target.name)}
                  className={`rounded-xl border p-4 transition-colors text-left min-w-[140px] ${target.status === 'online'
                      ? 'border-green-500/30 bg-green-500/10 hover:bg-green-500/20'
                      : target.status === 'offline'
                        ? 'border-red-500/30 bg-red-500/10 hover:bg-red-500/20'
                        : 'border-zinc-700/50 bg-zinc-800/50 hover:bg-zinc-800'
                    }`}
                >
                  <div className={`font-medium ${target.status === 'online' ? 'text-green-400' : target.status === 'offline' ? 'text-red-400' : 'text-zinc-200'}`}>
                    {target.name}
                  </div>
                  <div className="text-xs text-zinc-500 font-mono mt-1">{target.ip_address}</div>
                  <div className="text-[10px] text-zinc-600 font-mono">{target.mac_address}</div>
                </button>
              ))}
            </div>
          )}
        </section>

        {/* Layouts */}
        <section className="mb-10">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-zinc-200">Layouts</h2>
            {layouts.length > 0 && (
              <button
                onClick={() => setShowSaveForm(!showSaveForm)}
                className="text-xs bg-zinc-800 hover:bg-zinc-700 text-zinc-300 px-3 py-1.5 rounded-md transition-colors border border-zinc-700 cursor-pointer"
              >
                {showSaveForm ? "Cancel" : "Save Current"}
              </button>
            )}
          </div>

          {layoutsLoading && layouts.length === 0 ? (
            <div className="text-zinc-500 text-sm">Loading layouts...</div>
          ) : layoutsError ? (
            <div className="text-red-400 text-sm">{layoutsError}</div>
          ) : layouts.length === 0 ? (
            <div className="text-zinc-500 text-sm">No layouts found.</div>
          ) : (() => {
            const layoutScale = computeUniformScale(layouts, 500, 300);
            return (
              <>
                {showSaveForm && (
                  <div className="mb-6 p-4 rounded-xl border border-zinc-700 bg-zinc-800/50 flex flex-col sm:flex-row gap-3 items-end sm:items-center">
                    <div className="flex-1 w-full">
                      <label className="block text-xs text-zinc-500 mb-1">Name</label>
                      <input
                        type="text"
                        value={newLayoutName}
                        onChange={(e) => setNewLayoutName(e.target.value)}
                        className="w-full rounded-md border border-zinc-600 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 focus:outline-none focus:border-blue-500"
                        placeholder="My Layout"
                      />
                    </div>
                    <div className="w-full sm:w-24">
                      <label className="block text-xs text-zinc-500 mb-1">Emoji</label>
                      <input
                        type="text"
                        value={newLayoutEmoji}
                        onChange={(e) => setNewLayoutEmoji(e.target.value)}
                        className="w-full rounded-md border border-zinc-600 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 focus:outline-none focus:border-blue-500"
                        placeholder="🖥️"
                      />
                    </div>
                    <button
                      onClick={() => saveCurrentLayout(newLayoutName, newLayoutEmoji)}
                      disabled={!newLayoutName.trim()}
                      className="w-full sm:w-auto rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                    >
                      Save
                    </button>
                  </div>
                )}

                <div className="flex flex-wrap gap-3">
                  {layouts.map((l) => (
                    <LayoutCard
                      key={l.id}
                      layout={l}
                      isCurrent={l.id === currentLayout}
                      disabled={switching}
                      scale={layoutScale}
                      onClick={() => switchLayout(l.id)}
                      onDelete={() => removeLayout(l.id)}
                    />
                  ))}
                </div>
              </>
            );
          })()}
        </section>

        {/* Monitors */}
        <section>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-zinc-200">Monitors</h2>
            <span className="text-xs text-zinc-500">
              {monitors.length} connected
            </span>
          </div>
          {monitorsLoading && monitors.length === 0 ? (
            <div className="text-zinc-500 text-sm">Loading monitors...</div>
          ) : monitorsError ? (
            <div className="text-red-400 text-sm">{monitorsError}</div>
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
      </div>
    </div>
  );
}
