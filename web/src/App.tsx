import { useState, useEffect, useCallback } from "react";
import { OttomanWithLogo } from "./OttomanWithLogo";
import { LoginForm } from "./LoginForm";
import { Trackpad } from "./Trackpad";
import { Monitors } from "./Monitors";
import { WakeTargets } from "./WakeTargets";
import { Layouts } from "./Layouts";

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  // refreshSignal triggers refreshes. silent=true avoids showing loading indicators for polling.
  const [refreshSignal, setRefreshSignal] = useState({ key: 0, silent: false });

  // Check auth on mount
  useEffect(() => {
    fetch("/api/auth/check")
      .then((res) => setAuthed(res.ok))
      .catch(() => setAuthed(false));
  }, []);

  // Periodic refresh
  useEffect(() => {
    if (!authed) return;
    const interval = setInterval(() => {
      setRefreshSignal((prev) => ({ key: prev.key + 1, silent: true }));
    }, 5000);
    return () => clearInterval(interval);
  }, [authed]);

  const refresh = useCallback(() => {
    setRefreshSignal((prev) => ({ key: prev.key + 1, silent: false }));
  }, []);

  const logout = async () => {
    await fetch("/api/auth/logout", { method: "POST" });
    setAuthed(false);
  };

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
    return <LoginForm onSuccess={() => setAuthed(true)} />;
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
              onClick={refresh}
              className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
            >
              Refresh
            </button>
            <button
              onClick={logout}
              className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
            >
              Log out
            </button>
          </div>
        </header>

        <WakeTargets
          authed={!!authed}
          refreshSignal={refreshSignal}
          onWake={refresh}
          onShutdown={refresh}
        />

        <Layouts authed={!!authed} refreshSignal={refreshSignal} onChange={refresh} />

        <Monitors authed={!!authed} refreshSignal={refreshSignal} />

        <Trackpad authed={!!authed} refreshSignal={refreshSignal} />
      </div>
    </div>
  );
}
