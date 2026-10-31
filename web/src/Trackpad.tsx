import { useState, useEffect, useCallback, useRef } from "react";
import {
  ChevronDown,
  ChevronUp,
  Keyboard,
  Settings,
  X,
} from "lucide-react";
import { MonitorDisplay } from "./MonitorDisplay";
import { useStore } from "./store";
import { Modifier, MouseButton, type TrackpadMessage } from "./api";
import { characterCount, diffText } from "./trackpadText";
import {
  CustomKeyboard,
  type KeyboardSettings,
} from "./CustomKeyboard";

function getModifiers(e: {
  shiftKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  metaKey: boolean;
}) {
  const mod: Modifier[] = [];
  if (e.shiftKey) mod.push(Modifier.SHIFT);
  if (e.ctrlKey) mod.push(Modifier.CTRL);
  if (e.altKey) mod.push(Modifier.ALT);
  if (e.metaKey) mod.push(Modifier.META);
  return mod.length > 0 ? mod : [];
}

function combineModifiers(...groups: Modifier[][]): Modifier[] {
  return [...new Set(groups.flat())];
}

interface TrackpadSettings extends KeyboardSettings {
  cursorSensitivity: number;
  cursorFriction: number;
  scrollSensitivity: number;
  scrollFriction: number;
  clickAndDrag: boolean;
  customKeyboard: boolean;
}

const DEFAULT_SETTINGS: TrackpadSettings = {
  cursorSensitivity: 1.5,
  cursorFriction: 0.92,
  scrollSensitivity: 1.5,
  scrollFriction: 0.92,
  clickAndDrag: false,
  customKeyboard: true,
  numberRow: true,
  symbolHints: true,
  doubleSpacePeriod: true,
  haptics: true,
  longPressDelayMs: 430,
  spaceHoldDelayMs: 320,
  backspaceRepeatDelayMs: 430,
  backspaceRepeatIntervalMs: 65,
  shiftDoubleTapMs: 350,
  previewFadeMs: 4500,
};

function loadSettings(): TrackpadSettings {
  try {
    const saved = localStorage.getItem("ottoman-trackpad-settings");
    const parsed = saved ? JSON.parse(saved) as Partial<TrackpadSettings> : null;
    return parsed ? { ...DEFAULT_SETTINGS, ...parsed } : DEFAULT_SETTINGS;
  } catch {
    return DEFAULT_SETTINGS;
  }
}

