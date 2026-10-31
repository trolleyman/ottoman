import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronUp } from "lucide-react";
import type { TrackpadMessage } from "./api";
import { haptic } from "./keyboardHaptics";

export function ScrollRail({ send, sensitivity, friction, haptics, onInteract }: {
  send: (message: TrackpadMessage) => void;
  sensitivity: number;
  friction: number;
  haptics: boolean;
  onInteract: () => void;
}) {
  const motion = useRef({ y: 0, time: 0, velocity: 0, frame: 0, pointer: -1, remainder: 0, feedback: 0 });
  const [phase, setPhase] = useState<"idle" | "dragging" | "coasting">("idle");
  const [thumb, setThumb] = useState(50);
  useEffect(() => () => cancelAnimationFrame(motion.current.frame), []);

  const emit = (delta: number) => {
    const state = motion.current;
    state.remainder += delta;
    const dy = Math.trunc(state.remainder);
    if (dy) {
      state.remainder -= dy;
      send({ type: "mousescroll", dx: 0, dy, precise: true });
    }
  };
  const finish = (cancelled: boolean) => {
    const state = motion.current;
    state.pointer = -1;
    if (cancelled || performance.now() - state.time > 90 || Math.abs(state.velocity) < 0.03) {
      setPhase("idle");
      return;
    }
    setPhase("coasting");
    let last = performance.now();
    const coast = (now: number) => {
      const dt = Math.min(now - last, 32);
      last = now;
      state.velocity *= Math.pow(friction, dt / 16);
      emit(state.velocity * dt);
      setThumb((value) => Math.max(15, Math.min(85, value - state.velocity * dt / 12)));
      if (Math.abs(state.velocity) > 0.015) state.frame = requestAnimationFrame(coast);
      else setPhase("idle");
    };
    state.frame = requestAnimationFrame(coast);
  };
  return (
    <div role="group" aria-label="One-finger scroll control" data-scroll-phase={phase}
      className={`absolute inset-y-3 right-2 flex w-10 touch-none select-none flex-col items-center justify-between rounded-full border py-3 shadow-lg backdrop-blur transition-colors ${phase === "idle" ? "border-zinc-600/80 bg-zinc-950/80 text-zinc-400" : "border-blue-400/70 bg-blue-950/90 text-blue-200 ring-2 ring-blue-400/15"}`}
      onPointerDown={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onInteract();
        event.currentTarget.setPointerCapture(event.pointerId);
        const state = motion.current;
        cancelAnimationFrame(state.frame);
        Object.assign(state, { y: event.clientY, time: performance.now(), velocity: 0, pointer: event.pointerId, remainder: 0, feedback: 0 });
        setPhase("dragging");
        haptic(haptics);
      }}
      onPointerMove={(event) => {
        const state = motion.current;
        if (state.pointer !== event.pointerId) return;
        event.preventDefault();
        event.stopPropagation();
        const now = performance.now();
        const delta = (state.y - event.clientY) * sensitivity;
        const dt = Math.max(8, now - state.time);
        const instantaneous = Math.max(-3, Math.min(3, delta / dt));
        state.velocity = dt > 80 ? instantaneous : state.velocity * 0.25 + instantaneous * 0.75;
        state.y = event.clientY;
        state.time = now;
        emit(delta);
        const box = event.currentTarget.getBoundingClientRect();
        setThumb(Math.max(15, Math.min(85, (event.clientY - box.top) / box.height * 100)));
        state.feedback += Math.abs(delta);
        if (state.feedback > 50) { haptic(haptics, 4); state.feedback = 0; }
      }}
      onPointerUp={() => finish(false)}
      onPointerCancel={() => finish(true)}
      onLostPointerCapture={() => { if (motion.current.pointer !== -1) finish(true); }}
      onContextMenu={(event) => event.preventDefault()}
    >
      <ChevronUp className="h-4 w-4" />
      <div className="absolute inset-y-10 w-px bg-zinc-700/60" />
      <div className={`absolute h-10 w-1.5 -translate-y-1/2 rounded-full ${phase === "idle" ? "bg-zinc-600" : "bg-blue-300 shadow-[0_0_12px_rgba(147,197,253,.35)]"}`}
        style={{ top: `${thumb}%` }} />
      <ChevronDown className="h-4 w-4" />
    </div>
  );
}
