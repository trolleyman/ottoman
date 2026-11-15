import { useEffect } from "react";
import { LoginForm } from "./LoginForm";
import { RemotePage } from "./RemotePage";
import { useTrackpadWebSocket } from "./useTrackpadWebSocket";

import { useStore } from "./store";

export default function App() {
  const authed = useStore((s) => s.authed);
  const checkAuth = useStore((s) => s.checkAuth);
  const refreshAll = useStore((s) => s.refreshAll);
  const startPolling = useStore((s) => s.startPolling);
  const stopPolling = useStore((s) => s.stopPolling);
  const refreshKey = useStore((s) => s.refreshKey);

  const { textContext, connected, connecting, cursorPos, cursorSupported, send } = useTrackpadWebSocket(!!authed, refreshKey);

  useEffect(() => {
    const manifest = document.querySelector<HTMLLinkElement>('link[rel="manifest"]');
    if (manifest) manifest.href = "/remote.webmanifest";
  }, []);

  // Check auth on mount
  useEffect(() => {
    void checkAuth();
  }, [checkAuth]);

  // Start polling when authenticated
  useEffect(() => {
    if (!authed) return;
    void refreshAll(false);
    startPolling();
    return () => stopPolling();
    // refreshAll/startPolling/stopPolling are stable Zustand actions.
  }, [authed, refreshAll, startPolling, stopPolling]);

  // Auth check pending
  if (authed === null) {
    return (
      <div className="min-h-screen bg-zinc-950 flex items-center justify-center">
        <div className="text-zinc-500 text-sm">Loading...</div>
      </div>
    );
  }

  // Not authenticated — show login
  if (!authed) {
    return <LoginForm />;
  }

  return <RemotePage textContext={textContext} connected={connected} connecting={connecting} cursorPos={cursorPos} cursorSupported={cursorSupported} send={send} />;
}
