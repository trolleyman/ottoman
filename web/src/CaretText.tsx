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
  // Empty inline boxes sit at the visual end of an LTR line. Bidi text needs
  // its logical range endpoint instead, which may be on the opposite side.
  const inlineEnd = !/[\u0590-\u08ff\ufb1d-\ufdff\ufe70-\ufeff\u{10800}-\u{10fff}\u{1e800}-\u{1eeff}]/u.test(value);
  const root = useRef<HTMLDivElement>(null);
  const text = useRef<HTMLSpanElement>(null);
  const endAnchor = useRef<HTMLSpanElement>(null);
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
      const caret = box(end === value.length && inlineEnd ? endAnchor.current!.getBoundingClientRect() : range.getBoundingClientRect());
      range.setStart(node, start);
      range.setEnd(node, end);
      const selection = start === end ? [] : Array.from(range.getClientRects(), box);
      const last = selection.at(-1);
      // WebKit rounds range endpoints differently from inline text layout.
      // At the end, use the real inline anchor for a complete final highlight.
      if (last && inlineEnd && end === value.length && Math.abs(last.top - caret.top) < 1 && caret.left >= last.left) {
        last.width = caret.left - last.left;
      }
      // Search follows the end of a long query without squeezing the caret.
      if (variant === "search") container.scrollLeft = Math.max(0, caret.left - container.clientWidth + 2);
      setGeometry({ caret, selection });
    };
    // DOM measurements must precede paint so the overlay never trails typing.
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(container);
    // Font changes can alter glyph widths without resizing this block.
    let active = true;
    void document.fonts.ready.then(() => { if (active) measure(); });
    document.fonts.addEventListener("loadingdone", measure);
    return () => {
      active = false;
      observer.disconnect();
      document.fonts.removeEventListener("loadingdone", measure);
    };
  }, [value, cursor, selectionEnd, variant, inlineEnd]);

  const atEnd = inlineEnd && cursor === value.length && selectionEnd === cursor;
  const caret = !hideCaret && cursor === selectionEnd && <span key={`${value}:${cursor}`} aria-hidden="true"
    data-preview-caret={variant === "preview" || undefined} data-search-caret={variant === "search" || undefined}
    className="keyboard-caret pointer-events-none absolute z-20 w-px bg-blue-300"
    style={atEnd ? { left: 0, top: 0, height: "100%" } : geometry ? { left: geometry.caret.left, top: geometry.caret.top, height: geometry.caret.height } : { display: "none" }} />;

  return <div ref={root} className={`relative ${className}`}>
    {/* Keep the text uninterrupted. The empty inline anchor follows its real
        rendered end even when mobile font metrics change after measurement. */}
    <span ref={text} data-caret-text className="relative z-10">{value + "\u200b"}</span><span ref={endAnchor} aria-hidden="true" className="relative">{atEnd && caret}</span>
    {geometry?.selection.map((box, index) => <span key={index} aria-hidden="true" className="pointer-events-none absolute bg-blue-500/40" style={box} />)}
    {!atEnd && caret}
  </div>;
}
