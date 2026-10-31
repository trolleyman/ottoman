import { useEffect, useRef, useState } from "react";
import type { KeyPositions, Point } from "./swipeTyping";

export function useSwipeTyping(enabled: boolean, holdDelay: number, context: number, onWord: (word: string, previous?: string) => void) {
  const decoder = useRef<typeof import("./swipeDictionary").decodeSwipe | null>(null);
  const gesture = useRef<{ pointer: number; start: number; points: Point[]; keys: KeyPositions; active: boolean } | null>(null);
  const previousWord = useRef({ word: "", context });
  const consumed = useRef(false);
  const [active, setActive] = useState(false);
  const [trail, setTrail] = useState<Point[]>([]);
  const [suggestions, setSuggestions] = useState<{ context: number; words: string[] }>({ context, words: [] });
  const candidates = suggestions.context === context ? suggestions.words : [];
  const [ready, setReady] = useState(false);
  useEffect(() => {
    if (!enabled) return;
    let disposed = false;
    void import("./swipeDictionary").then((module) => {
      if (disposed) return;
      decoder.current = module.decodeSwipe;
      setReady(true);
    }).catch(() => { /* Tap typing remains available if the dictionary cannot load. */ });
    return () => { disposed = true; };
  }, [enabled]);

  const clear = () => { setSuggestions({ context, words: [] }); previousWord.current = { context, word: "" }; };
  const stop = () => { gesture.current = null; setActive(false); setTrail([]); };
  const point = (event: React.PointerEvent<HTMLDivElement>) => {
    const box = event.currentTarget.getBoundingClientRect();
    return { x: event.clientX - box.left, y: event.clientY - box.top };
  };
  return {
    active, trail, candidates, ready, clear,
    suppressTap: () => consumed.current,
    select(word: string) {
      onWord(word, previousWord.current.context === context ? previousWord.current.word : undefined);
      previousWord.current = { context, word };
    },
    handlers: {
      onPointerDownCapture(event: React.PointerEvent<HTMLDivElement>) {
        consumed.current = false;
        if (!enabled || !decoder.current || !event.isPrimary) return;
        const target = event.target as HTMLElement;
        if (!target.closest("[data-letter]")) return;
        const box = event.currentTarget.getBoundingClientRect();
        const keys: KeyPositions = {};
        event.currentTarget.querySelectorAll<HTMLElement>("[data-letter]").forEach((key) => {
          const rect = key.getBoundingClientRect();
          keys[key.dataset.letter!] = { x: rect.x + rect.width / 2 - box.x, y: rect.y + rect.height / 2 - box.y };
        });
        gesture.current = { pointer: event.pointerId, start: performance.now(), points: [point(event)], keys, active: false };
        event.preventDefault();
      },
      onPointerMoveCapture(event: React.PointerEvent<HTMLDivElement>) {
        const current = gesture.current;
        if (!current || current.pointer !== event.pointerId) return;
        const next = point(event);
        const first = current.points[0];
        const width = Math.abs(current.keys.q.x - current.keys.w.x);
        if (!current.active) {
          if (performance.now() - current.start >= holdDelay) { gesture.current = null; return; }
          if (Math.hypot(next.x - first.x, next.y - first.y) < width * 0.7) return;
          current.active = true;
          consumed.current = true;
          setActive(true);
          clear();
        }
        event.preventDefault();
        const last = current.points[current.points.length - 1];
        if (Math.hypot(next.x - last.x, next.y - last.y) > 2) {
          current.points.push(next);
          // Bound work for a very long gesture without dropping its first point.
          if (current.points.length > 300) current.points = current.points.filter((_, index) => index % 2 === 0);
          setTrail([...current.points]);
        }
      },
      onPointerUpCapture(event: React.PointerEvent<HTMLDivElement>) {
        const current = gesture.current;
        if (!current || current.pointer !== event.pointerId) return;
        if (current.active) {
          current.points.push(point(event));
          const words = decoder.current?.(current.points, current.keys) ?? [];
          setSuggestions({ context, words });
          if (words[0]) {
            onWord(words[0]);
            previousWord.current = { context, word: words[0] };
          }
        }
        stop();
      },
      onPointerCancelCapture: stop,
    },
  };
}