function TouchArea({
  connected,
  connecting,
  send,
  settings,
  onSettingsChange,
}: {
  connected: boolean;
  connecting: boolean;
  send: (msg: TrackpadMessage) => void;
  settings: TrackpadSettings;
  onSettingsChange: (patch: Partial<TrackpadSettings>) => void;
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
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const [focused, setFocused] = useState(false);
  const [keyboardOpen, setKeyboardOpen] = useState(false);
  const [keyboardSettingsOpen, setKeyboardSettingsOpen] = useState(false);
  const [activeModifiers, setActiveModifiers] = useState<Modifier[]>([]);
  const [draft, setDraft] = useState("");
  const [draftCursor, setDraftCursor] = useState(0);
  const [draftVisible, setDraftVisible] = useState(false);
  const draftRef = useRef("");
  const draftFadeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const remoteCursor = useRef(0);
  // Fallback mouse position tracking when pointer lock is unavailable (e.g. after Escape cooldown)
  const lastMousePos = useRef<{ x: number; y: number } | null>(null);

  // Two-finger scroll refs
  const twoFingerRef = useRef(false);
  const twoFingerLastX = useRef(0);
  const twoFingerLastY = useRef(0);
  const twoFingerStartTime = useRef(0);
  const twoFingerMoved = useRef(false);

  // Where the previous single-finger gesture ended, and whether that gesture was
  // one that a following quick touch should turn into a click-and-drag (a genuine
  // tap, or an active drag being continued). A plain cursor move is neither, so
  // moving then quickly moving again keeps moving instead of starting a drag.
  const lastTapEndPos = useRef<{ x: number; y: number } | null>(null);
  const lastGestureAllowsDrag = useRef(false);
  const scrollVelocity = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const inertiaFrame = useRef(0);
  const lastScrollTime = useRef(0);

  // Cursor inertia refs
  const cursorVelocity = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const cursorInertiaFrame = useRef(0);
  const lastCursorTime = useRef(0);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent | KeyboardEvent) => {
      // Skip unidentified keys - handled by input event on mobile
      if (e.key === "Unidentified" || e.key === "Process") return;

      e.preventDefault();
      e.stopPropagation();

      send({
        type: "keydown",
        key: e.key,
        modifiers: combineModifiers(getModifiers(e), activeModifiers),
      });
    },
    [activeModifiers, send],
  );

  // Release keys pressed via handleKeyDown (physical keyboard). Without a keyup
  // the key stays held on the host's virtual device and wedges input.
  const handleKeyUp = useCallback(
    (e: React.KeyboardEvent | KeyboardEvent) => {
      if (e.key === "Unidentified" || e.key === "Process") return;

      e.preventDefault();
      e.stopPropagation();

      send({
        type: "keyup",
        key: e.key,
        modifiers: combineModifiers(getModifiers(e), activeModifiers),
      });
    },
    [activeModifiers, send],
  );

  const sendKeyPress = useCallback(
    (key: string, modifiers: Modifier[] = []) => {
      send({ type: "keydown", key, modifiers });
      send({ type: "keyup", key, modifiers });
    },
    [send],
  );

  const showDraftIndicator = useCallback(() => {
    setDraftVisible(true);
    if (draftFadeTimer.current) clearTimeout(draftFadeTimer.current);
    draftFadeTimer.current = setTimeout(
      () => setDraftVisible(false),
      settings.previewFadeMs,
    );
  }, [settings.previewFadeMs]);

  useEffect(() => () => {
    if (draftFadeTimer.current) clearTimeout(draftFadeTimer.current);
  }, []);

  const toggleModifier = useCallback((modifier: Modifier) => {
    setActiveModifiers((current) =>
      current.includes(modifier)
        ? current.filter((item) => item !== modifier)
        : [...current, modifier],
    );
  }, []);

  const moveRemoteCursor = useCallback(
    (target: number) => {
      const delta = target - remoteCursor.current;
      const key = delta < 0 ? "ArrowLeft" : "ArrowRight";
      for (let i = 0; i < Math.abs(delta); i++) sendKeyPress(key);
      remoteCursor.current = target;
    },
    [sendKeyPress],
  );

  const handleInputKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (
        e.key === "Unidentified" ||
        e.key === "Process" ||
        e.key.length === 1 ||
        e.key === "Backspace"
      ) return;

      e.preventDefault();
      e.stopPropagation();
      sendKeyPress(e.key);
    },
    [sendKeyPress],
  );

  const handleTextChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const next = e.currentTarget.value;
      const edit = diffText(draftRef.current, next);
      if (edit) {
        const editStart = characterCount(draftRef.current.slice(0, edit.start));
        const removedCount = characterCount(edit.removed);
        moveRemoteCursor(editStart + removedCount);
        for (let i = 0; i < removedCount; i++) sendKeyPress("Backspace");
        remoteCursor.current = editStart;
        if (edit.inserted) {
          send({ type: "text", text: edit.inserted });
          remoteCursor.current += characterCount(edit.inserted);
        }
      }

      draftRef.current = next;
      setDraft(next);
      setDraftCursor(e.currentTarget.selectionEnd);
      showDraftIndicator();
      moveRemoteCursor(characterCount(next.slice(0, e.currentTarget.selectionEnd)));
    },
    [moveRemoteCursor, send, sendKeyPress, showDraftIndicator],
  );

  const handleSelection = useCallback(
    (e: React.SyntheticEvent<HTMLTextAreaElement>) => {
      const input = e.currentTarget;
      if (input.selectionStart === input.selectionEnd) {
        setDraftCursor(input.selectionEnd);
        showDraftIndicator();
        moveRemoteCursor(characterCount(input.value.slice(0, input.selectionEnd)));
      }
    },
    [moveRemoteCursor, showDraftIndicator],
  );

  const appendText = useCallback(
    (value: string, shifted = false) => {
      const before = draftRef.current;
      const commandModifiers = activeModifiers.filter(
        (modifier) => modifier !== Modifier.SHIFT,
      );
      if (commandModifiers.length > 0 && characterCount(value) === 1) {
        const shortcutModifiers = combineModifiers(
          activeModifiers,
          shifted ? [Modifier.SHIFT] : [],
        );
        sendKeyPress(value.toLocaleLowerCase(), shortcutModifiers);
        return;
      }
      if (
        settings.doubleSpacePeriod &&
        value === " " &&
        /\S $/.test(before)
      ) {
        sendKeyPress("Backspace");
        send({ type: "text", text: ". " });
        const next = `${before.slice(0, -1)}. `;
        draftRef.current = next;
        remoteCursor.current = Math.max(0, remoteCursor.current - 1) + 2;
        setDraft(next);
        setDraftCursor(next.length);
        showDraftIndicator();
        return;
      }

      send({ type: "text", text: value });
      const next = before + value;
      draftRef.current = next;
      remoteCursor.current += characterCount(value);
      setDraft(next);
      setDraftCursor(next.length);
      showDraftIndicator();
    },
    [activeModifiers, send, sendKeyPress, settings.doubleSpacePeriod, showDraftIndicator],
  );

  const backspace = useCallback(() => {
    sendKeyPress("Backspace", activeModifiers);
    if (activeModifiers.some((modifier) => modifier !== Modifier.SHIFT)) return;
    const characters = Array.from(draftRef.current);
    characters.pop();
    const next = characters.join("");
    draftRef.current = next;
    remoteCursor.current = Math.max(0, remoteCursor.current - 1);
    setDraft(next);
    setDraftCursor(next.length);
    showDraftIndicator();
  }, [activeModifiers, sendKeyPress, showDraftIndicator]);

  const onTouchStart = (e: React.TouchEvent) => {
    e.preventDefault();
    setDraftVisible(false);

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
      twoFingerStartTime.current = performance.now();
      twoFingerMoved.current = false;
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
      twoFingerLastX.current = midX;
      twoFingerLastY.current = midY;
      scrollVelocity.current = { x: 0, y: 0 };
      lastScrollTime.current = performance.now();
      // Do not let the remaining finger inherit stale single-finger movement
      // state when one finger is lifted at the end of a scroll.
      lastTouchRef.current = null;
      touchStartPos.current = null;
      return;
    }

    twoFingerRef.current = false;
    const touch = e.touches[0];
    touchStartTime.current = performance.now();
    cursorVelocity.current = { x: 0, y: 0 };
    lastCursorTime.current = performance.now();

    // Check for double-tap-drag start. Only start a drag when the previous
    // gesture was a genuine tap (or an active drag we're continuing) AND this
    // touch lands close to where that gesture ended. This prevents a cursor move
    // followed by another quick move from being misread as a click-and-drag —
    // that sequence should just keep moving the cursor.
    const dist = lastTapEndPos.current
      ? Math.hypot(
          touch.clientX - lastTapEndPos.current.x,
          touch.clientY - lastTapEndPos.current.y,
        )
      : Infinity;

    if (
      lastGestureAllowsDrag.current &&
      touchStartTime.current - lastTouchEndTime.current < 300 &&
      dist < 40
    ) {
      isDragging.current = true;
      send({ type: "mousedown", btn: MouseButton.LEFT, modifiers: activeModifiers });
    }

    touchStartPos.current = { x: touch.clientX, y: touch.clientY };
    lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
  };

  const onTouchMove = (e: React.TouchEvent) => {
    e.preventDefault();
    const now = performance.now();
    if (now - lastMoveTime.current < 16) return;
    lastMoveTime.current = now;

    // Two-finger scroll
    if (twoFingerRef.current) {
      // A two-finger gesture remains a scroll/right-click until every finger is
      // lifted. In particular, never reinterpret its last finger as cursor move.
      if (e.touches.length !== 2) return;
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
        twoFingerMoved.current = true;
        send({
          type: "mousescroll",
          dx: Math.round(-dx),
          dy: Math.round(-dy),
          precise: true,
        });
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
        cursorVelocity.current = {
          x: cursorVelocity.current.x * alpha + vx * (1 - alpha),
          y: cursorVelocity.current.y * alpha + vy * (1 - alpha),
        };
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

        if (
          !twoFingerMoved.current &&
          performance.now() - twoFingerStartTime.current < 300
        ) {
          send({ type: "mouseclick", btn: MouseButton.RIGHT, modifiers: activeModifiers });
          // Only trigger inertia if the user was scrolling recently and with sufficient velocity
        } else if (
          timeSinceLastScroll < 50 &&
          (Math.abs(vx) > 0.1 || Math.abs(vy) > 0.1)
        ) {
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
            send({
              type: "mousescroll",
              dx: Math.round(-dx),
              dy: Math.round(-dy),
              precise: true,
            });
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
      send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: activeModifiers });
      isDragging.current = false;
      // A drag can be continued: lifting and quickly re-touching resumes it.
      lastGestureAllowsDrag.current = true;
    } else {
      // Cursor inertia
      const { x: vx, y: vy } = cursorVelocity.current;
      const timeSinceLastMove = performance.now() - lastCursorTime.current;

      if (
        timeSinceLastMove < 50 &&
        (Math.abs(vx) > 0.1 || Math.abs(vy) > 0.1)
      ) {
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

      // A genuine tap (short and near-stationary) is a click, and it also arms
      // the next quick touch to become a click-and-drag. A longer or moved
      // gesture was a cursor move, which must not arm a drag.
      let wasTap = false;
      if (touchStartPos.current && now - touchStartTime.current < 200) {
        const touch = e.changedTouches[0];
        const dx = touch.clientX - touchStartPos.current.x;
        const dy = touch.clientY - touchStartPos.current.y;
        if (Math.sqrt(dx * dx + dy * dy) < 10) {
          send({ type: "mouseclick", btn: MouseButton.LEFT, modifiers: activeModifiers });
          wasTap = true;
        }
      }
      lastGestureAllowsDrag.current = wasTap;
    }

    // Remember where this gesture lifted so the next touch can measure closeness.
    const endTouch = e.changedTouches[0];
    if (endTouch)
      lastTapEndPos.current = { x: endTouch.clientX, y: endTouch.clientY };

    touchStartPos.current = null;
    lastTouchRef.current = null;
  };

  // Mouse drag via Pointer Lock (or fallback position tracking after Escape cooldown)
  const onPointerDown = (e: React.PointerEvent) => {
    if (e.pointerType === "touch") return;
    setDraftVisible(false);

    const modifiers = combineModifiers(getModifiers(e), activeModifiers);

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
        send({
          type: "mousemoverel",
          dx: e.movementX * settings.cursorSensitivity,
          dy: e.movementY * settings.cursorSensitivity,
        });
      } else if (mouseHeld.current && lastMousePos.current) {
        // Fallback: track delta manually (after Escape cooldown denies pointer lock)
        const dx =
          (e.clientX - lastMousePos.current.x) * settings.cursorSensitivity;
        const dy =
          (e.clientY - lastMousePos.current.y) * settings.cursorSensitivity;
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
        send({
          type: "mouseup",
          btn: MouseButton.LEFT,
          modifiers: combineModifiers(getModifiers(e), activeModifiers),
        }); // TODO: Handle non-LEFT
      }
    };

    const handlePointerLockChange = () => {
      if (!document.pointerLockElement) {
        if (mouseHeld.current) {
          mouseHeld.current = false;
          send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: activeModifiers });
        }
        if (dragLocked.current) {
          dragLocked.current = false;
          send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: activeModifiers });
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
      document.removeEventListener(
        "pointerlockchange",
        handlePointerLockChange,
      );
      if (document.pointerLockElement) document.exitPointerLock();
    };
  }, [activeModifiers, connected, send, settings.cursorSensitivity]);

  // Keyboard capture on the trackpad div
  useEffect(() => {
    const el = trackpadRef.current;
    if (!el || !connected) return;

    el.addEventListener("keydown", handleKeyDown);
    el.addEventListener("keyup", handleKeyUp);
    return () => {
      el.removeEventListener("keydown", handleKeyDown);
      el.removeEventListener("keyup", handleKeyUp);
    };
  }, [connected, handleKeyDown, handleKeyUp]);

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
  }, [connected, send, settings.scrollSensitivity]);

  const indicatorStart = Math.max(0, draftCursor - 160);
  const indicatorEnd = Math.min(draft.length, indicatorStart + 260);
  const indicatorBefore = draft.slice(indicatorStart, draftCursor);
  const indicatorAfter = draft.slice(draftCursor, indicatorEnd);

  return (
    <div className="w-full md:max-w-sm md:shrink-0 flex flex-col gap-2">
      <div className="relative">
        <div
          ref={trackpadRef}
          tabIndex={0}
          aria-label="Trackpad. Tap to click, move one finger to point, tap two fingers to right click, or move two fingers to scroll."
          className={`relative w-full aspect-square max-h-[60vh] overflow-hidden rounded-xl border-2 transition-colors select-none touch-none outline-none ${
            connected
              ? focused || keyboardOpen
                ? "border-blue-500 ring-2 ring-blue-500/30 bg-zinc-900/80 cursor-crosshair"
                : "border-zinc-700 bg-zinc-900/80 cursor-crosshair"
              : "border-red-500/50 bg-zinc-900/40 pointer-events-none opacity-50"
          }`}
          style={
            connected
              ? {
                  backgroundImage:
                    "radial-gradient(circle, rgba(63,63,70,0.3) 1px, transparent 1px)",
                  backgroundSize: "20px 20px",
                }
              : undefined
          }
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
          {connected && draft && (
            <div
              aria-live="polite"
              aria-atomic="true"
              className={`pointer-events-none absolute inset-x-3 bottom-14 max-h-24 select-none overflow-hidden whitespace-pre-wrap break-words rounded-lg border border-white/5 bg-zinc-950/55 px-3 py-2 text-sm leading-5 text-zinc-200 shadow-lg backdrop-blur-md transition-all duration-300 ${
                draftVisible
                  ? "translate-y-0 opacity-100"
                  : "translate-y-1 opacity-0"
              }`}
            >
              {indicatorStart > 0 && <span className="text-zinc-500">…</span>}
              {indicatorBefore}
              <span className="mx-px inline-block h-[1.15em] w-px translate-y-0.5 animate-pulse bg-blue-300" />
              {indicatorAfter}
              {indicatorEnd < draft.length && <span className="text-zinc-500">…</span>}
            </div>
          )}
        </div>
        {connected && (
          <ScrollRail send={send} sensitivity={settings.scrollSensitivity} />
        )}
        <button
          type="button"
          disabled={!connected}
          aria-label={settings.customKeyboard
            ? "Open Ottoman keyboard to type on the remote computer"
            : "Open device keyboard to type on the remote computer"}
          title={settings.customKeyboard ? "Open Ottoman keyboard" : "Open device keyboard"}
          onClick={() => {
            if (settings.customKeyboard) {
              setKeyboardSettingsOpen(false);
              setKeyboardOpen(true);
            }
            else inputRef.current?.focus();
          }}
          className="absolute bottom-3 left-3 z-20 flex h-10 w-10 items-center justify-center rounded-full border border-white/10 bg-zinc-950/75 text-zinc-300 shadow-lg backdrop-blur-md transition-colors hover:bg-zinc-800 hover:text-white disabled:opacity-40"
        >
          <Keyboard className="h-[18px] w-[18px]" aria-hidden="true" />
        </button>
        {!settings.customKeyboard && (
          <button
            type="button"
            disabled={!connected}
            aria-label="Open keyboard settings"
            title="Keyboard settings"
            onClick={() => {
              setKeyboardSettingsOpen(true);
              setKeyboardOpen(true);
            }}
            className="absolute bottom-4 left-16 z-20 flex h-8 w-8 items-center justify-center rounded-full border border-white/10 bg-zinc-950/65 text-zinc-400 shadow-lg backdrop-blur-md transition-colors hover:bg-zinc-800 hover:text-white disabled:opacity-40"
          >
            <Settings className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        )}
        {!settings.customKeyboard && (
          <textarea
            ref={inputRef}
            rows={1}
            value={draft}
            disabled={!connected}
            aria-label="Type on the remote computer with the device keyboard"
            className="pointer-events-none absolute bottom-2 left-2 h-px w-px resize-none opacity-0"
            autoComplete="on"
            autoCapitalize="sentences"
            autoCorrect="on"
            spellCheck={true}
            enterKeyHint="enter"
            inputMode="text"
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            onKeyDown={handleInputKeyDown}
            onChange={handleTextChange}
            onSelect={handleSelection}
          />
        )}
        <CustomKeyboard
          key={keyboardSettingsOpen ? "settings" : "keyboard"}
          open={keyboardOpen}
          connected={connected}
          settings={settings}
          onText={appendText}
          onBackspace={backspace}
          onEnter={(shifted) => {
            const modifiers = combineModifiers(
              activeModifiers,
              shifted ? [Modifier.SHIFT] : [],
            );
            sendKeyPress("Enter", modifiers);
            if (activeModifiers.length > 0) return;
            const next = `${draftRef.current}\n`;
            draftRef.current = next;
            remoteCursor.current += 1;
            setDraft(next);
            setDraftCursor(next.length);
            showDraftIndicator();
          }}
          onTab={() => sendKeyPress("Tab", activeModifiers)}
          onNavigate={(key) => sendKeyPress(key, activeModifiers)}
          onClose={() => {
            setKeyboardOpen(false);
            setKeyboardSettingsOpen(false);
          }}
          onSettingsChange={onSettingsChange}
          onUseCustomKeyboard={() => onSettingsChange({ customKeyboard: true })}
          onUseDeviceKeyboard={() => {
            onSettingsChange({ customKeyboard: false });
            setKeyboardOpen(false);
            setKeyboardSettingsOpen(false);
            requestAnimationFrame(() => inputRef.current?.focus());
          }}
          usingCustomKeyboard={settings.customKeyboard}
          settingsPageOpen={keyboardSettingsOpen}
          modifiers={activeModifiers}
          onToggleModifier={toggleModifier}
        />
      </div>
      {keyboardOpen && (
        <div className="h-[360px] md:hidden" aria-hidden="true" />
      )}
    </div>
  );
}

