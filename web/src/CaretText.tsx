import { useId, useLayoutEffect, useRef, useState } from "react";

type Box = { left: number; top: number; width: number; height: number };

// Keep one uninterrupted text run: inserting an inline caret changes kerning,
// line wrapping and the space available to letters on either side of it.
export function CaretText({ value, cursor, selectionEnd = cursor, hideCaret = false, dragging = false, variant, className, colors }: {
  colors?: { start: number; end: number; color: string }[];
  value: string;
  cursor: number;
  selectionEnd?: number;
  hideCaret?: boolean;
  dragging?: boolean;
  variant: "preview" | "search";
  className: string;
}) {
  // Empty inline boxes sit at the visual end of an LTR line. Bidi text needs
  // its logical range endpoint instead, which may be on the opposite side.
  const inlineEnd = !/[\u0590-\u08ff\ufb1d-\ufdff\ufe70-\ufeff\u{10800}-\u{10fff}\u{1e800}-\u{1eeff}]/u.test(value);
  const root = useRef<HTMLDivElement>(null);
  const text = useRef<HTMLSpanElement>(null);
  const endMarker = useRef<HTMLSpanElement>(null);

  const highlightId = useId().replace(/[^a-zA-Z0-9]/g, "");
  useLayoutEffect(() => {
    // CSS highlights keep the single text node used by caret measurement and
    // hit testing. Browsers without the API retain a monochrome preview.
    if (!colors?.length || !CSS.highlights || typeof Highlight === "undefined") return;
    const node = text.current!.firstChild!;
    const names: string[] = [];
    const rules: string[] = [];
    for (const [index, run] of (colors ?? []).slice(0, 64).entries()) {
      if (!/^#[0-9a-f]{6}$/i.test(run.color) || run.start < 0 || run.end > value.length || run.end <= run.start) continue;
      const range = document.createRange();
      range.setStart(node, run.start); range.setEnd(node, run.end);
      const name = `terminal-${highlightId}-${index}`;
      CSS.highlights.set(name, new Highlight(range));
      names.push(name);
      rules.push(`::highlight(${name}) { color: ${run.color}; }`);
    }
    const style = document.createElement("style");
    style.textContent = rules.join("\n");
    document.head.append(style);
    return () => { names.forEach(name => CSS.highlights.delete(name)); style.remove(); };
  }, [colors, value, highlightId]);

  const [geometry, setGeometry] = useState<{ caret: Box; selection: Box[] } | null>(null);
  useLayoutEffect(() => {
    const container = root.current!;
    const node = text.current!.firstChild!;
    const measureGeometry = (followCaret: boolean) => {
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
      let caretRect = range.getBoundingClientRect();
      if (!caretRect.height) { range.setEnd(node, Math.min(end + 1, node.textContent!.length)); caretRect = range.getBoundingClientRect(); }
      // Use the browser's real inline layout at the end, rather than a text
      // range edge: WebKit can report the latter inside the final glyph.
      if (inlineEnd && end === value.length) {
        const marker = endMarker.current!.getBoundingClientRect();
        // A real zero-width glyph supplies both coordinates on the endpoint's
        // line. Combining its x with the previous glyph's y breaks at wraps.
        if (marker.height) caretRect = new DOMRect(marker.left, marker.top, 0, marker.height);
      }
      const caret = box(caretRect);
      range.setStart(node, start);
      range.setEnd(node, end);
      const selection = start === end ? [] : Array.from(range.getClientRects(), box);
      const last = selection.at(-1);
      // Align a final selection highlight with the measured inline endpoint.
      if (last && inlineEnd && end === value.length && Math.abs(last.top - caret.top) < 1 && caret.left >= last.left) {
        last.width = caret.left - last.left;
      }
      // Search follows the end of a long query without squeezing the caret.
      if (variant === "search") container.scrollLeft = Math.max(0, caret.left - container.clientWidth + 2);
      const viewport = container.closest<HTMLElement>("[data-preview-scroll]");
      if (viewport && !hideCaret && followCaret) {
        const visible = viewport.getBoundingClientRect();
        if (caretRect.bottom > visible.bottom) viewport.scrollTop += caretRect.bottom - visible.bottom + 4;
        else if (caretRect.top < visible.top) viewport.scrollTop -= visible.top - caretRect.top + 4;
      }
      setGeometry({ caret, selection });
    };
    const measure = () => measureGeometry(true);
    // DOM measurements must precede paint so the overlay never trails typing.
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(container);
    // Scroll-fade style changes must not pull a manually scrolled terminal
    // back to its caret while inherited font metrics are remeasured.
    const metrics = new MutationObserver(() => measureGeometry(false));
    const surface = container.closest("[data-text-preview]") ?? container;
    let ancestor: HTMLElement | null = container;
    while (ancestor) {
      metrics.observe(ancestor, { attributes: true, attributeFilter: ["style", "class"] });
      if (ancestor === surface) break;
      ancestor = ancestor.parentElement;
    }
    // Font changes can alter glyph widths without resizing this block.
    let active = true;
    void document.fonts.ready.then(() => { if (active) measure(); });
    document.fonts.addEventListener("loadingdone", measure);
    return () => {
      active = false;
      observer.disconnect();
      metrics.disconnect();
      document.fonts.removeEventListener("loadingdone", measure);
    };
  }, [value, cursor, selectionEnd, variant, inlineEnd, hideCaret]);

  useLayoutEffect(() => {
    const container = root.current!;
    container.dataset.caretMoving = "true";
    const timer = setTimeout(() => { delete container.dataset.caretMoving; }, 450);
    return () => clearTimeout(timer);
  }, [value, cursor, selectionEnd]);

  const caret = !hideCaret && (cursor === selectionEnd || dragging) && <span aria-hidden="true"
    data-preview-caret={variant === "preview" || undefined} data-search-caret={variant === "search" || undefined}
    className="keyboard-caret pointer-events-none absolute z-20 w-px bg-blue-300"
    data-caret-parked={dragging || undefined}
    style={geometry ? { background: dragging ? "#a1a1aa" : undefined, left: geometry.caret.left, top: geometry.caret.top, height: geometry.caret.height } : { display: "none" }} />;

  return <div ref={root} data-cursor-dragging={dragging || undefined} className={`relative ${className}`}>
    {/* One uninterrupted text run keeps wrapping stable as the caret moves. */}
    <span ref={text} data-caret-text className="relative z-10">{value + "\u200b"}</span><span ref={endMarker} data-caret-end-marker aria-hidden="true" >{"\u2060"}</span>
    {geometry?.selection.map((box, index) => <span key={index} aria-hidden="true" className="pointer-events-none absolute bg-blue-500/40" style={box} />)}
    {caret}
  </div>;
}
