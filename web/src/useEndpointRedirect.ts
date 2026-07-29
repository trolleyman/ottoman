import { useEffect } from "react";
import type { StatusResponse } from "./api";

// Don't re-attempt a hop for this long after one was made, so a user who
// bounced back from an unreachable target isn't immediately redirected again.
const ATTEMPT_KEY = "ottoman-endpoint-redirect-at";
const ATTEMPT_COOLDOWN_MS = 60_000;

function normalizeOrigin(url: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return "";
  }
}

// useEndpointRedirect hops the SPA to the best reachable server endpoint.
// /api/status reports the origins to prefer (`endpoints`, best first), so a
// page loaded via a public tunnel moves to the lowest-latency origin that
// works — the controller's own LAN address. The agent is intentionally never
// advertised as a hop target: it sleeps / powers off, so redirecting straight
// to it would strand the UI whenever it's down.
//
// Each better-ranked candidate's /health is probed before navigating
// (cross-origin, allowed by the servers' HealthCORS middleware). On an https
// page (e.g. ngrok) mixed-content rules block probing http targets, so we stay
// put rather than guess at a hop that may strand the UI.
export function useEndpointRedirect(status: StatusResponse | null) {
  useEffect(() => {
    if (!status?.endpoints?.length) return;
    const endpoints = status.endpoints.map(normalizeOrigin).filter(Boolean);
    const rank = endpoints.indexOf(window.location.origin);
    const candidates = rank === -1 ? endpoints : endpoints.slice(0, rank);
    if (candidates.length === 0) return;

    const lastAttempt = Number(sessionStorage.getItem(ATTEMPT_KEY) ?? 0);
    if (Date.now() - lastAttempt < ATTEMPT_COOLDOWN_MS) return;

    const navigate = (target: string) => {
      sessionStorage.setItem(ATTEMPT_KEY, String(Date.now()));
      window.location.href =
        target + window.location.pathname + window.location.search + window.location.hash;
    };

    if (window.location.protocol === "https:") return;

    let cancelled = false;
    void (async () => {
      for (const target of candidates) {
        try {
          const controller = new AbortController();
          const timeoutId = setTimeout(() => controller.abort(), 1500);
          const resp = await fetch(`${target}/health`, { signal: controller.signal });
          clearTimeout(timeoutId);
          if (cancelled) return;
          if (resp.ok) {
            navigate(target);
            return;
          }
        } catch {
          // Unreachable from this client — try the next candidate.
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [status]);
}
