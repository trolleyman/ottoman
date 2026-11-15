import { useLayoutEffect, useRef, type RefObject } from "react";
import { previewPositions } from "./previewNavigation";

export function usePhoneTextLayout(inputRef: RefObject<HTMLTextAreaElement | null>, rulerRef: RefObject<HTMLDivElement | null>, content: string | null, caret: number) {
  const previous = useRef<string | null>(null);
  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!input || content === null) { previous.current = null; return; }
    // Measure an unfocused copy: changing the live textarea's height to zero
    // would disturb native selection handles and its scroll position on iOS.
    const measure = document.createElement("textarea");
    measure.className = "remote-native-text remote-text-measure";
    measure.setAttribute("aria-hidden", "true");
    measure.tabIndex = -1;
    measure.readOnly = true;
    input.parentElement!.append(measure);
    const fit = () => {
      const style = getComputedStyle(input);
      measure.style.width = `${input.getBoundingClientRect().width}px`;
      measure.value = input.value;
      const border = parseFloat(style.borderTopWidth) + parseFloat(style.borderBottomWidth);
      const padding = parseFloat(style.paddingTop) + parseFloat(style.paddingBottom);
      const line = parseFloat(style.lineHeight);
      const height = Math.max(line + padding + border, Math.min(measure.scrollHeight + border, line * 2 + padding + border));
      input.parentElement!.style.setProperty("--remote-field-height", `${height}px`);
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(input);
    const changed = previous.current !== content;
    previous.current = content;
    if (changed && caret === content.length) {
      input.scrollTop = input.scrollHeight;
      if (rulerRef.current) rulerRef.current.scrollTop = input.scrollTop;
    } else if (changed && rulerRef.current) {
      const ruler = rulerRef.current;
      ruler.scrollTop = input.scrollTop;
      const point = previewPositions(ruler).find(point => point.offset === caret);
      const bounds = input.getBoundingClientRect();
      if (point) {
        if (point.y < bounds.top + 8) input.scrollTop -= bounds.top + 8 - point.y;
        else if (point.y + point.height > bounds.bottom - 8) input.scrollTop += point.y + point.height - bounds.bottom + 8;
        ruler.scrollTop = input.scrollTop;
      }
    }
    return () => { observer.disconnect(); measure.remove(); };
  }, [inputRef, rulerRef, content, caret]);
}
