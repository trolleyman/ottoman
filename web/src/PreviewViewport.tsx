import { useLayoutEffect, useRef, useState, type ReactNode } from "react";

export function PreviewViewport({ interactive, children }: { interactive: boolean; children: ReactNode }) {
  const root = useRef<HTMLDivElement>(null);
  const [hiddenAbove, setHiddenAbove] = useState(false);
  useLayoutEffect(() => {
    const element = root.current!;
    const update = () => setHiddenAbove(element.scrollTop > 1);
    const observer = new ResizeObserver(update);
    observer.observe(element);
    if (element.firstElementChild) observer.observe(element.firstElementChild);
    element.addEventListener("scroll", update, { passive: true });
    update();
    return () => { observer.disconnect(); element.removeEventListener("scroll", update); };
  }, []);
  return <div ref={root} data-preview-scroll data-preview-interactive={interactive} data-preview-hidden-above={hiddenAbove || undefined}
    className={`${interactive ? "pointer-events-auto" : "pointer-events-none"} min-h-0 overflow-y-auto overscroll-contain rounded-lg border border-white/5 bg-zinc-950/65 shadow-lg backdrop-blur-md`}
    style={{ maskImage: hiddenAbove ? "linear-gradient(to bottom, transparent, black 22px)" : undefined }}>
    {children}
  </div>;
}
