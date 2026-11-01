import { useSyncExternalStore, type MouseEvent } from "react";

function subscribe(onChange: () => void) {
  window.addEventListener("popstate", onChange);
  return () => window.removeEventListener("popstate", onChange);
}

export function usePathname() {
  return useSyncExternalStore(subscribe, () => window.location.pathname);
}

export function followAppLink(event: MouseEvent<HTMLAnchorElement>) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.currentTarget.target) return;
  const url = new URL(event.currentTarget.href);
  if (url.origin !== window.location.origin) return;
  event.preventDefault();
  if (url.href === window.location.href) return;
  window.history.pushState(null, "", url.href);
  window.dispatchEvent(new PopStateEvent("popstate"));
  window.scrollTo(0, 0);
}
