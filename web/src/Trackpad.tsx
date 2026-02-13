import { useState, useEffect, useCallback, useRef } from "react";
import { MonitorDisplay } from "./MonitorDisplay";
import type { Layout, LayoutsResponse, TrackpadRecvArgs, TrackpadSendArgs } from "./types";
import { fetchJSON, sortedLayouts } from "./utils";

export function useTrackpadWebSocket(authed: boolean, refreshKey: number) {
  const [connected, setConnected] = useState(false);
  const [cursorPos, setCursorPos] = useState<{ x: number; y: number } | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const connect = useCallback(() => {
    if (!authed) return;

    if (reconnectRef.current) {
      clearTimeout(reconnectRef.current);
      reconnectRef.current = null;
    }
    if (wsRef.current) {
      wsRef.current.onclose = null;
      wsRef.current.close();
    }

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
        const msg: TrackpadRecvArgs = JSON.parse(e.data);
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

  useEffect(() => {
    if (!connected && authed) {
      connect();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshKey]);

  const send = useCallback((msg: TrackpadSendArgs) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  return { connected, cursorPos, send };
}

const SPECIAL_KEYS = new Set([
  "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight",
  "Enter", "Tab", "Escape", "Backspace", "Delete",
  "Home", "End", "PageUp", "PageDown", "Insert",
  "F1", "F2", "F3", "F4", "F5", "F6",
  "F7", "F8", "F9", "F10", "F11", "F12",
  " ", "PrintScreen", "ScrollLock", "Pause", "NumLock", "CapsLock",
]);

function TouchArea({
  connected,
  send,
  silent,
}: {
  connected: boolean;
  send: (msg: TrackpadSendArgs) => void;
  silent: boolean;
}) {
  const trackpadRef = useRef<HTMLDivElement>(null);
  const lastTouchRef = useRef<{ x: number; y: number } | null>(null);
  const lastMoveTime = useRef(0);
  const touchStartTime = useRef(0);
  const touchStartPos = useRef<{ x: number; y: number } | null>(null);
  const lastTouchEndTime = useRef(0);
  const isDragging = useRef(false);
  const pointerLocked = useRef(false);
  const mouseHeld = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const [focused, setFocused] = useState(false);

  // Two-finger scroll refs
  const twoFingerRef = useRef(false);
  const twoFingerLastX = useRef(0);
  const twoFingerLastY = useRef(0);

  const onTouchStart = (e: React.TouchEvent) => {
    e.preventDefault();

    // Two-finger scroll detection
    if (e.touches.length === 2) {
      twoFingerRef.current = true;
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
      twoFingerLastX.current = midX;
      twoFingerLastY.current = midY;
      return;
    }

    twoFingerRef.current = false;
    const touch = e.touches[0];
    touchStartTime.current = performance.now();

    // Check for double-tap-drag start
    if (touchStartTime.current - lastTouchEndTime.current < 300) {
      isDragging.current = true;
      send({ t: "d" });
    }

    touchStartPos.current = { x: touch.clientX, y: touch.clientY };
    lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
    send({ t: "s", touch: true });
    // Focus hidden input for on-screen keyboard on mobile
    inputRef.current?.focus();
  };

  const onTouchMove = (e: React.TouchEvent) => {
    e.preventDefault();
    const now = performance.now();
    if (now - lastMoveTime.current < 16) return;
    lastMoveTime.current = now;

    // Two-finger scroll
    if (e.touches.length === 2 && twoFingerRef.current) {
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
      const dx = midX - twoFingerLastX.current;
      const dy = midY - twoFingerLastY.current;
      twoFingerLastX.current = midX;
      twoFingerLastY.current = midY;
      if (Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5) {
        send({ t: "sc", dx: Math.round(dx), dy: Math.round(-dy), precise: true });
      }
      return;
    }

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

    // Two-finger scroll end
    if (twoFingerRef.current) {
      if (e.touches.length === 0) {
        twoFingerRef.current = false;
      }
      return;
    }

    const now = performance.now();
    lastTouchEndTime.current = now;

    if (isDragging.current) {
      send({ t: "u" });
      isDragging.current = false;
    } else {
      if (touchStartPos.current && now - touchStartTime.current < 200) {
        const touch = e.changedTouches[0];
        const dx = touch.clientX - touchStartPos.current.x;
        const dy = touch.clientY - touchStartPos.current.y;
        if (Math.sqrt(dx * dx + dy * dy) < 10) {
          send({ t: "c" });
        }
      }
    }

    touchStartPos.current = null;
    lastTouchRef.current = null;
    send({ t: "e" });
  };

  // Mouse drag via Pointer Lock: cursor stays locked inside the trackpad div
  const onPointerDown = (e: React.PointerEvent) => {
    if (e.pointerType === "touch") return;

    // Middle click
    if (e.button === 1) {
      e.preventDefault();
      send({ t: "c", btn: "middle" });
      return;
    }

    // Right click handled by onContextMenu
    if (e.button === 2) return;

    // Left click / drag
    pointerActive.current = true;
    send({ t: "s", touch: false });
    trackpadRef.current?.requestPointerLock();
    // Focus trackpad div for desktop keyboard capture
    trackpadRef.current?.focus();
  };

  // Pointer lock mouse movement and release
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

  // Keyboard capture on the trackpad div
  useEffect(() => {
    const el = trackpadRef.current;
    if (!el || !connected) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      const hasModifier = e.ctrlKey || e.altKey || e.metaKey;
      const isSpecial = SPECIAL_KEYS.has(e.key);

      if (isSpecial || hasModifier) {
        e.preventDefault();
        e.stopPropagation();

        const mod: string[] = [];
        if (e.shiftKey) mod.push("shift");
        if (e.ctrlKey) mod.push("ctrl");
        if (e.altKey) mod.push("alt");
        if (e.metaKey) mod.push("meta");

        send({ t: "key", key: e.key, mod: mod.length > 0 ? mod : undefined });
      }
      // Regular characters without ctrl/alt/meta fall through to hidden input onChange
    };

    el.addEventListener("keydown", handleKeyDown);
    return () => el.removeEventListener("keydown", handleKeyDown);
  }, [connected, send]);

  // Wheel scroll handler (passive: false to allow preventDefault)
  useEffect(() => {
    const el = trackpadRef.current;
    if (!el || !connected) return;

    const handleWheel = (e: WheelEvent) => {
      e.preventDefault();
      // deltaMode: 0 = pixels (trackpads), 1 = lines (mouse wheels), 2 = pages
      if (e.deltaMode === 1) {
        // Line-based scrolling (mouse wheel)
        const dx = Math.round(e.deltaX);
        const dy = Math.round(e.deltaY);
        if (dx !== 0 || dy !== 0) {
          send({ t: "sc", dx, dy });
        }
      } else if (e.deltaMode === 2) {
        // Page-based: treat as lines with a multiplier
        const dx = Math.round(e.deltaX * 10);
        const dy = Math.round(e.deltaY * 10);
        if (dx !== 0 || dy !== 0) {
          send({ t: "sc", dx, dy });
        }
      } else {
        // Pixel-based (trackpads, smooth scrolling)
        const dx = Math.round(e.deltaX);
        const dy = Math.round(e.deltaY);
        if (dx !== 0 || dy !== 0) {
          send({ t: "sc", dx, dy, precise: true });
        }
      }
    };

    el.addEventListener("wheel", handleWheel, { passive: false });
    return () => el.removeEventListener("wheel", handleWheel);
  }, [connected, send]);

  return (
    <div
      ref={trackpadRef}
      tabIndex={0}
      className={`w-full aspect-square md:max-w-sm md:shrink-0 rounded-xl border-2 transition-colors select-none touch-none outline-none ${connected
          ? focused
            ? "border-blue-500 ring-2 ring-blue-500/30 bg-zinc-900/80 cursor-crosshair"
            : "border-zinc-700 bg-zinc-900/80 cursor-crosshair"
          : "border-red-500/50 bg-zinc-900/40 pointer-events-none opacity-50"
        }`}
      style={connected ? {
        backgroundImage: "radial-gradient(circle, rgba(63,63,70,0.3) 1px, transparent 1px)",
        backgroundSize: "20px 20px",
      } : undefined}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      onTouchStart={connected ? onTouchStart : undefined}
      onTouchMove={connected ? onTouchMove : undefined}
      onTouchEnd={connected ? onTouchEnd : undefined}
      onPointerDown={connected ? onPointerDown : undefined}
      onContextMenu={(e) => {
        e.preventDefault();
        if (connected) send({ t: "c", btn: "right" });
      }}
    >
      <input
        ref={inputRef}
        type="text"
        className="opacity-0 fixed top-0 left-0 h-0 w-0 pointer-events-none"
        autoComplete="off"
        onChange={(e) => {
          if (e.target.value) send({ t: "k", text: e.target.value });
          e.target.value = "";
        }}
      />
      {!connected && (
        <div className="flex flex-col items-center justify-center h-full text-zinc-500 text-sm gap-2">
          {!silent ? (
            <>
              <div className="w-5 h-5 border-2 border-zinc-600 border-t-zinc-400 rounded-full animate-spin" />
              <span>Connecting...</span>
            </>
          ) : (
            <span>Disconnected</span>
          )}
        </div>
      )}
    </div>
  );
}

export function Trackpad({
  authed,
  refreshSignal,
  connected,
  cursorPos,
  send,
}: {
  authed: boolean;
  refreshSignal: { key: number; silent: boolean };
  connected: boolean;
  cursorPos: { x: number; y: number } | null;
  send: (msg: TrackpadSendArgs) => void;
}) {
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
        <TouchArea connected={connected} send={send} silent={refreshSignal.silent} />
        <MonitorDisplay
          layouts={layouts}
          currentLayout={currentLayout}
          cursorPos={cursorPos}
          connected={connected}
          loading={loading}
          onSetPosition={(x, y) => send({ t: "a", x, y })}
        />
      </div>
    </section>
  );
}
