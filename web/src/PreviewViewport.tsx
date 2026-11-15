import { useRef, type ReactNode } from "react";
import { useOverflowFade } from "./useOverflowFade";

export function PreviewViewport({ children }: { children: ReactNode }) {
  const root = useRef<HTMLDivElement>(null);
  useOverflowFade(root, children);
  return <div className="preview-text-frame pointer-events-auto min-h-0 overflow-hidden rounded-lg border border-white/5 bg-zinc-950/65 shadow-lg backdrop-blur-md">
    <div ref={root} data-preview-scroll className="min-h-0 overflow-y-auto overscroll-contain touch-pan-y">{children}</div>
  </div>;
}
