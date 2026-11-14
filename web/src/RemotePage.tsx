import { useCallback, useEffect, useImperativeHandle, useRef, useState, type Ref } from "react";
import { ArrowLeft, Maximize, Minimize, Monitor, ChevronDown, Volume2, Sun } from "lucide-react";
import { followAppLink } from "./navigation";
import { OfflineTVControls } from "./Monitors";
import { Trackpad } from "./Trackpad";
import { useStore } from "./store";
import { useCoalescedSlider } from "./useCoalescedSlider";
import type { TrackpadMessage, TrackpadMessageTextContext } from "./api";

type VolumeControl = { step: (delta: number) => void };

function RemoteVolume({ label, value, onVolume, onTarget, brightness = false, onMode, controlRef, hidden = false }: {
  controlRef?: Ref<VolumeControl>; hidden?: boolean;
  brightness?: boolean; onMode?: () => void;
  onTarget?: () => void; label: string; value: number; onVolume: (value: number) => Promise<void>;
}) {
  const slider = useCoalescedSlider(value, onVolume);
  // Keep audio mounted while brightness is visible so keys share its optimistic
  // value and coalesced writes, even with a slow TV or stale status polls.
  useImperativeHandle(controlRef, () => ({
    step: delta => slider.set(current => Math.min(100, Math.max(0, current + delta))),
  }));
  return <aside hidden={hidden} className={`${hidden ? "hidden" : "flex"} remote-volume w-12 shrink-0 flex-col items-center gap-2 rounded-xl border border-white/8 bg-white/3 py-2`} aria-label={brightness ? "Remote brightness" : "Remote audio"}>
    <button type="button" onClick={onTarget} disabled={!onTarget} aria-label={`${brightness ? "Display" : "Audio output"}: ${label}. Switch output`} className="text-xs font-medium text-zinc-300 disabled:cursor-default">{label}</button>
    <span className="text-[10px] tabular-nums text-zinc-500">{slider.value}%</span>
    <input type="range" aria-label={`${label} ${brightness ? "brightness" : "volume"}`} min={0} max={100} value={slider.value} {...slider.dragProps} onChange={(event) => slider.set(Number(event.target.value))} className="remote-volume-slider min-h-0 flex-1 accent-blue-400" />
    {onMode && <button type="button" onClick={onMode} aria-label={brightness ? "Switch to volume" : "Switch to brightness"} className="p-2 text-zinc-300 hover:text-white">{brightness ? <Sun className="h-4 w-4" /> : <Volume2 className="h-4 w-4" />}</button>}
  </aside>;
}

export function RemotePage(props: {
  textContext: TrackpadMessageTextContext | null;
  connected: boolean; connecting: boolean; cursorPos: { x: number; y: number } | null;
  cursorSupported: boolean; send: (message: TrackpadMessage) => void;
}) {
  const { agentInfo, layouts, currentLayout, switchLayout, audioSinks, monitors, setSinkVolume, setMonitorVolume, setMonitorBrightness } = useStore();
  const volumeControl = useRef<VolumeControl>(null);
  const stepVolume = useCallback((delta: number) => volumeControl.current?.step(delta), []);
  const [settingsContainer, setSettingsContainer] = useState<HTMLDivElement | null>(null);
  const [brightnessMode, setBrightnessMode] = useState(false);
  const [brightnessTarget, setBrightnessTarget] = useState("");
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
    ...tvs.map((tv, index) => ({ id: tv.edid, label: tvs.length > 1 ? `TV ${index + 1}` : "TV", value: tv.tv_state!.volume, onVolume: (value: number) => setMonitorVolume(tv.edid, value) })),
    ...(sink ? [{ id: sink.name, label: "PC", value: Math.round(sink.volume * 100), onVolume: (value: number) => setSinkVolume(sink.name, value / 100) }] : []),
  ];
  const outputIndex = Math.max(0, outputs.findIndex((item) => item.id === audioTarget));
  const audioOutput = outputs[outputIndex];
  const displays = monitors.filter(item => item.capabilities?.brightness);
  const displayIndex = Math.max(0, displays.findIndex(item => item.edid === brightnessTarget));
  const display = displays[displayIndex];
  const isBrightness = !!display && (brightnessMode || !audioOutput);
  const output = isBrightness ? { id: display.edid + "-brightness", label: displays.length > 1 ? `D${displayIndex + 1}` : "Screen", value: display.brightness != null && display.brightness >= 0 ? Math.min(100, display.brightness) : 50, onVolume: (value: number) => setMonitorBrightness(display.edid, value) } : audioOutput;
  return <main className="pc-remote bg-zinc-950 text-zinc-100">
    <header className="flex shrink-0 items-center gap-3 border-b border-white/8 pb-3">
      <a href="/" onClick={followAppLink} aria-label="Back to dashboard" className="rounded-lg border border-white/10 p-2 text-zinc-400 hover:text-white"><ArrowLeft className="h-4 w-4" /></a>
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
    {!props.connected ? <OfflineTVControls /> : <div className="remote-workspace">
      <div className="remote-pad"><Trackpad {...props} mode="remote" settingsContainer={settingsContainer} onVolumeStep={audioOutput ? stepVolume : undefined}
        sideControls={<>{audioOutput && <RemoteVolume key={audioOutput.id} {...audioOutput} controlRef={volumeControl} hidden={isBrightness}
          onMode={display ? () => setBrightnessMode(true) : undefined}
          onTarget={outputs.length > 1 ? () => setAudioTarget(outputs[(outputIndex + 1) % outputs.length].id) : undefined} />}
        {isBrightness && output && <RemoteVolume key={output.id} {...output} brightness onMode={audioOutput ? () => setBrightnessMode(false) : undefined}
          onTarget={displays.length > 1 ? () => setBrightnessTarget(displays[(displayIndex + 1) % displays.length].edid) : undefined} />}</>} /></div>
    </div>}
  </main>;
}
