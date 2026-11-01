import { useEffect, useState } from "react";
import { ArrowLeft, Maximize, Minimize, Monitor, ChevronDown, Volume2, VolumeX } from "lucide-react";
import { Trackpad } from "./Trackpad";
import { useStore } from "./store";
import { useCoalescedSlider } from "./useCoalescedSlider";
import type { TrackpadMessage, TrackpadMessageTextContext } from "./api";

function RemoteVolume({ label, value, muted, onVolume, onMute, onTarget }: {
  onTarget?: () => void; label: string; value: number; muted: boolean; onVolume: (value: number) => Promise<void>; onMute: () => void;
}) {
  const slider = useCoalescedSlider(value, onVolume);
  return <aside className="remote-volume flex w-12 shrink-0 flex-col items-center gap-2 rounded-xl border border-white/8 bg-white/3 py-2" aria-label="Remote audio">
    <button type="button" onClick={onTarget} disabled={!onTarget} aria-label={`Audio output: ${label}. Switch output`} className="text-xs font-medium text-zinc-300 disabled:cursor-default">{label}</button>
    <span className="text-[10px] tabular-nums text-zinc-500">{slider.value}%</span>
    <input type="range" aria-label={`${label} volume`} min={0} max={100} value={slider.value} {...slider.dragProps} onChange={(event) => slider.set(Number(event.target.value))} className="remote-volume-slider min-h-0 flex-1 accent-blue-400" />
    <button type="button" aria-label={`${muted ? "Unmute" : "Mute"} ${label}`} onClick={onMute} className="p-2 text-zinc-400 hover:text-white">{muted ? <VolumeX className="h-4 w-4" /> : <Volume2 className="h-4 w-4" />}</button>
  </aside>;
}

export function RemotePage(props: {
  textContext: TrackpadMessageTextContext | null;
  connected: boolean; connecting: boolean; cursorPos: { x: number; y: number } | null;
  cursorSupported: boolean; send: (message: TrackpadMessage) => void;
}) {
  const { agentInfo, layouts, currentLayout, switchLayout, audioSinks, monitors, setSinkVolume, setSinkMute, setMonitorVolume, setMonitorMute } = useStore();
  const [settingsContainer, setSettingsContainer] = useState<HTMLDivElement | null>(null);
  const [audioTarget, setAudioTarget] = useState("");
  const [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const [showFullscreenHelp, setShowFullscreenHelp] = useState(false);
  const [fullscreenError, setFullscreenError] = useState("");
  useEffect(() => {
    const changed = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", changed);
    return () => document.removeEventListener("fullscreenchange", changed);
  }, []);
  const sink = audioSinks.find((item) => item.default);
  const tvs = monitors.filter((item) => item.capabilities?.volume && item.tv_state?.paired);
  const outputs = [
    ...tvs.map((tv, index) => ({ id: tv.edid, label: tvs.length > 1 ? `TV ${index + 1}` : "TV", value: tv.tv_state!.volume, muted: tv.tv_state!.muted, onVolume: (value: number) => setMonitorVolume(tv.edid, value), onMute: () => void setMonitorMute(tv.edid, !tv.tv_state!.muted) })),
    ...(sink ? [{ id: sink.name, label: "PC", value: Math.round(sink.volume * 100), muted: sink.muted, onVolume: (value: number) => setSinkVolume(sink.name, value / 100), onMute: () => void setSinkMute(sink.name, !sink.muted) }] : []),
  ];
  const outputIndex = Math.max(0, outputs.findIndex((item) => item.id === audioTarget));
  const output = outputs[outputIndex];
  return <main className="pc-remote bg-zinc-950 text-zinc-100">
    <header className="flex shrink-0 items-center gap-3 border-b border-white/8 pb-3">
      <a href="/" aria-label="Back to dashboard" className="rounded-lg border border-white/10 p-2 text-zinc-400 hover:text-white"><ArrowLeft className="h-4 w-4" /></a>
      <div className="min-w-0 flex-1"><div className="flex items-center gap-2 text-sm font-semibold"><Monitor className="h-4 w-4 text-blue-400" />PC remote</div><div className="mt-0.5 flex items-center gap-1.5 text-[11px] text-zinc-500"><span className={`h-1.5 w-1.5 rounded-full ${props.connected ? "bg-emerald-400" : "bg-red-400"}`} />{agentInfo?.hostname || "Computer"} · {props.connected ? "Connected" : "Disconnected"}</div></div>
      {layouts.length > 0 && <div className="relative"><select aria-label="Display layout" value={currentLayout ?? ""} onChange={(event) => void switchLayout(event.target.value)} className="max-w-28 appearance-none rounded-lg border border-white/10 bg-zinc-900 py-2 pl-2 pr-7 text-xs text-zinc-100"><option value="" disabled>Layout</option>{layouts.map((layout) => <option key={layout.id} value={layout.id}>{layout.name}</option>)}</select><ChevronDown aria-hidden="true" className="pointer-events-none absolute right-2 top-1/2 h-3 w-3 -translate-y-1/2 text-zinc-300" /></div>}
      <button type="button" aria-label={!document.fullscreenEnabled ? "Open without address bar" : fullscreen ? "Exit fullscreen" : "Enter fullscreen"} onClick={() => {
        if (!document.fullscreenEnabled) { setShowFullscreenHelp((open) => !open); return; }
        setFullscreenError("");
        void (fullscreen ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => setFullscreenError("Fullscreen could not open. Try again from the fullscreen button."));
      }} className="rounded-lg border border-white/10 p-2 text-zinc-400 hover:text-white">{fullscreen ? <Minimize className="h-4 w-4" /> : <Maximize className="h-4 w-4" />}</button>
      <div ref={setSettingsContainer} />
    </header>
    {showFullscreenHelp && <p role="status" className="shrink-0 text-xs text-zinc-300">To open without the address bar on iPhone, use Safari’s Share menu → Add to Home Screen. Enable Open as Web App if shown, then launch PC remote from your Home Screen.</p>}
    {fullscreenError && <p role="alert" className="text-xs text-amber-300">{fullscreenError}</p>}
    <div className="remote-workspace">
      <div className="remote-pad"><Trackpad {...props} mode="remote" settingsContainer={settingsContainer}
        sideControls={output && <RemoteVolume key={output.id} {...output} onTarget={outputs.length > 1 ? () => setAudioTarget(outputs[(outputIndex + 1) % outputs.length].id) : undefined} />} /></div>
    </div>
  </main>;
}
