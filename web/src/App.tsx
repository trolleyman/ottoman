import { useEffect } from "react";
import { followAppLink, usePathname } from "./navigation";
import { OttomanWithLogo } from "./OttomanWithLogo";
import { LoginForm } from "./LoginForm";
import { RemotePage } from "./RemotePage";
import { Trackpad } from "./Trackpad";
import { useTrackpadWebSocket } from "./useTrackpadWebSocket";

import { Monitors } from "./Monitors";
import { Audio } from "./Audio";
import { ClientStatus } from "./ClientStatus";
import { Layouts } from "./Layouts";
import { useStore } from "./store";

export default function App() {
  const pathname = usePathname();
  const authed = useStore((s) => s.authed);
  const checkAuth = useStore((s) => s.checkAuth);
  const refreshAll = useStore((s) => s.refreshAll);
  const startPolling = useStore((s) => s.startPolling);
  const stopPolling = useStore((s) => s.stopPolling);
  const logout = useStore((s) => s.logout);
  const refreshKey = useStore((s) => s.refreshKey);

  const { textContext, connected, connecting, cursorPos, cursorSupported, send } = useTrackpadWebSocket(!!authed, refreshKey);

  useEffect(() => {
    const manifest = document.querySelector<HTMLLinkElement>('link[rel="manifest"]');
    if (manifest) manifest.href = pathname === "/remote" ? "/remote.webmanifest" : "/site.webmanifest";
  }, [pathname]);

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

  if (pathname === "/remote") return <RemotePage textContext={textContext} connected={connected} connecting={connecting} cursorPos={cursorPos} cursorSupported={cursorSupported} send={send} />;

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 overflow-x-hidden">
      <div className="max-w-6xl mx-auto px-4 py-6 sm:px-6 sm:py-10 space-y-10">
        {/* Header */}
        <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <OttomanWithLogo>
            <p className="text-zinc-500 italic text-sm">Display Management</p>
          </OttomanWithLogo>
          <nav aria-label="Dashboard actions" className="flex flex-wrap items-center gap-x-5 gap-y-3 self-end whitespace-nowrap sm:shrink-0">
            <a href="/remote" onClick={followAppLink} className="text-xs text-blue-400 hover:text-blue-300">PC remote</a>
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
          </nav>
        </header>

        <ClientStatus />

        <Layouts />

        <Monitors />

        <Audio />

        <Trackpad
          textContext={textContext}
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
