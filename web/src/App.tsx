import { useEffect } from "react";
import { OttomanWithLogo } from "./OttomanWithLogo";
import { LoginForm } from "./LoginForm";
import { Trackpad } from "./Trackpad";
import { useTrackpadWebSocket } from "./useTrackpadWebSocket";

import { Monitors } from "./Monitors";
import { Audio } from "./Audio";
import { ClientStatus } from "./ClientStatus";
import { Layouts } from "./Layouts";
import { useStore } from "./store";
import { useEndpointRedirect } from "./useEndpointRedirect";

export default function App() {
  const authed = useStore((s) => s.authed);
  const status = useStore((s) => s.status);
  const checkAuth = useStore((s) => s.checkAuth);
  const refreshAll = useStore((s) => s.refreshAll);
  const startPolling = useStore((s) => s.startPolling);
  const stopPolling = useStore((s) => s.stopPolling);
  const logout = useStore((s) => s.logout);
  const refreshKey = useStore((s) => s.refreshKey);

  const { connected, connecting, cursorPos, cursorSupported, send } = useTrackpadWebSocket(!!authed, refreshKey);

  // Hop to a better endpoint (direct agent > controller) when one is reachable.
  useEndpointRedirect(status);

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

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 overflow-x-hidden">
      <div className="max-w-6xl mx-auto px-6 py-10 space-y-10">
        {/* Header */}
        <header className="flex items-center justify-between">
          <OttomanWithLogo>
            <p className="text-zinc-500 italic text-sm">Display Management</p>
          </OttomanWithLogo>
          <div className="flex items-center gap-4">
            <button
              onClick={() => void refreshAll(false)}
              className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
            >
              Refresh
            </button>
            <button
              onClick={() => void logout()}
              className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
            >
              Log out
            </button>
          </div>
        </header>

        <ClientStatus />

        <Layouts />

        <Monitors />

        <Audio />

        <Trackpad
          connected={connected}
          connecting={connecting}
          cursorPos={cursorPos}
          cursorSupported={cursorSupported}
          send={send}
        />

      </div>
    </div>
  );
}
