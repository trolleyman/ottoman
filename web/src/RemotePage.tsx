import { useCallback, useEffect, useImperativeHandle, useRef, useState, type Ref } from "react";
import { House, X, Maximize, Minimize, Monitor, ChevronRight, Volume2, Sun, Settings, LayoutGrid, Power, LogOut, ArrowLeft } from "lucide-react";
import { BuildVersions } from "./BuildVersions";
import { ComputerPower } from "./ComputerPower";
import { followAppLink } from "./navigation";
import { OttomanWithLogo } from "./OttomanWithLogo";
import { NowPlaying } from "./NowPlaying";
import { Trackpad } from "./Trackpad";
import { KeyboardSettingsPanel } from "./KeyboardSettingsPanel";
import { Monitors } from "./Monitors";
import { MonitorDisplay } from "./MonitorDisplay";
import { Audio } from "./Audio";
import { Layouts } from "./Layouts";
import { ClientStatus } from "./ClientStatus";
import { useMonitorPower } from "./useMonitorPower";
import { useStore } from "./store";
import { loadSettings, savePreference } from "./preferences";
import { useCoalescedSlider } from "./useCoalescedSlider";
import type { TrackpadMessage, TrackpadMessageTextContext } from "./api";
import type { TrackpadSettings } from "./trackpadSettings";

type VolumeControl = { step: (delta: number) => void };
function RemoteSlider({ label, value, onChange, brightness = false, controlRef, disabled = false }: {
  label: string; value: number; onChange: (value: number) => Promise<void>; brightness?: boolean; controlRef?: Ref<VolumeControl>; disabled?: boolean;
}) {
  const slider = useCoalescedSlider(value, onChange);
  useImperativeHandle(controlRef, () => ({ step: delta => { if (!disabled) slider.set(current => Math.min(100, Math.max(0, current + delta))); } }));
  return <label className="remote-menu-slider"><span>{brightness ? <Sun /> : <Volume2 />}<span>{brightness ? "Brightness" : "Volume"}</span><output>{value < 0 ? "—" : `${slider.value}%`}</output></span><input aria-label={label} type="range" min="0" max="100" value={slider.value} disabled={disabled || value < 0} {...slider.dragProps} onChange={e => slider.set(Number(e.target.value))} /></label>;
}

function TVPower({ edid, on }: { edid: string; on: boolean }) {
  const power = useMonitorPower(edid, on);
  return <button className="remote-tv-power" aria-label="TV power" aria-pressed={power.on} aria-busy={power.loading} disabled={power.loading} onClick={() => void power.toggle(!power.on)}><Power /></button>;
}

