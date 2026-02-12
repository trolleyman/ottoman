import { useState, useEffect, useCallback, useRef } from "react";
import { MonitorDisplay } from "./MonitorDisplay";
import type { Layout, LayoutsResponse } from "./types";
import { fetchJSON, sortedLayouts } from "./utils";

export function useTrackpadWebSocket(authed: boolean) {
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

function TouchArea({
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
      className={`w-full aspect-square md:max-w-sm md:shrink-0 rounded-xl border-2 transition-colors select-none touch-none ${connected
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

export function Trackpad({
  authed,
  refreshSignal,
}: {
  authed: boolean;
  refreshSignal: { key: number; silent: boolean };
}) {
  const { connected, cursorPos, send } = useTrackpadWebSocket(authed);
  const [layouts, setLayouts] = useState<Layout[]>([]);
  const [currentLayout, setCurrentLayout] = useState("");
  const [loading, setLoading] = useState(false);

  const fetchLayouts = useCallback(async (silent: boolean) => {
    if (!authed) return;
    if (!silent) setLoading(true);
    try {
      const layoutsData = await fetchJSON<LayoutsResponse>("/api/layouts");
      setLayouts(sortedLayouts(layoutsData.layouts ?? []));
      setCurrentLayout(layoutsData.current_layout ?? "");
    } catch {
      setLayouts([]);
      setCurrentLayout("");
    } finally {
      setLoading(false);
    }
  }, [authed]);

  useEffect(() => {
    fetchLayouts(refreshSignal.silent);
  }, [fetchLayouts, refreshSignal]);

  return (
    <section>
      <h2 className="text-lg font-semibold text-zinc-200 mb-4 flex items-center gap-2">
        Trackpad
        <span
          className={`inline-block w-2 h-2 rounded-full ${
            connected ? "bg-green-400" : "bg-red-400"
          }`}
        />
      </h2>
      <div className="flex flex-col-reverse md:flex-row gap-6 sm:items-start">
        <TouchArea connected={connected} send={send} />
        <MonitorDisplay
          layouts={layouts}
          currentLayout={currentLayout}
          cursorPos={cursorPos}
          connected={connected}
          loading={loading}
        />
      </div>
    </section>
  );
}
