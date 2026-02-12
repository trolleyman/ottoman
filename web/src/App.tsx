import { useState, useEffect, useCallback } from "react";
import { OttomanWithLogo } from "./OttomanWithLogo";
import { LoginForm } from "./LoginForm";
import { Trackpad } from "./Trackpad";
import { Monitors } from "./Monitors";
import { WakeTargets } from "./WakeTargets";
import { Layouts } from "./Layouts";

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  // refreshKey is used to trigger a refresh of all components when the user clicks "Refresh"
  // or when a layout switch happens (which affects monitors and current layout)
  const [refreshKey, setRefreshKey] = useState(0);

  // Check auth on mount
  useEffect(() => {
    fetch("/api/auth/check")
      .then((res) => setAuthed(res.ok))
      .catch(() => setAuthed(false));
  }, []);

  const refresh = useCallback(() => {
    setRefreshKey((k) => k + 1);
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

        <WakeTargets authed={!!authed} refreshKey={refreshKey} />

        <Layouts authed={!!authed} refreshKey={refreshKey} onChange={refresh} />

        <Monitors authed={!!authed} refreshKey={refreshKey} />

        <Trackpad authed={!!authed} refreshKey={refreshKey} />
      </div>
    </div>
  );
}
