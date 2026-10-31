import { useLayoutEffect, useRef, useState } from "react";

type Box = { left: number; top: number; width: number; height: number };

// Keep one uninterrupted text run: inserting an inline caret changes kerning,
// line wrapping and the space available to letters on either side of it.
export function CaretText({ value, cursor, selectionEnd = cursor, hideCaret = false, variant, className }: {
  value: string;
  cursor: number;
  selectionEnd?: number;
  hideCaret?: boolean;
  variant: "preview" | "search";
  className: string;
}) {
  const root = useRef<HTMLDivElement>(null);
  const text = useRef<HTMLSpanElement>(null);
  const [geometry, setGeometry] = useState<{ caret: Box; selection: Box[] } | null>(null);
  useLayoutEffect(() => {
    const container = root.current!;
    const node = text.current!.firstChild!;
    const measure = () => {
      const bounds = container.getBoundingClientRect();
      const box = (rect: DOMRect): Box => ({
        left: rect.left - bounds.left + container.scrollLeft,
        top: rect.top - bounds.top + container.scrollTop,
        width: rect.width,
        height: rect.height,
      });
      const start = Math.max(0, Math.min(cursor, value.length));
      const end = Math.max(start, Math.min(selectionEnd, value.length));
      const range = document.createRange();
      range.setStart(node, end);
      range.collapse(true);
      const caret = box(range.getBoundingClientRect());
      range.setStart(node, start);
      range.setEnd(node, end);
      const selection = start === end ? [] : Array.from(range.getClientRects(), box);
      // Search follows the end of a long query without squeezing the caret.
      if (variant === "search") container.scrollLeft = Math.max(0, caret.left - container.clientWidth + 2);
      setGeometry({ caret, selection });
    };
    // DOM measurements must precede paint so the overlay never trails typing.
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(container);
    return () => observer.disconnect();
  }, [value, cursor, selectionEnd, variant]);

  return <div ref={root} className={`relative ${className}`}>
    {/* The zero-width sentinel gives empty text and trailing newlines a caret box. */}
    <span ref={text} data-caret-text className="relative z-10">{value + "\u200b"}</span>
    {geometry?.selection.map((box, index) => <span key={index} aria-hidden="true" className="pointer-events-none absolute bg-blue-500/40" style={box} />)}
    {!hideCaret && geometry && <span key={`${value}:${selectionEnd}`} aria-hidden="true"
      data-preview-caret={variant === "preview" || undefined} data-search-caret={variant === "search" || undefined}
      className="keyboard-caret pointer-events-none absolute z-20 w-px bg-blue-300"
      style={{ left: geometry.caret.left, top: geometry.caret.top, height: geometry.caret.height }} />}
  </div>;
}
