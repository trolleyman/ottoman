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
  const [contentOffset, setContentOffset] = useState(0);
  const [direction, setDirection] = useState<"up" | "down">("up");
  useEffect(() => () => cancelAnimationFrame(motion.current.frame), []);

  const emit = (delta: number) => {
    const state = motion.current;
    state.remainder += delta;
    const dy = Math.trunc(state.remainder);
    if (dy) {
      state.remainder -= dy;
      send({ type: "mousescroll", dx: 0, dy, precise: true });
      setDirection(dy > 0 ? "up" : "down");
      setContentOffset((value) => (value - dy * 0.35) % 18);
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
      if (Math.abs(state.velocity) > 0.015) state.frame = requestAnimationFrame(coast);
      else setPhase("idle");
    };
    state.frame = requestAnimationFrame(coast);
  };
  return (
    <div role="group" aria-label="One-finger scroll control" data-scroll-phase={phase} data-scroll-direction={direction}
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
        state.feedback += Math.abs(delta);
        if (state.feedback > 50) { haptic(haptics, 4); state.feedback = 0; }
      }}
      onPointerUp={() => finish(false)}
      onPointerCancel={() => finish(true)}
      onLostPointerCapture={() => { if (motion.current.pointer !== -1) finish(true); }}
      onContextMenu={(event) => event.preventDefault()}
    >
      <ChevronUp className={`h-4 w-4 ${phase !== "idle" && direction === "up" ? "text-blue-200" : "text-zinc-600"}`} />
      <div aria-hidden="true" className="relative h-28 w-6 overflow-hidden" style={{ maskImage: "linear-gradient(transparent,black 20%,black 80%,transparent)" }}>
        <div className="absolute inset-x-0 -top-5 flex flex-col gap-3" style={{ transform: `translateY(${contentOffset}px)` }}>
          {Array.from({ length: 11 }, (_, index) => <span key={index} className={`h-1 rounded-full ${phase === "idle" ? "bg-zinc-700" : "bg-blue-300/80"}`} style={{ width: index % 3 === 2 ? "65%" : "100%" }} />)}
        </div>
      </div>
      <ChevronDown className={`h-4 w-4 ${phase !== "idle" && direction === "down" ? "text-blue-200" : "text-zinc-600"}`} />
    </div>
  );
}
