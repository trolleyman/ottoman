import { useState, useEffect, useCallback, useRef } from "react";
import { MonitorDisplay } from "./MonitorDisplay";
import { useStore } from "./store";
import { Modifier, MouseButton, type TrackpadMessage } from "./api";

function getModifiers(e: { shiftKey: boolean; ctrlKey: boolean; altKey: boolean; metaKey: boolean }) {
  const mod: Modifier[] = [];
  if (e.shiftKey) mod.push(Modifier.SHIFT);
  if (e.ctrlKey) mod.push(Modifier.CTRL);
  if (e.altKey) mod.push(Modifier.ALT);
  if (e.metaKey) mod.push(Modifier.META);
  return mod.length > 0 ? mod : [];
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
  connecting,
  send,
  settings,
}: {
  connected: boolean;
  connecting: boolean;
  send: (msg: TrackpadMessage) => void;
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
  // Fallback mouse position tracking when pointer lock is unavailable (e.g. after Escape cooldown)
  const lastMousePos = useRef<{ x: number; y: number } | null>(null);

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

  const handleKeyDown = useCallback((e: React.KeyboardEvent | KeyboardEvent) => {
    // Skip unidentified keys - handled by input event on mobile
    if (e.key === "Unidentified" || e.key === "Process") return;

    e.preventDefault();
    e.stopPropagation();

    send({ type: "keydown", key: e.key, modifiers: getModifiers(e) });
  }, [send]);

  // Fallback for characters that don't produce proper keydown events (mobile symbol keyboards)
  const handleInput = useCallback(() => {
    const input = inputRef.current;
    if (!input) return;
    const value = input.value;
    if (value) {
      send({ type: "text", text: value });
      input.value = "";
    }
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
      send({ type: "mousedown", btn: MouseButton.LEFT, modifiers: [] }); // TODO: Modifiers
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
        send({ type: "mousescroll", dx: Math.round(-dx), dy: Math.round(-dy), precise: true });
      }
      return;
    }

    const touch = e.touches[0];
    if (lastTouchRef.current) {
      const rawDx = touch.clientX - lastTouchRef.current.x;
      const rawDy = touch.clientY - lastTouchRef.current.y;
      const dx = rawDx * settings.cursorSensitivity;
      const dy = rawDy * settings.cursorSensitivity;

      send({ type: "mousemoverel", dx, dy });
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
            send({ type: "mousescroll", dx: Math.round(-dx), dy: Math.round(-dy), precise: true });
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
      send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: [] }); // TODO: Modifiers
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
          send({ type: "mousemoverel", dx, dy });
          cursorInertiaFrame.current = requestAnimationFrame(step);
        };
        cursorInertiaFrame.current = requestAnimationFrame(step);
      }

      if (touchStartPos.current && now - touchStartTime.current < 200) {
        const touch = e.changedTouches[0];
        const dx = touch.clientX - touchStartPos.current.x;
        const dy = touch.clientY - touchStartPos.current.y;
        if (Math.sqrt(dx * dx + dy * dy) < 10) {
          send({ type: "mouseclick", btn: MouseButton.LEFT, modifiers: [] }); // TODO: Modifiers
        }
      }
    }

    touchStartPos.current = null;
    lastTouchRef.current = null;
  };

  // Mouse drag via Pointer Lock (or fallback position tracking after Escape cooldown)
  const onPointerDown = (e: React.PointerEvent) => {
    if (e.pointerType === "touch") return;

    const modifiers = getModifiers(e);

    // Middle click
    if (e.button === 1) {
      e.preventDefault();
      send({ type: "mouseclick", btn: MouseButton.MIDDLE, modifiers });
      return;
    }

    // Right click handled by onContextMenu
    if (e.button === 2) {
      e.preventDefault();
      send({ type: "mouseclick", btn: MouseButton.RIGHT, modifiers }); // TODO: Implement MIDDLE/BACK/FORWARD
      return;
    }

    // Left button: grab pointer lock if not already locked
    if (!document.pointerLockElement) {
      // requestPointerLock may fail after Escape (browser cooldown ~1.5s).
      // In that case, fallback mouse tracking via lastMousePos kicks in.
      void trackpadRef.current?.requestPointerLock();
      lastMousePos.current = { x: e.clientX, y: e.clientY };
    }
    trackpadRef.current?.focus();

    // Handle click/drag
    if (document.pointerLockElement && settings.clickAndDrag) {
      if (dragLocked.current) {
        send({ type: "mouseup", btn: MouseButton.LEFT, modifiers }); // TODO: Left or right or other?
        dragLocked.current = false;
      } else {
        send({ type: "mousedown", btn: MouseButton.LEFT, modifiers });
        dragLocked.current = true;
      }
    } else {
      mouseHeld.current = true;
      send({ type: "mousedown", btn: MouseButton.LEFT, modifiers });
    }
  };

  // Mouse movement and release (pointer lock + fallback)
  useEffect(() => {
    if (!connected) return;

    const handleMouseMove = (e: MouseEvent) => {
      const now = performance.now();
      if (now - lastMoveTime.current < 16) return;
      lastMoveTime.current = now;

      if (document.pointerLockElement) {
        // Pointer lock active: use movementX/Y
        send({ type: "mousemoverel", dx: e.movementX * settings.cursorSensitivity, dy: e.movementY * settings.cursorSensitivity });
      } else if (mouseHeld.current && lastMousePos.current) {
        // Fallback: track delta manually (after Escape cooldown denies pointer lock)
        const dx = (e.clientX - lastMousePos.current.x) * settings.cursorSensitivity;
        const dy = (e.clientY - lastMousePos.current.y) * settings.cursorSensitivity;
        lastMousePos.current = { x: e.clientX, y: e.clientY };
        if (dx !== 0 || dy !== 0) {
          send({ type: "mousemoverel", dx, dy });
        }
      }
    };

    const handleMouseUp = (e: MouseEvent) => {
      if (mouseHeld.current) {
        mouseHeld.current = false;
        lastMousePos.current = null;
        send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: getModifiers(e) }); // TODO: Handle non-LEFT
      }
    };

    const handlePointerLockChange = () => {
      if (!document.pointerLockElement) {
        if (mouseHeld.current) {
          mouseHeld.current = false;
          send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: [] }); // TODO: Modifiers
        }
        if (dragLocked.current) {
          dragLocked.current = false;
          send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: [] }); // TODO: Modifiers
        }
        lastMousePos.current = null;
        // Don't blur - keep trackpad focused for keyboard events and easy re-lock
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

    el.addEventListener("keydown", handleKeyDown);
    return () => el.removeEventListener("keydown", handleKeyDown);
  }, [connected, handleKeyDown]);

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
          send({ type: "mousescroll", dx, dy });
        }
      } else if (e.deltaMode === 2) {
        // Page-based: treat as lines with a multiplier
        const dx = Math.round(e.deltaX * 10);
        const dy = Math.round(e.deltaY * 10 * settings.scrollSensitivity);
        if (dx !== 0 || dy !== 0) {
          send({ type: "mousescroll", dx, dy });
        }
      } else {
        // Pixel-based (trackpads, smooth scrolling)
        const dx = Math.round(e.deltaX);
        const dy = Math.round(e.deltaY);
        if (Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5) {
          send({ type: "mousescroll", dx, dy, precise: true });
        }
      }
    };

    el.addEventListener("wheel", handleWheel, { passive: false });
    return () => el.removeEventListener("wheel", handleWheel);
  }, [connected, send]);

  // When the mobile keyboard opens/closes, re-scroll the trackpad into view
  useEffect(() => {
    if (!focused) return;
    const vv = window.visualViewport;
    if (!vv) return;
    let lastHeight = vv.height;
    const onResize = () => {
      // Only scroll when height decreases significantly (keyboard opened)
      if (lastHeight - vv.height > 100) {
        trackpadRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
      }
      lastHeight = vv.height;
    };
    vv.addEventListener("resize", onResize);
    return () => vv.removeEventListener("resize", onResize);
  }, [focused]);

  return (
    <div
      ref={trackpadRef}
      tabIndex={0}
      className={`w-full aspect-square max-h-[60vh] md:max-w-sm md:shrink-0 rounded-xl border-2 transition-colors select-none touch-none outline-none ${connected
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
      }}
    >
      <input
        ref={inputRef}
        type="text"
        className="opacity-0 fixed top-0 left-0 h-0 w-0 pointer-events-none"
        autoComplete="off"
        onKeyDown={handleKeyDown}
        onInput={handleInput}
      />
      {!connected && (
        <div className="flex flex-col items-center justify-center h-full text-zinc-500 text-sm gap-2">
          {connecting ? (
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
  connected,
  connecting,
  cursorPos,
  send,
}: {
  connected: boolean;
  connecting: boolean;
  cursorPos: { x: number; y: number } | null;
  send: (msg: TrackpadMessage) => void;
}) {
  const layouts = useStore((s) => s.layouts);
  const currentLayout = useStore((s) => s.currentLayout);
  const loading = useStore((s) => s.layoutsLoading);
  const status = useStore((s) => s.status);

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

  // Check for local network connection and redirect if possible
  useEffect(() => {
    if (!status?.ip_address || !status?.port) return;
    if (window.location.hostname === status.ip_address) return;

    const checkLocalConnection = async () => {
      try {
        const protocol = window.location.protocol;
        const localUrl = `${protocol}//${status.ip_address}:${status.port}`;

        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), 1000);
        const resp = await fetch(`${localUrl}/health`, { signal: controller.signal });
        clearTimeout(timeoutId);

        if (resp.ok) {
          window.location.href = localUrl;
        }
      } catch {
        // Ignore errors (e.g. not on same network)
      }
    };

    void checkLocalConnection();
  }, [status?.ip_address, status?.port]);

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          {"Trackpad"}
          <span
            className={`inline-block w-2 h-2 rounded-full ${connected ? "bg-green-400" : "bg-red-400"
              }`}
          />
        </h2>
        <div className="relative" ref={settingsRef}>
          <button
            onClick={() => setShowSettings(!showSettings)}
            className="text-xs bg-zinc-800 hover:bg-zinc-700 text-zinc-300 px-3 py-1.5 rounded-md transition-colors border border-zinc-700 cursor-pointer"
          >
            {showSettings ? "Close" : "Settings"}
          </button>
          {showSettings && (
            <div className="absolute right-0 top-full mt-2 w-64 bg-zinc-900 border border-zinc-700 rounded-lg shadow-xl p-4 z-20 flex flex-col gap-4">
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
          connecting={connecting}
          send={send}
          settings={settings}
        />
        <MonitorDisplay
          layouts={layouts}
          currentLayout={currentLayout}
          cursorPos={cursorPos}
          connected={connected}
          loading={loading}
          onSetPosition={(x, y) => send({ type: "mousemoveto", x, y })}
        />
      </div>
    </section>
  );
}
