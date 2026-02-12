import { useState, useRef, useCallback, useEffect } from "react";
import type { Layout } from "./types";
import { MiniLayoutPreview } from "./MiniLayoutPreview";

export function MonitorDisplay({
  layouts,
  currentLayout,
  cursorPos,
  connected,
  loading,
  onSetPosition,
}: {
  layouts: Layout[];
  currentLayout: string;
  cursorPos: { x: number; y: number } | null;
  connected: boolean;
  loading?: boolean;
  onSetPosition?: (x: number, y: number) => void;
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

  const layout = layouts.find((l) => l.id === currentLayout) ?? layouts[0];
  const monitors = layout?.monitors ?? [];
  const effectiveWidth = containerWidth || 300;

  // No monitor data — show placeholder
  if (monitors.length === 0) {
    // Assume 1920x1080 reference for scale
    const scale = effectiveWidth / 1920;
    const placeholderW = Math.min(effectiveWidth, 1920 * scale);
    const placeholderH = 1080 * scale;
    const isUnknown = !loading;

    return (
      <div ref={containerRef} className="flex-1 min-w-0 w-full flex flex-col items-center gap-1">
        <div
          className={`w-full rounded-lg border flex items-center justify-center transition-colors ${isUnknown ? "border-red-500/50 bg-zinc-900/40 border-dashed" : "border-zinc-700/50 bg-zinc-800/20 border-dashed"}`}
          style={{ height: placeholderH, maxWidth: placeholderW }}
        >
          <span className={`text-sm ${isUnknown ? "text-red-400" : "text-zinc-600 animate-pulse"}`}>{loading ? "Loading..." : "Unknown"}</span>
        </div>
      </div>
    );
  }

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const m of monitors) {
    minX = Math.min(minX, m.position_x);
    minY = Math.min(minY, m.position_y);
    maxX = Math.max(maxX, m.position_x + m.width);
    maxY = Math.max(maxY, m.position_y + m.height);
  }
  const totalW = maxX - minX;
  const totalH = maxY - minY;
  if (totalW <= 0 || totalH <= 0) return <div ref={containerRef} className="flex-1 min-w-0 w-full" />;

  const maxH = 300;
  const scale = Math.min(effectiveWidth / totalW, maxH / totalH);

  const hasCursor = connected && cursorPos;
  const dotX = hasCursor ? (cursorPos!.x - minX) * scale : 0;
  const dotY = hasCursor ? (cursorPos!.y - minY) * scale : 0;

  const handlePointer = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!onSetPosition) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    const rect = e.currentTarget.getBoundingClientRect();
    const x = Math.round((e.clientX - rect.left) / scale + minX);
    const y = Math.round((e.clientY - rect.top) / scale + minY);
    onSetPosition(x, y);
  };

  return (
    <div
      ref={containerRef}
      className="flex-1 min-w-0 w-full flex flex-col items-center gap-1 touch-none"
      onPointerDown={(e) => {
        if (!onSetPosition) return;
        handlePointer(e);
      }}
      onPointerMove={(e) => {
        if (e.buttons > 0) handlePointer(e);
      }}
    >
      <div className="relative">
        <MiniLayoutPreview monitors={monitors} scale={scale} />
        {hasCursor && (
          <div
            className="absolute w-2 h-2 rounded-full bg-red-500 -translate-x-1/2 -translate-y-1/2 z-10 shadow-[0_0_4px_rgba(239,68,68,0.7)]"
            style={{ left: dotX, top: dotY }}
          />
        )}
      </div>
      {hasCursor && (
        <span className="text-[10px] text-zinc-500 font-mono">
          {cursorPos!.x}, {cursorPos!.y}
        </span>
      )}
    </div>
  );
}
