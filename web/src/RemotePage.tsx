import { useEffect, useState } from "react";
import { ArrowLeft, Maximize, Minimize, Monitor, Volume2, VolumeX } from "lucide-react";
import { Trackpad } from "./Trackpad";
import { useStore } from "./store";
import { useCoalescedSlider } from "./useCoalescedSlider";
import type { TrackpadMessage, TrackpadMessageTextContext } from "./api";

function RemoteVolume({ label, value, muted, onVolume, onMute }: {
  label: string; value: number; muted: boolean; onVolume: (value: number) => Promise<void>; onMute: () => void;
}) {
  const slider = useCoalescedSlider(value, onVolume);
  return <div className="min-w-0 rounded-xl border border-white/8 bg-white/3 p-3">
    <div className="mb-2 flex items-center justify-between gap-2 text-xs"><span className="font-medium text-zinc-300">{label}</span><span className="tabular-nums text-zinc-500">{slider.value}%</span></div>
    <div className="flex items-center gap-3">
      <button type="button" aria-label={`${muted ? "Unmute" : "Mute"} ${label}`} onClick={onMute} className="text-zinc-400 hover:text-white">{muted ? <VolumeX className="h-4 w-4" /> : <Volume2 className="h-4 w-4" />}</button>
      <input type="range" aria-label={label} min={0} max={100} value={slider.value} {...slider.dragProps} onChange={(event) => slider.set(Number(event.target.value))} className="min-w-0 flex-1 accent-blue-400" />
    </div>
  </div>;
}

export function RemotePage(props: {
  textContext: TrackpadMessageTextContext | null;
  connected: boolean; connecting: boolean; cursorPos: { x: number; y: number } | null;
  cursorSupported: boolean; send: (message: TrackpadMessage) => void;
}) {
  const { agentInfo, layouts, currentLayout, switchLayout, audioSinks, audioError, monitors, setSinkVolume, setSinkMute, setMonitorVolume, setMonitorMute } = useStore();
  const [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const [fullscreenError, setFullscreenError] = useState("");
  useEffect(() => {
    const changed = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", changed);
    return () => document.removeEventListener("fullscreenchange", changed);
  }, []);
  const sink = audioSinks.find((item) => item.default);
  const tvs = monitors.filter((item) => item.capabilities?.volume && item.tv_state?.paired);
  return <main className="pc-remote bg-zinc-950 text-zinc-100">
    <header className="flex shrink-0 items-center gap-3 border-b border-white/8 pb-3">
      <a href="/" aria-label="Back to dashboard" className="rounded-lg border border-white/10 p-2 text-zinc-400 hover:text-white"><ArrowLeft className="h-4 w-4" /></a>
      <div className="min-w-0 flex-1"><div className="flex items-center gap-2 text-sm font-semibold"><Monitor className="h-4 w-4 text-blue-400" />PC remote</div><div className="mt-0.5 flex items-center gap-1.5 text-[11px] text-zinc-500"><span className={`h-1.5 w-1.5 rounded-full ${props.connected ? "bg-emerald-400" : "bg-red-400"}`} />{agentInfo?.hostname || "Computer"} · {props.connected ? "Connected" : "Disconnected"}</div></div>
      {layouts.length > 0 && <select aria-label="Display layout" value={currentLayout ?? ""} onChange={(event) => void switchLayout(event.target.value)} className="max-w-28 rounded-lg border border-white/10 bg-zinc-900 px-2 py-2 text-xs"><option value="" disabled>Layout</option>{layouts.map((layout) => <option key={layout.id} value={layout.id}>{layout.name}</option>)}</select>}
      {document.fullscreenEnabled && <button type="button" aria-label={fullscreen ? "Exit fullscreen" : "Enter fullscreen"} onClick={() => {
        setFullscreenError("");
        void (fullscreen ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => setFullscreenError("Fullscreen could not open. Try again from the fullscreen button."));
      }} className="rounded-lg border border-white/10 p-2 text-zinc-400 hover:text-white">{fullscreen ? <Minimize className="h-4 w-4" /> : <Maximize className="h-4 w-4" />}</button>}
    </header>
    {fullscreenError && <p role="alert" className="text-xs text-amber-300">{fullscreenError}</p>}
    <div className="remote-workspace">
      <div className="remote-pad"><Trackpad {...props} mode="remote" /></div>
      <aside className="remote-audio" aria-label="Remote audio">
        {!audioSinks.length && <p className="text-xs text-zinc-500">{audioError || "No PC audio output available"}</p>}
        {sink && <RemoteVolume key={sink.name} label="PC volume" value={Math.round(sink.volume * 100)} muted={sink.muted} onVolume={(value) => setSinkVolume(sink.name, value / 100)} onMute={() => void setSinkMute(sink.name, !sink.muted)} />}
        {tvs.map((tv) => <RemoteVolume key={tv.edid} label={tvs.length > 1 ? `${tv.friendly_name || tv.name} volume` : "TV volume"} value={tv.tv_state!.volume} muted={tv.tv_state!.muted} onVolume={(value) => setMonitorVolume(tv.edid, value)} onMute={() => void setMonitorMute(tv.edid, !tv.tv_state!.muted)} />)}
      </aside>
    </div>
  </main>;
}