type Panel = "remote" | "settings" | "displays" | "audio" | "layouts" | "connection";
export function RemotePage(props: {
  textContext: TrackpadMessageTextContext | null; connected: boolean; connecting: boolean; cursorPos: { x: number; y: number } | null;
  cursorSupported: boolean; send: (message: TrackpadMessage) => void;
}) {
  const store = useStore();
  const [settings, setSettings] = useState(loadSettings);
  const changeSettings = useCallback((patch: Partial<TrackpadSettings>) => setSettings(current => ({ ...current, ...patch })), []);
  useEffect(() => savePreference("ottoman-trackpad-settings", settings), [settings]);
  const [panel, setPanel] = useState<Panel>("remote");
  const [wide, setWide] = useState(() => typeof matchMedia !== "undefined" && matchMedia("(min-width: 1024px)").matches);
  const [menuOpen, setMenuOpen] = useState(false);
  const [sidebarClosed, setSidebarClosed] = useState(false);
  const menu = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const [audioTarget, setAudioTarget] = useState("");
  const controls = useRef(new Map<string, VolumeControl>());
  const [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const [error, setError] = useState("");
  useEffect(() => {
    const query = matchMedia("(min-width: 1024px)");
    const resize = () => { setWide(query.matches); setMenuOpen(false); };
    query.addEventListener("change", resize);
    const changed = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", changed);
    return () => { query.removeEventListener("change", resize); document.removeEventListener("fullscreenchange", changed); };
  }, []);
  useEffect(() => {
    if (!menuOpen || wide) return;
    const previous = document.activeElement;
    const node = menu.current!;
    node.querySelector<HTMLElement>("a,button")?.focus();
    const keys = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); setMenuOpen(false); }
      if (event.key !== "Tab") return;
      const elements = Array.from(node.querySelectorAll<HTMLElement>('button:not(:disabled),a,input:not(:disabled),select:not(:disabled)')).filter(el => el.getClientRects().length);
      const first = elements[0], last = elements.at(-1);
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener("keydown", keys);
    return () => { document.removeEventListener("keydown", keys); if (previous instanceof HTMLElement && previous.isConnected) previous.focus(); };
  }, [menuOpen, wide]);
  const tvs = store.monitors.filter(item => item.tv_state?.paired);
  const sink = store.audioSinks.find(item => item.default) ?? store.audioSinks[0];
  const outputs = [...tvs.filter(tv => tv.capabilities?.volume).map(tv => tv.edid), ...(sink ? [sink.name] : [])];
  const selected = outputs.includes(audioTarget) ? audioTarget : outputs[0];
  const selectedRef = useRef(selected);
  useEffect(() => { selectedRef.current = selected; }, [selected]);
  const stepVolume = useCallback((delta: number) => { const id = selectedRef.current; if (id) controls.current.get(id)?.step(delta); }, [selectedRef]);
  const visible = wide ? !sidebarClosed : menuOpen;
  useEffect(() => { if (visible) menu.current?.scrollTo({ top: 0 }); }, [visible]);
  const connectionStatus = props.connected ? "Connected" : props.connecting ? "Connecting" : "Disconnected";
  const navigate = (next: Panel) => { setPanel(next); setMenuOpen(false); };
  return <main className={`pc-remote ${wide && !sidebarClosed ? "remote-has-sidebar" : ""}`}>
    <header className="remote-header">
      <button ref={menuButton} type="button" aria-label="Open menu" aria-expanded={visible} aria-controls="remote-menu" onClick={() => wide ? setSidebarClosed(value => !value) : setMenuOpen(value => !value)}><span className="remote-menu-icon" aria-hidden="true"><i /><i /><i /></span></button>
      <h1>{store.agentInfo?.hostname || "Computer"}</h1>
      {document.fullscreenEnabled && <button className="remote-fullscreen" aria-label={fullscreen ? "Exit fullscreen" : "Enter fullscreen"} onClick={() => { setError(""); void (fullscreen ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => setError("Fullscreen could not open.")); }}>{fullscreen ? <Minimize /> : <Maximize />}</button>}
      <span className="remote-connection" role="status" aria-label={connectionStatus} title={connectionStatus}><i data-connected={props.connected} aria-hidden="true" /></span>
    </header>
    <button className="remote-menu-scrim" data-open={menuOpen && !wide} aria-hidden={!menuOpen || wide} inert={!menuOpen || wide} tabIndex={-1} aria-label="Close menu" onClick={() => setMenuOpen(false)} />
    <aside ref={menu} id="remote-menu" className="remote-menu" data-open={visible} inert={!visible} aria-hidden={!visible} role={!wide ? "dialog" : undefined} aria-modal={!wide ? true : undefined} aria-label="Desktop controls">
      <header><a href="/" className="remote-brand" aria-label="Ottoman home" onClick={event => { followAppLink(event); if (event.defaultPrevented) navigate("remote"); }}><OttomanWithLogo /></a><button aria-label="Close menu" onClick={() => wide ? setSidebarClosed(true) : setMenuOpen(false)}><X /></button></header>
      {store.layouts.length > 0 && <label className="remote-layout"><span>Layout</span><select aria-label="Display layout" value={store.currentLayout ?? ""} disabled={store.switching} onChange={e => void store.switchLayout(e.target.value)}><option value="" disabled>Choose layout</option>{store.layouts.map(layout => <option key={layout.id} value={layout.id}>{layout.name}</option>)}</select></label>}
      {store.layoutNotice && <p role="status" className="remote-notice">{store.layoutNotice.text}</p>}
      <section><h2><button className="remote-output" aria-label="Select PC audio" aria-pressed={!!sink && selected === sink.name} disabled={!sink} onClick={() => sink && setAudioTarget(sink.name)}><Monitor />PC</button><span>{store.agentInfo?.hostname || "Computer"}</span><ComputerPower /></h2>{sink && <RemoteSlider key={sink.name} label="PC volume" value={Math.round(sink.volume * 100)} onChange={value => store.setSinkVolume(sink.name, value / 100)} disabled={!props.connected} controlRef={value => { if (value) controls.current.set(sink.name, value); else controls.current.delete(sink.name); }} />}<NowPlaying active={visible && props.connected} /></section>
      {tvs.map((tv, index) => <section key={tv.edid}><h2><button className="remote-output" aria-label={`Select ${tvs.length > 1 ? `TV ${index + 1}` : "TV"} audio`} aria-pressed={selected === tv.edid} onClick={() => setAudioTarget(tv.edid)}><Monitor />{tv.friendly_name || (tvs.length > 1 ? `TV ${index + 1}` : "TV")}</button>{tv.capabilities?.power && tv.visibility?.power !== false && <TVPower edid={tv.edid} on={tv.tv_state!.on} />}</h2>
        {tv.capabilities?.volume && tv.visibility?.volume !== false && <RemoteSlider label={`${tvs.length > 1 ? `TV ${index + 1}` : "TV"} volume`} value={tv.tv_state!.volume} onChange={value => store.setMonitorVolume(tv.edid, value)} controlRef={value => { if (value) controls.current.set(tv.edid, value); else controls.current.delete(tv.edid); }} />}
        {tv.capabilities?.brightness && tv.visibility?.brightness !== false && <RemoteSlider brightness label={`${tvs.length > 1 ? `TV ${index + 1}` : "TV"} brightness`} value={tv.brightness ?? -1} onChange={value => store.setMonitorBrightness(tv.edid, value)} />}
      </section>)}
      {store.monitors.filter(item => !item.tv_state && item.capabilities?.brightness && item.visibility?.brightness !== false).map(display => <section key={display.edid}><h2><Monitor />{display.friendly_name || display.name}</h2><RemoteSlider brightness label={`${display.name} brightness`} value={display.brightness ?? -1} onChange={value => store.setMonitorBrightness(display.edid, value)} disabled={!props.connected} /></section>)}
      <nav aria-label="Remote navigation">
        <a href="/" onClick={event => { followAppLink(event); if (event.defaultPrevented) navigate("remote"); }}><House />Home<ChevronRight /></a>
        <button onClick={() => navigate("displays")}><Monitor />Displays<ChevronRight /></button>
        <button onClick={() => navigate("audio")}><Volume2 />Audio outputs<ChevronRight /></button>
        <button onClick={() => navigate("layouts")}><LayoutGrid />Display layouts<ChevronRight /></button>
        <button aria-label="Keyboard settings" onClick={() => navigate("settings")}><Settings />Settings<ChevronRight /></button>
        <button onClick={() => navigate("connection")}><Power />Connection & power<ChevronRight /></button>
        <button onClick={() => void store.logout()}><LogOut />Log out</button>
      </nav>
      {(store.monitorsError || store.audioError) && <p className="remote-notice" role="alert">{store.monitorsError || store.audioError}</p>}
      <BuildVersions active={visible} agent={store.agentInfo?.version} />
    </aside>
    <div className="remote-content" inert={menuOpen && !wide ? true : undefined}>
      {error && <p role="alert" className="remote-notice">{error}</p>}
      <div className="remote-workspace" hidden={panel !== "remote"}><div className="remote-pad"><Trackpad {...props} inputEnabled={panel === "remote" && (!menuOpen || wide)} settings={settings} onVolumeStep={selected ? stepVolume : undefined} /></div></div>
      {panel === "settings" && <KeyboardSettingsPanel settings={settings} onSettingsChange={changeSettings} onBack={() => setPanel("remote")} />}
      {panel !== "remote" && panel !== "settings" && <div className="remote-management"><button className="remote-back" onClick={() => setPanel("remote")}><ArrowLeft />PC remote</button>{panel === "displays" ? <><MonitorDisplay layouts={store.layouts} currentLayout={store.currentLayout ?? ""} loading={store.layoutsLoading} cursorPos={props.cursorPos} connected={props.connected} onSetPosition={props.connected && props.cursorSupported ? (x, y) => props.send({ type: "mousemoveto", x, y }) : undefined} /><Monitors /></> : panel === "audio" ? <Audio /> : panel === "layouts" ? <Layouts /> : <ClientStatus />}</div>}
    </div>
  </main>;
}
