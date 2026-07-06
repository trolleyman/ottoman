import { useEffect, useRef, useState } from "react";

// useCoalescedSlider keeps a range input smooth against a slow, async backend
// whose reported value is also being overwritten by a background poll. It
// solves the two problems that make a naive `value={store} onChange={write}`
// slider janky:
//
//   1. Snap-back — a poll (or optimistic refetch) replaces the store value with
//      the backend's reported one, which lags a slow write, so a controlled
//      thumb jumps backward mid-drag. Here the drag value is owned locally and
//      is authoritative; incoming server values are ignored while dragging and
//      until the server echoes our last committed write.
//   2. Flooding — a range input fires onChange per integer step, and each write
//      may be a slow round-trip (e.g. ddcutil over DDC/CI). We coalesce to at
//      most one in-flight commit and always send the latest value, so the
//      network paces itself to the backend instead of queuing stale writes.
//
// Values are expected to round-trip exactly through the backend (use integers),
// so equality reliably detects when the server has caught up to our write. The
// caller owns display/units; `set` is wired to the input's onChange and `value`
// drives both the thumb and any label.
export function useCoalescedSlider(
  serverValue: number,
  commit: (value: number) => Promise<void> | void,
): {
  value: number;
  set: (value: number) => void;
  dragProps: {
    onPointerDown: () => void;
    onPointerUp: () => void;
    onPointerCancel: () => void;
  };
} {
  const [value, setValue] = useState(serverValue);
  const draggingRef = useRef(false);
  // The most recent value committed to the backend; while set, server values
  // that don't yet match it are stale echoes and must be ignored.
  const lastSentRef = useRef<number | null>(null);
  // Coalescing state: at most one write in flight, with the latest pending
  // value queued behind it.
  const pendingRef = useRef<number | null>(null);
  const inflightRef = useRef(false);
  // Always commit through the latest closure (captures fresh ids/handlers).
  const commitRef = useRef(commit);
  commitRef.current = commit;

  // Adopt the server value only when idle and it reflects our last write (or we
  // never wrote) — otherwise a lagging poll would snap the thumb backward.
  useEffect(() => {
    if (draggingRef.current) return;
    if (lastSentRef.current !== null && serverValue !== lastSentRef.current) return;
    lastSentRef.current = null;
    setValue(serverValue);
  }, [serverValue]);

  const flush = async () => {
    if (inflightRef.current || pendingRef.current === null) return;
    inflightRef.current = true;
    const v = pendingRef.current;
    pendingRef.current = null;
    lastSentRef.current = v;
    try {
      await commitRef.current(v);
    } finally {
      inflightRef.current = false;
      // A newer value may have queued while this write was in flight.
      void flush();
    }
  };

  const set = (v: number) => {
    setValue(v);
    pendingRef.current = v;
    void flush();
  };

  return {
    value,
    set,
    dragProps: {
      onPointerDown: () => { draggingRef.current = true; },
      onPointerUp: () => { draggingRef.current = false; },
      onPointerCancel: () => { draggingRef.current = false; },
    },
  };
}