function ScrollRail({
  send,
  sensitivity,
}: {
  send: (msg: TrackpadMessage) => void;
  sensitivity: number;
}) {
  const lastY = useRef<number | null>(null);

  const start = (e: React.TouchEvent) => {
    e.preventDefault();
    e.stopPropagation();
    lastY.current = e.touches[0]?.clientY ?? null;
  };
  const move = (e: React.TouchEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const y = e.touches[0]?.clientY;
    if (y == null || lastY.current == null) return;
    const dy = (y - lastY.current) * sensitivity;
    lastY.current = y;
    if (Math.abs(dy) >= 0.5)
      send({ type: "mousescroll", dx: 0, dy: Math.round(-dy), precise: true });
  };

  return (
    <div
      className="absolute inset-y-3 right-2 flex w-10 touch-none select-none flex-col items-center justify-between rounded-full border border-zinc-600/80 bg-zinc-950/80 py-3 text-zinc-400 shadow-lg backdrop-blur"
      aria-label="One-finger scroll control"
      onTouchStart={start}
      onTouchMove={move}
      onTouchEnd={() => {
        lastY.current = null;
      }}
    >
      <ChevronUp className="h-4 w-4" />
      <div className="h-12 w-1 rounded-full bg-zinc-600" />
      <ChevronDown className="h-4 w-4" />
    </div>
  );
}

