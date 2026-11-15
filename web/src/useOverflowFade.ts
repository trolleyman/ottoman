import { useLayoutEffect, type RefObject } from "react";

// Fade only the scrollable text layer; its surrounding frame stays opaque.
export function useOverflowFade(ref: RefObject<HTMLElement | null>, content: unknown) {
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const update = () => {
      const above = element.scrollTop > 1;
      const below = element.scrollHeight - element.clientHeight - element.scrollTop > 1;
      for (const [name, hidden] of [["above", above], ["below", below]] as const) {
        if (hidden) element.setAttribute(`data-preview-hidden-${name}`, "true");
        else element.removeAttribute(`data-preview-hidden-${name}`);
      }
      element.style.maskImage = above || below
        ? `linear-gradient(to bottom, transparent, black ${above ? 8 : 0}px, black calc(100% - ${below ? 8 : 0}px), transparent)`
        : "none";
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    if (element.firstElementChild) observer.observe(element.firstElementChild);
    element.addEventListener("scroll", update, { passive: true });
    return () => { observer.disconnect(); element.removeEventListener("scroll", update); };
  }, [ref, content]);
}
