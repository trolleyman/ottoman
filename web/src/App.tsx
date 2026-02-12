import { useState, useEffect, useCallback, useRef } from "react";

// --- Types matching Go API responses ---

interface MonitorInfo {
  edid: string;
  port: string;
  name: string;
  manufacturer: string;
  active?: ActiveMonitorInfo;
}

interface ActiveMonitorInfo {
  width: number;
  height: number;
  refresh_rate: number;
  position_x: number;
  position_y: number;
  primary: boolean;
  model: string;
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

function OttomanTitle() {
  return <h1 className="text-3xl font-bold font-serif tracking-[-0.075em] text-zinc-100">
    Ottoman
  </h1>
}

type OttomanWithLogoProps = React.PropsWithChildren<{
  className?: string;
}>;

function OttomanWithLogo({ children, className }: OttomanWithLogoProps) {
  return <>
    <div className={`flex items-center gap-4 ${className}`}>
      <picture>
        <source srcSet="/ottoman_logo.avif" type="image/avif" />
        <source srcSet="/ottoman_logo.webp" type="image/webp" />
        <img src="/ottoman_logo.png" alt="Ottoman" className="h-14 w-auto" />
      </picture>
      <div>
        <OttomanTitle />
        {children}
      </div>
    </div>
  </>
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
    if ((layout.monitors ?? []).length === 0) continue;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const m of layout.monitors) {
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
  if (monitors.length === 0) return null;

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const m of monitors) {
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
      {monitors.map((m, i) => {
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

/** Sort layouts: #, if one of the aliases is a number, then by ID */
function sortedLayouts(layouts: Layout[]): Layout[] {
  return [...layouts].sort((a, b) => {
    const aNum = a.aliases?.find((alias) => !isNaN(Number(alias)));
    const bNum = b.aliases?.find((alias) => !isNaN(Number(alias)));
    if (aNum && bNum) {
      const aNumVal = Number(aNum);
      const bNumVal = Number(bNum);
      if (aNumVal !== bNumVal) return aNumVal - bNumVal;
    }
    if (a.id !== b.id) return a.id.localeCompare(b.id);
    return 0;
  });
}


/** Sort monitors: active first, then left-to-right, top-to-bottom */
function sortedMonitors(monitors: MonitorInfo[]): MonitorInfo[] {
  return [...monitors].sort((a, b) => {
    // Active monitors before inactive
    if (!!a.active !== !!b.active) return a.active ? -1 : 1;
    const ax = a.active?.position_x ?? 0;
    const bx = b.active?.position_x ?? 0;
    if (ax !== bx) return ax - bx;
    return (a.active?.position_y ?? 0) - (b.active?.position_y ?? 0);
  });
}

/** Sort layout monitors: left-to-right, top-to-bottom */
function sortedLayoutMonitors(monitors: LayoutMonitor[]): LayoutMonitor[] {
  return [...monitors].sort((a, b) => {
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
  const enabled = sortedLayoutMonitors(layout.monitors ?? []);
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
        <OttomanWithLogo className="mb-4">
          <p className="text-zinc-500 text-sm">
            Enter your auth token to continue.
          </p>
        </OttomanWithLogo>

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

// --- Trackpad ---

function useTrackpadWebSocket(authed: boolean) {
  const [connected, setConnected] = useState(false);
  const [cursorPos, setCursorPos] = useState<{ x: number; y: number } | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const connect = useCallback(() => {
    if (!authed) return;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/trackpad`);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => {
      setConnected(false);
      setCursorPos(null);
      reconnectRef.current = setTimeout(connect, 3000);
    };
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.t === "p") {
          setCursorPos({ x: msg.x ?? 0, y: msg.y ?? 0 });
        }
      } catch { /* ignore parse errors */ }
    };
  }, [authed]);

  useEffect(() => {
    connect();
    return () => {
      wsRef.current?.close();
      if (reconnectRef.current) clearTimeout(reconnectRef.current);
    };
  }, [connect]);

  const send = useCallback((msg: object) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  return { connected, cursorPos, send };
}

function Trackpad({
  connected,
  send,
}: {
  connected: boolean;
  send: (msg: object) => void;
}) {
  const trackpadRef = useRef<HTMLDivElement>(null);
  const lastTouchRef = useRef<{ x: number; y: number } | null>(null);
  const lastMoveTime = useRef(0);
  const pointerActive = useRef(false);

  const onTouchStart = (e: React.TouchEvent) => {
    e.preventDefault();
    const touch = e.touches[0];
    lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
    send({ t: "s", touch: true });
  };

  const onTouchMove = (e: React.TouchEvent) => {
    e.preventDefault();
    const now = performance.now();
    if (now - lastMoveTime.current < 16) return;
    lastMoveTime.current = now;

    const touch = e.touches[0];
    if (lastTouchRef.current) {
      const dx = touch.clientX - lastTouchRef.current.x;
      const dy = touch.clientY - lastTouchRef.current.y;
      send({ t: "m", dx, dy });
      lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
    }
  };

  const onTouchEnd = (e: React.TouchEvent) => {
    e.preventDefault();
    lastTouchRef.current = null;
    send({ t: "e" });
  };

  // Mouse drag via Pointer Lock: cursor stays locked inside the trackpad div
  const onPointerDown = (e: React.PointerEvent) => {
    if (e.pointerType === "touch") return;
    pointerActive.current = true;
    send({ t: "s", touch: false });
    trackpadRef.current?.requestPointerLock();
  };

  useEffect(() => {
    if (!connected) return;

    const handleMouseMove = (e: MouseEvent) => {
      if (!pointerActive.current) return;
      const now = performance.now();
      if (now - lastMoveTime.current < 16) return;
      lastMoveTime.current = now;
      send({ t: "m", dx: e.movementX, dy: e.movementY });
    };

    const handleMouseUp = () => {
      if (!pointerActive.current) return;
      pointerActive.current = false;
      document.exitPointerLock();
      send({ t: "e" });
    };

    const handlePointerLockChange = () => {
      if (!document.pointerLockElement && pointerActive.current) {
        pointerActive.current = false;
        send({ t: "e" });
      }
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    document.addEventListener("pointerlockchange", handlePointerLockChange);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      document.removeEventListener("pointerlockchange", handlePointerLockChange);
      if (document.pointerLockElement) document.exitPointerLock();
    };
  }, [connected, send]);

  return (
    <div
      ref={trackpadRef}
      className={`w-full aspect-square sm:max-w-sm sm:shrink-0 rounded-xl border-2 transition-colors select-none touch-none ${connected
          ? "border-zinc-700 bg-zinc-900/80 cursor-crosshair"
          : "border-red-500/50 bg-zinc-900/40 pointer-events-none opacity-50"
        }`}
      style={connected ? {
        backgroundImage: "radial-gradient(circle, rgba(63,63,70,0.3) 1px, transparent 1px)",
        backgroundSize: "20px 20px",
      } : undefined}
      onTouchStart={connected ? onTouchStart : undefined}
      onTouchMove={connected ? onTouchMove : undefined}
      onTouchEnd={connected ? onTouchEnd : undefined}
      onPointerDown={connected ? onPointerDown : undefined}
    >
      {!connected && (
        <div className="flex items-center justify-center h-full text-zinc-500 text-sm">
          Disconnected
        </div>
      )}
    </div>
  );
}

function CursorPositionDisplay({
  layouts,
  currentLayout,
  cursorPos,
  connected,
}: {
  layouts: Layout[];
  currentLayout: string;
  cursorPos: { x: number; y: number } | null;
  connected: boolean;
}) {
  const [containerWidth, setContainerWidth] = useState(0);
  const observerRef = useRef<ResizeObserver | null>(null);

  const containerRef = useCallback((el: HTMLDivElement | null) => {
    if (observerRef.current) {
      observerRef.current.disconnect();
      observerRef.current = null;
    }
    if (!el) return;
    const observer = new ResizeObserver((entries) => {
      setContainerWidth(entries[0].contentRect.width);
    });
    observer.observe(el);
    observerRef.current = observer;
  }, []);

  useEffect(() => {
    return () => { observerRef.current?.disconnect(); };
  }, []);

  if (!connected || !cursorPos) return null;

  const layout = layouts.find((l) => l.id === currentLayout) ?? layouts[0];
  const monitors = layout?.monitors ?? [];
  if (monitors.length === 0) return null;

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const m of monitors) {
    minX = Math.min(minX, m.position_x);
    minY = Math.min(minY, m.position_y);
    maxX = Math.max(maxX, m.position_x + m.width);
    maxY = Math.max(maxY, m.position_y + m.height);
  }
  const totalW = maxX - minX;
  const totalH = maxY - minY;
  if (totalW <= 0 || totalH <= 0) return null;

  // Use measured width, fall back to 300px on first frame before ResizeObserver fires
  const effectiveWidth = containerWidth || 300;
  const maxH = 250;
  const scale = Math.min(effectiveWidth / totalW, maxH / totalH);
  const dotX = (cursorPos.x - minX) * scale;
  const dotY = (cursorPos.y - minY) * scale;

  return (
    <div ref={containerRef} className="flex-1 min-w-0 w-full flex flex-col items-center gap-1">
      <div className="relative">
        <MiniLayoutPreview monitors={monitors} scale={scale} />
        <div
          className="absolute w-2 h-2 rounded-full bg-red-500 -translate-x-1/2 -translate-y-1/2 z-10 shadow-[0_0_4px_rgba(239,68,68,0.7)]"
          style={{ left: dotX, top: dotY }}
        />
      </div>
      <span className="text-[10px] text-zinc-500 font-mono">
        {cursorPos.x}, {cursorPos.y}
      </span>
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
  const [wakingTargets, setWakingTargets] = useState<Set<string>>(new Set());
  const [shuttingDownTargets, setShuttingDownTargets] = useState<Set<string>>(new Set());

  const { connected: trackpadConnected, cursorPos, send: trackpadSend } = useTrackpadWebSocket(!!authed);

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
      setMonitors(sortedMonitors(monitorsData));
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
      setLayouts(sortedLayouts(layoutsData.layouts ?? []));
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
  }, [authed, refresh]);

  // Poll wake targets while any target is waking or shutting down
  useEffect(() => {
    if (wakingTargets.size === 0 && shuttingDownTargets.size === 0) return;

    const poll = async () => {
      try {
        const res = await fetch("/api/wake/targets");
        if (res.ok) {
          const targets: WakeTarget[] = await res.json();
          setWakeTargets(targets);

          // Clear waking state for targets that came online
          const nowOnline = targets.filter((t) => t.status === "online").map((t) => t.name);
          setWakingTargets((prev) => {
            const next = new Set(prev);
            let changed = false;
            for (const name of nowOnline) {
              if (next.delete(name)) changed = true;
            }
            if (!changed) return prev;
            if (next.size === 0) {
              // Client just came online — refresh everything
              fetchMonitors();
              fetchLayouts();
            }
            return next;
          });

          // Clear shutting down state for targets that went offline
          const nowOffline = targets.filter((t) => t.status === "offline").map((t) => t.name);
          setShuttingDownTargets((prev) => {
            const next = new Set(prev);
            let changed = false;
            for (const name of nowOffline) {
              if (next.delete(name)) changed = true;
            }
            if (!changed) return prev;
            if (next.size === 0) {
              // Client just went offline — refresh everything
              fetchMonitors();
              fetchLayouts();
            }
            return next;
          });
        }
      } catch { /* ignore polling errors */ }
    };

    const id = setInterval(poll, 3000);
    poll(); // immediate first poll
    return () => clearInterval(id);
  }, [wakingTargets, shuttingDownTargets, fetchMonitors, fetchLayouts]);

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
      if (data.success) {
        setWakingTargets((prev) => new Set(prev).add(target));
      } else {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send wake packet");
    }
  };

  const shutdown = async (target: string) => {
    if (!confirm(`Are you sure you want to shut down ${target}?`)) return;
    try {
      const res = await fetch("/api/shutdown", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
      });
      const data = await res.json();
      if (data.success) {
        setShuttingDownTargets((prev) => new Set(prev).add(target));
      } else {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send shutdown command");
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
      <div className="max-w-4xl mx-auto px-6 py-10 space-y-10">
        {/* Header */}
        <header className="flex items-center justify-between">
          <OttomanWithLogo>
            <p className="text-zinc-500 italic text-sm">Display Management</p>
          </OttomanWithLogo>
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
        <section>
          <h2 className="text-lg font-semibold text-zinc-200 mb-4">Wake on LAN</h2>
          {wakeTargetsLoading && wakeTargets.length === 0 ? (
            <div className="text-zinc-500 text-sm">Loading targets...</div>
          ) : wakeTargetsError ? (
            <div className="text-red-400 text-sm">{wakeTargetsError}</div>
          ) : wakeTargets.length === 0 ? (
            <div className="text-zinc-500 text-sm">No wake targets configured.</div>
          ) : (
            <div className="flex flex-wrap gap-3">
              {wakeTargets.map((target) => {
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

        {/* Layouts */}
        <section>
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
              {monitors.filter((m) => m.active).length} active / {monitors.length} total
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

        {/* Trackpad */}
        <section>
          <h2 className="text-lg font-semibold text-zinc-200 mb-4 flex items-center gap-2">
            Trackpad
            <span className={`inline-block w-2 h-2 rounded-full ${trackpadConnected ? "bg-green-400" : "bg-red-400"
              }`} />
          </h2>
          <div className="flex flex-col-reverse sm:flex-row gap-6 sm:items-start">
            <Trackpad connected={trackpadConnected} send={trackpadSend} />
            <CursorPositionDisplay
              layouts={layouts}
              currentLayout={currentLayout}
              cursorPos={cursorPos}
              connected={trackpadConnected}
            />
          </div>
        </section>
      </div>
    </div>
  );
}
