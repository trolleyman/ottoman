import { useState, useEffect, useCallback, useRef } from "react";
import { MonitorDisplay } from "./MonitorDisplay";
import type { Layout, LayoutsResponse, Modifier, StatusResponse, TrackpadRecvArgs, TrackpadSendArgs } from "./types";
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

    ws.onopen = () => {
      // Debounce: only report connected after WS stays open 500ms.
      // Prevents flicker when server accepts but immediately closes
      // because the client is unreachable.
      const timer = setTimeout(() => setConnected(true), 500);
      ws.addEventListener("close", () => clearTimeout(timer), { once: true });
    };
    ws.onclose = () => {
      setConnected(false);
      setCursorPos(null);
      // Attempt reconnect after 3 seconds
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

function getModifiers(e: { shiftKey: boolean; ctrlKey: boolean; altKey: boolean; metaKey: boolean }) {
  const mod: Modifier[] = [];
  if (e.shiftKey) mod.push("shift");
  if (e.ctrlKey) mod.push("ctrl");
  if (e.altKey) mod.push("alt");
  if (e.metaKey) mod.push("meta");
  return mod.length > 0 ? mod : undefined;
}

interface TrackpadSettings {
  cursorSensitivity: number;
  cursorFriction: number;
  scrollSensitivity: number;
  scrollFriction: number;
  clickAndDrag: boolean;
}

function TouchArea({
  connected,
  send,
  silent,
  settings,
}: {
  connected: boolean;
  send: (msg: TrackpadSendArgs) => void;
  silent: boolean;
  settings: TrackpadSettings;
}) {
  const trackpadRef = useRef<HTMLDivElement>(null);
  const lastTouchRef = useRef<{ x: number; y: number } | null>(null);
  const lastMoveTime = useRef(0);
  const touchStartTime = useRef(0);
  const touchStartPos = useRef<{ x: number; y: number } | null>(null);
  const lastTouchEndTime = useRef(0);
  const isDragging = useRef(false);
  const mouseHeld = useRef(false);
  const dragLocked = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const [focused, setFocused] = useState(false);

  // Two-finger scroll refs
  const twoFingerRef = useRef(false);
  const twoFingerLastX = useRef(0);
  const twoFingerLastY = useRef(0);

  const lastTapPos = useRef<{ x: number; y: number } | null>(null);
  const scrollVelocity = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const inertiaFrame = useRef(0);
  const lastScrollTime = useRef(0);

  // Cursor inertia refs
  const cursorVelocity = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const cursorInertiaFrame = useRef(0);
  const lastCursorTime = useRef(0);

  const handleKey = useCallback((e: React.KeyboardEvent | KeyboardEvent) => {
    e.preventDefault();
    e.stopPropagation();

    send({ t: "key", key: e.key, mod: getModifiers(e) });
  }, [send]);

  const onTouchStart = (e: React.TouchEvent) => {
    e.preventDefault();

    // Stop any active inertia scrolling when the user touches the screen again
    if (inertiaFrame.current) {
      cancelAnimationFrame(inertiaFrame.current);
      inertiaFrame.current = 0;
    }
    if (cursorInertiaFrame.current) {
      cancelAnimationFrame(cursorInertiaFrame.current);
      cursorInertiaFrame.current = 0;
    }

    // Two-finger scroll detection
    if (e.touches.length === 2) {
      twoFingerRef.current = true;
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
      twoFingerLastX.current = midX;
      twoFingerLastY.current = midY;
      scrollVelocity.current = { x: 0, y: 0 };
      lastScrollTime.current = performance.now();
      return;
    }

    twoFingerRef.current = false;
    const touch = e.touches[0];
    touchStartTime.current = performance.now();
    cursorVelocity.current = { x: 0, y: 0 };
    lastCursorTime.current = performance.now();

    // Check for double-tap-drag start
    // We check distance to ensure the second tap is close to the first one (preventing accidental drags on fast moves)
    const dist = lastTapPos.current
      ? Math.hypot(touch.clientX - lastTapPos.current.x, touch.clientY - lastTapPos.current.y)
      : Infinity;

    if (touchStartTime.current - lastTouchEndTime.current < 300 && dist < 40) {
      isDragging.current = true;
      send({ t: "d" });
    }

    lastTapPos.current = { x: touch.clientX, y: touch.clientY };
    touchStartPos.current = { x: touch.clientX, y: touch.clientY };
    lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
    // Focus hidden input for on-screen keyboard on mobile
    inputRef.current?.focus();
    trackpadRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
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
      const dx = (midX - twoFingerLastX.current) * settings.scrollSensitivity;
      const dy = (midY - twoFingerLastY.current) * settings.scrollSensitivity;
      twoFingerLastX.current = midX;
      twoFingerLastY.current = midY;

      const dt = now - lastScrollTime.current;
      if (dt > 0) {
        // Calculate velocity for inertia (pixels per ms)
        const vx = dx / dt;
        const vy = dy / dt;
        // Simple low-pass filter to smooth velocity
        const alpha = 0.5;
        scrollVelocity.current = {
          x: scrollVelocity.current.x * alpha + vx * (1 - alpha),
          y: scrollVelocity.current.y * alpha + vy * (1 - alpha),
        };
        lastScrollTime.current = now;
      }

      if (Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5) {
        send({ t: "sc", dx: Math.round(dx), dy: Math.round(-dy), precise: true });
      }
      return;
    }

    const touch = e.touches[0];
    if (lastTouchRef.current) {
      const rawDx = touch.clientX - lastTouchRef.current.x;
      const rawDy = touch.clientY - lastTouchRef.current.y;
      const dx = rawDx * settings.cursorSensitivity;
      const dy = rawDy * settings.cursorSensitivity;

      send({ t: "m", dx, dy });
      lastTouchRef.current = { x: touch.clientX, y: touch.clientY };

      const dt = now - lastCursorTime.current;
      if (dt > 0) {
        const vx = dx / dt;
        const vy = dy / dt;
        const alpha = 0.5;
        cursorVelocity.current = { x: cursorVelocity.current.x * alpha + vx * (1 - alpha), y: cursorVelocity.current.y * alpha + vy * (1 - alpha) };
        lastCursorTime.current = now;
      }
    }
  };

  const onTouchEnd = (e: React.TouchEvent) => {
    e.preventDefault();

    // Two-finger scroll end
    if (twoFingerRef.current) {
      if (e.touches.length === 0) {
        twoFingerRef.current = false;

        const { x: vx, y: vy } = scrollVelocity.current;
        const timeSinceLastScroll = performance.now() - lastScrollTime.current;

        // Only trigger inertia if the user was scrolling recently and with sufficient velocity
        if (timeSinceLastScroll < 50 && (Math.abs(vx) > 0.1 || Math.abs(vy) > 0.1)) {
          let cx = vx;
          let cy = vy;
          let lastT = performance.now();

          const step = () => {
            const t = performance.now();
            const dt = t - lastT;
            lastT = t;

            // Apply friction to velocity
            const friction = Math.pow(settings.scrollFriction, dt / 16);
            cx *= friction;
            cy *= friction;

            // Stop when velocity is negligible
            if (Math.abs(cx) < 0.05 && Math.abs(cy) < 0.05) {
              inertiaFrame.current = 0;
              return;
            }

            const dx = cx * dt;
            const dy = cy * dt;
            send({ t: "sc", dx: Math.round(dx), dy: Math.round(-dy), precise: true });
            inertiaFrame.current = requestAnimationFrame(step);
          };
          inertiaFrame.current = requestAnimationFrame(step);
        }
      }
      return;
    }

    const now = performance.now();
    lastTouchEndTime.current = now;

    if (isDragging.current) {
      send({ t: "u" });
      isDragging.current = false;
    } else {
      // Cursor inertia
      const { x: vx, y: vy } = cursorVelocity.current;
      const timeSinceLastMove = performance.now() - lastCursorTime.current;

      if (timeSinceLastMove < 50 && (Math.abs(vx) > 0.1 || Math.abs(vy) > 0.1)) {
        let cx = vx;
        let cy = vy;
        let lastT = performance.now();

        const step = () => {
          const t = performance.now();
          const dt = t - lastT;
          lastT = t;

          const friction = Math.pow(settings.cursorFriction, dt / 16);
          cx *= friction;
          cy *= friction;

          if (Math.abs(cx) < 0.05 && Math.abs(cy) < 0.05) {
            cursorInertiaFrame.current = 0;
            return;
          }

          const dx = cx * dt;
          const dy = cy * dt;
          send({ t: "m", dx, dy });
          cursorInertiaFrame.current = requestAnimationFrame(step);
        };
        cursorInertiaFrame.current = requestAnimationFrame(step);
      }

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
  };

  // Mouse drag via Pointer Lock: cursor stays locked inside the trackpad div
  // This handles the actual mouse button events when the pointer is already locked.
  const onPointerDown = (e: React.PointerEvent) => {
    if (e.pointerType === "touch") return;

    const mod = getModifiers(e);

    // Middle click
    if (e.button === 1) {
      e.preventDefault();
      send({ t: "c", btn: "middle", mod });
      return;
    }

    // Right click handled by onContextMenu
    if (e.button === 2) {
      e.preventDefault();
      send({ t: "c", btn: "right", mod });
      return;
    }

    // Left click / drag
    if (document.pointerLockElement) {
      if (settings.clickAndDrag) {
        if (dragLocked.current) {
          send({ t: "u", mod });
          dragLocked.current = false;
        } else {
          send({ t: "d", mod });
          dragLocked.current = true;
        }
      } else {
        mouseHeld.current = true;
        send({ t: "d", mod });
      }
    } else {
      mouseHeld.current = true;
      send({ t: "d", mod });
    }
  };

  // Request pointer lock on click. This must be done in a click handler (mouse up)
  // rather than down to satisfy browser user activation requirements reliably.
  const onClick = () => {
    if (!document.pointerLockElement) {
      trackpadRef.current?.requestPointerLock();
      trackpadRef.current?.focus();
    }
  };

  // Pointer lock mouse movement and release
  useEffect(() => {
    if (!connected) return;

    const handleMouseMove = (e: MouseEvent) => {
      if (!document.pointerLockElement) return;
      const now = performance.now();
      if (now - lastMoveTime.current < 16) return;
      lastMoveTime.current = now;
      send({ t: "m", dx: e.movementX * settings.cursorSensitivity, dy: e.movementY * settings.cursorSensitivity });
    };

    const handleMouseUp = (e: MouseEvent) => {
      if (mouseHeld.current) {
        mouseHeld.current = false;
        send({ t: "u", mod: getModifiers(e) });
      }
    };

    const handlePointerLockChange = () => {
      if (!document.pointerLockElement) {
        if (mouseHeld.current) {
          mouseHeld.current = false;
          send({ t: "u" });
        }
        if (dragLocked.current) {
          dragLocked.current = false;
          send({ t: "u" });
        }
        trackpadRef.current?.blur();
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

    el.addEventListener("keydown", handleKey);
    return () => el.removeEventListener("keydown", handleKey);
  }, [connected, handleKey]);

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
        const dy = Math.round(e.deltaY * settings.scrollSensitivity);
        if (dx !== 0 || dy !== 0) {
          send({ t: "sc", dx, dy });
        }
      } else if (e.deltaMode === 2) {
        // Page-based: treat as lines with a multiplier
        const dx = Math.round(e.deltaX * 10);
        const dy = Math.round(e.deltaY * 10 * settings.scrollSensitivity);
        if (dx !== 0 || dy !== 0) {
          send({ t: "sc", dx, dy });
        }
      } else {
        // Pixel-based (trackpads, smooth scrolling)
        const dx = Math.round(e.deltaX);
        const dy = Math.round(e.deltaY);
        if (Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5) {
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
      onClick={connected ? onClick : undefined}
      onContextMenu={(e) => {
        e.preventDefault();
      }}
    >
      <input
        ref={inputRef}
        type="text"
        className="opacity-0 fixed top-0 left-0 h-0 w-0 pointer-events-none"
        autoComplete="off"
        onKeyDown={handleKey}
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
  const [showSettings, setShowSettings] = useState(false);
  const [settings, setSettings] = useState<TrackpadSettings>({
    cursorSensitivity: 1.5,
    cursorFriction: 0.92,
    scrollSensitivity: 1.5,
    scrollFriction: 0.92,
    clickAndDrag: false,
  });
  const settingsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (showSettings) {
      const onClick = (e: MouseEvent) => {
        if (settingsRef.current && !settingsRef.current.contains(e.target as Node)) {
          setShowSettings(false);
        }
      };
      document.addEventListener("mousedown", onClick);
      return () => document.removeEventListener("mousedown", onClick);
    }
  }, [showSettings]);

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

  // Check for local network connection and redirect if possible
  useEffect(() => {
    if (!authed) return;

    const checkLocalConnection = async () => {
      try {
        const status = await fetchJSON<StatusResponse>("/api/status");
        if (status.local_ip && status.secret && status.port) {
          // If we are already on the local IP, do nothing
          if (window.location.hostname === status.local_ip) return;

          // Try to contact the local IP
          const protocol = window.location.protocol;
          const localUrl = `${protocol}//${status.local_ip}:${status.port}`;

          const controller = new AbortController();
          const timeoutId = setTimeout(() => controller.abort(), 1000);
          const localStatus = await fetch(`${localUrl}/api/status`, { signal: controller.signal }).then(r => r.json());
          clearTimeout(timeoutId);

          if (localStatus.secret === status.secret) {
            window.location.href = localUrl;
          }
        }
      } catch (e) {
        // Ignore errors (e.g. not on same network)
      }
    };

    checkLocalConnection();
  }, [authed]);

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          Trackpad
          <span
            className={`inline-block w-2 h-2 rounded-full ${connected ? "bg-green-400" : "bg-red-400"
              }`}
          />
        </h2>
        <div className="relative">
          <button
            onClick={() => setShowSettings(!showSettings)}
            className="p-2 text-zinc-400 hover:text-zinc-100 transition-colors rounded-full hover:bg-zinc-800 cursor-pointer"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.1a2 2 0 0 1-1-1.72v-.51a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"></path>
              <circle cx="12" cy="12" r="3"></circle>
            </svg>
          </button>
          {showSettings && (
            <div ref={settingsRef} className="absolute right-0 top-full mt-2 w-64 bg-zinc-900 border border-zinc-700 rounded-lg shadow-xl p-4 z-10 flex flex-col gap-4">
              <div>
                <label className="block text-xs text-zinc-400 mb-1">Cursor Sensitivity ({settings.cursorSensitivity.toFixed(1)})</label>
                <input
                  type="range" min="0.1" max="5" step="0.1"
                  value={settings.cursorSensitivity}
                  onChange={(e) => setSettings({ ...settings, cursorSensitivity: parseFloat(e.target.value) })}
                  className="w-full accent-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs text-zinc-400 mb-1">Cursor Friction ({settings.cursorFriction.toFixed(2)})</label>
                <input
                  type="range" min="0.8" max="0.99" step="0.01"
                  value={settings.cursorFriction}
                  onChange={(e) => setSettings({ ...settings, cursorFriction: parseFloat(e.target.value) })}
                  className="w-full accent-blue-500"
                />
              </div>
              <div className="h-px bg-zinc-800" />
              <div>
                <label className="block text-xs text-zinc-400 mb-1">Scroll Sensitivity ({settings.scrollSensitivity.toFixed(1)})</label>
                <input
                  type="range" min="0.1" max="5" step="0.1"
                  value={settings.scrollSensitivity}
                  onChange={(e) => setSettings({ ...settings, scrollSensitivity: parseFloat(e.target.value) })}
                  className="w-full accent-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs text-zinc-400 mb-1">Scroll Friction ({settings.scrollFriction.toFixed(2)})</label>
                <input
                  type="range" min="0.8" max="0.99" step="0.01"
                  value={settings.scrollFriction}
                  onChange={(e) => setSettings({ ...settings, scrollFriction: parseFloat(e.target.value) })}
                  className="w-full accent-blue-500"
                />
              </div>
              <div className="h-px bg-zinc-800" />
              <div className="flex items-center justify-between">
                <label className="text-xs text-zinc-400">Click and Drag</label>
                <input
                  type="checkbox"
                  checked={settings.clickAndDrag}
                  onChange={(e) => setSettings({ ...settings, clickAndDrag: e.target.checked })}
                />
              </div>
            </div>
          )}
        </div>
      </div>
      <div className="flex flex-col-reverse md:flex-row gap-6 sm:items-start">
        <TouchArea
          connected={connected}
          send={send}
          silent={refreshSignal.silent}
          settings={settings}
          />
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
