import { useEffect, useState } from "react";
import { Music2, Pause, Play, SkipBack, SkipForward } from "lucide-react";

type MediaState = { available: boolean; player: string; title: string; artist: string; playing: boolean; canToggle: boolean; canNext: boolean; canPrevious: boolean };
export function NowPlaying({ active }: { active: boolean }) {
  const [state, setState] = useState<MediaState | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    let running = false;
    const poll = async () => {
      if (running) return; running = true;
      try {
        const response = await fetch("/api/media", { signal: controller.signal, cache: "no-store" });
        if (!response.ok) { if (!controller.signal.aborted) setState(null); return; }
        const next = await response.json() as MediaState;
        if (!controller.signal.aborted) setState(next.available ? next : null);
      } catch { if (!controller.signal.aborted) setState(null); }
      finally { running = false; }
    };
    void poll(); const timer = setInterval(() => void poll(), 3000);
    return () => { controller.abort(); clearInterval(timer); };
  }, [active]);
  if (!active || !state) return null;
  const control = async (action: string) => {
    if (busy) return; setBusy(true); setError("");
    try {
      const response = await fetch("/api/media", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ player: state.player, action }), signal: AbortSignal.timeout(5000) });
      if (!response.ok) throw Error("Player unavailable.");
      if (action === "toggle") setState(current => current ? { ...current, playing: !current.playing } : null);
    } catch { setError("Player unavailable."); } finally { setBusy(false); }
  };
  return <div className="remote-now-playing" aria-label="Now playing"><div className="remote-media-art"><Music2 /></div><div className="remote-media-track"><strong>{state.title || "Media"}</strong>{state.artist && <span>{state.artist}</span>}</div><div className="remote-media-controls"><button aria-label="Previous track" disabled={busy || !state.canPrevious} onClick={() => void control("previous")}><SkipBack /></button><button aria-label={state.playing ? "Pause" : "Play"} disabled={busy || !state.canToggle} onClick={() => void control("toggle")}>{state.playing ? <Pause /> : <Play />}</button><button aria-label="Next track" disabled={busy || !state.canNext} onClick={() => void control("next")}><SkipForward /></button></div>{error && <p role="status">{error}</p>}</div>;
}