export function Trackpad({
  connected,
  connecting,
  cursorPos,
  cursorSupported,
  send,
}: {
  connected: boolean;
  connecting: boolean;
  cursorPos: { x: number; y: number } | null;
  cursorSupported: boolean;
  send: (msg: TrackpadMessage) => void;
}) {
  const layouts = useStore((s) => s.layouts);
  const currentLayout = useStore((s) => s.currentLayout);
  const loading = useStore((s) => s.layoutsLoading);
  const [showSettings, setShowSettings] = useState(false);
  const [settings, setSettings] = useState<TrackpadSettings>(loadSettings);
  const settingsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    localStorage.setItem("ottoman-trackpad-settings", JSON.stringify(settings));
  }, [settings]);

  useEffect(() => {
    if (showSettings) {
      const onClick = (e: MouseEvent) => {
        if (
          settingsRef.current &&
          !settingsRef.current.contains(e.target as Node)
        ) {
          setShowSettings(false);
        }
      };
      document.addEventListener("mousedown", onClick);
      return () => document.removeEventListener("mousedown", onClick);
    }
  }, [showSettings]);

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          {"Trackpad"}
          <span
            className={`inline-block w-2 h-2 rounded-full ${
              connected ? "bg-green-400" : "bg-red-400"
            }`}
          />
        </h2>
        <div className="relative" ref={settingsRef}>
          <button
            onClick={() => setShowSettings(!showSettings)}
            title={showSettings ? "Close settings" : "Trackpad settings"}
            aria-label={showSettings ? "Close settings" : "Trackpad settings"}
            className={`w-8 h-8 flex items-center justify-center rounded-md transition-colors border cursor-pointer ${
              showSettings
                ? "bg-zinc-700 border-zinc-600 text-zinc-100"
                : "bg-zinc-800 hover:bg-zinc-700 border-zinc-700 text-zinc-300"
            }`}
          >
            {showSettings ? (
              <X className="w-4 h-4" />
            ) : (
              <Settings className="w-4 h-4" />
            )}
          </button>
          {showSettings && (
            <div className="absolute right-0 top-full z-[60] mt-2 flex max-h-[70vh] w-72 flex-col gap-4 overflow-y-auto rounded-lg border border-zinc-700 bg-zinc-900 p-4 shadow-xl">
              <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
                Trackpad
              </div>
              <div>
                <label className="block text-xs text-zinc-400 mb-1">
                  Cursor Sensitivity ({settings.cursorSensitivity.toFixed(1)})
                </label>
                <input
                  type="range"
                  min="0.1"
                  max="5"
                  step="0.1"
                  value={settings.cursorSensitivity}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      cursorSensitivity: parseFloat(e.target.value),
                    })
                  }
                  className="w-full accent-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs text-zinc-400 mb-1">
                  Cursor Friction ({settings.cursorFriction.toFixed(2)})
                </label>
                <input
                  type="range"
                  min="0.8"
                  max="0.99"
                  step="0.01"
                  value={settings.cursorFriction}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      cursorFriction: parseFloat(e.target.value),
                    })
                  }
                  className="w-full accent-blue-500"
                />
              </div>
              <div className="h-px bg-zinc-800" />
              <div>
                <label className="block text-xs text-zinc-400 mb-1">
                  Scroll Sensitivity ({settings.scrollSensitivity.toFixed(1)})
                </label>
                <input
                  type="range"
                  min="0.1"
                  max="5"
                  step="0.1"
                  value={settings.scrollSensitivity}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      scrollSensitivity: parseFloat(e.target.value),
                    })
                  }
                  className="w-full accent-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs text-zinc-400 mb-1">
                  Scroll Friction ({settings.scrollFriction.toFixed(2)})
                </label>
                <input
                  type="range"
                  min="0.8"
                  max="0.99"
                  step="0.01"
                  value={settings.scrollFriction}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      scrollFriction: parseFloat(e.target.value),
                    })
                  }
                  className="w-full accent-blue-500"
                />
              </div>
              <div className="h-px bg-zinc-800" />
              <div className="flex items-center justify-between">
                <label className="text-xs text-zinc-400">Click and Drag</label>
                <input
                  type="checkbox"
                  checked={settings.clickAndDrag}
                  onChange={(e) =>
                    setSettings({ ...settings, clickAndDrag: e.target.checked })
                  }
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
          onSettingsChange={(patch) => setSettings((current) => ({ ...current, ...patch }))}
        />
        <MonitorDisplay
          layouts={layouts}
          currentLayout={currentLayout}
          cursorPos={cursorPos}
          connected={connected}
          loading={loading}
          onSetPosition={
            cursorSupported
              ? (x, y) => send({ type: "mousemoveto", x, y })
              : undefined
          }
        />
      </div>
    </section>
  );
}
