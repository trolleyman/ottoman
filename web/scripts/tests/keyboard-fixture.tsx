import React, { useState, useRef, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { RemotePage } from "../../src/RemotePage";
import { CancelablePromise } from "../../src/api/core/CancelablePromise";
import { client, useStore } from "../../src/store";
import { DEFAULT_SETTINGS } from "../../src/trackpadSettings";
import { codePointOffset } from "../../src/remoteText";
import type { TrackpadMessage, TrackpadMessageTextContext } from "../../src/api";
import "../../src/index.css";

const initial: TrackpadMessageTextContext = { type: "textcontext", fieldId: "fixture", revision: 1, value: "hello world", selectionStart: 11, selectionEnd: 11, windowStart: 0, available: true, editable: true, multiline: true, password: false, protected: false };
const volumeWrites: { target: string; value: number }[] = [];
const writeVolume = async (target: string, value: number, update: () => void) => {
  volumeWrites.push({ target, value });
  await new Promise(resolve => setTimeout(resolve, 100));
  update();
};
useStore.setState({
  monitors: [{ edid: "fixture-screen", port: "DP-1", name: "Screen", manufacturer: "Fixture", brightness: 55, capabilities: { brightness: true, power: true, volume: false } }],
  audioSinks: [{ id: 1, name: "fixture-audio", description: "Audio", default: true, volume: 0.4, muted: false }],
  setMonitorBrightness: (edid, brightness) => { useStore.setState(state => ({ monitors: state.monitors.map(item => item.edid === edid ? { ...item, brightness } : item) })); return Promise.resolve(); },
});
// Exercise the production store actions, including a poll with the old value
// arriving after an optimistic write but before the device has applied it.
client.default.setAudioVolume = ({ name, volume } = { name: "" }) => new CancelablePromise((resolve, reject) => {
  void (async () => {
    const old = useStore.getState().audioSinks.find(item => item.name === name)?.volume;
    setTimeout(() => useStore.setState(state => ({ audioSinks: state.audioSinks.map(item => item.name === name ? { ...item, volume: old ?? item.volume } : item) })), 30);
    await writeVolume(name, volume!, () => useStore.setState(state => ({ audioSinks: state.audioSinks.map(item => item.name === name ? { ...item, volume: volume! } : item) })));
    resolve({ success: true });
  })().catch(reject);
});
client.default.setMonitorVolume = ({ edid, volume } = { edid: "" }) => new CancelablePromise((resolve, reject) => {
  void (async () => {
    const old = useStore.getState().monitors.find(item => item.edid === edid)?.tv_state?.volume;
    setTimeout(() => useStore.setState(state => ({ monitors: state.monitors.map(item => item.edid === edid && item.tv_state ? { ...item, tv_state: { ...item.tv_state, volume: old ?? item.tv_state.volume } } : item) })), 30);
    await writeVolume(edid, volume!, () => useStore.setState(state => ({ monitors: state.monitors.map(item => item.edid === edid && item.tv_state ? { ...item, tv_state: { ...item.tv_state, volume: volume! } } : item) })));
    resolve({ success: true });
  })().catch(reject);
});
export function Fixture() {
  const log = useRef<TrackpadMessage[]>([]);
  const [epoch, setEpoch] = useState(0);
  const [context, setContext] = useState(initial);
  const send = React.useCallback((message: TrackpadMessage) => {
    log.current.push(message);
    if (message.type !== "textedit") return;
    setContext(current => {
      const chars = Array.from(current.value);
      const value = message.selectionOnly ? current.value : chars.slice(0, message.start).join("") + message.text + chars.slice(message.end).join("");
      return { ...current, value, revision: current.revision + 1, requestId: message.requestId, selectionStart: message.selectionOnly ? message.start : message.start + Array.from(message.text).length, selectionEnd: message.selectionOnly ? message.end : message.start + Array.from(message.text).length };
    });
  }, []);
  useEffect(() => { Object.assign(window, { fixture: {
    log: log.current, context,
    setField(patch: Partial<TrackpadMessageTextContext>) { log.current.length = 0; setContext(current => ({ ...initial, ...patch, revision: current.revision + 1 })); },
    setOS(os: string) { useStore.setState({ agentInfo: { os, status: "online", version: "fixture", uptime: "0", hostname: "Fixture", ip_address: "127.0.0.1", port: "0" } }); },
    volumeWrites,
    addTV() { useStore.setState(state => ({ monitors: [...state.monitors, { edid: "fixture-tv", port: "HDMI-1", name: "TV", manufacturer: "Fixture", capabilities: { brightness: false, power: true, volume: true }, tv_state: { paired: true, pairing: false, on: true, volume: 12, muted: false } }] })); },
    removeTV() { useStore.setState(state => ({ monitors: state.monitors.filter(item => item.edid !== "fixture-tv") })); },
    brightness: () => useStore.getState().monitors[0].brightness,
    reset(value: string, cursor = value.length) { log.current.length = 0; setContext(current => ({ ...initial, revision: current.revision + 1, value, selectionStart: codePointOffset(value, cursor), selectionEnd: codePointOffset(value, cursor) })); },
    configure(patch: Partial<typeof DEFAULT_SETTINGS>) { localStorage.setItem("ottoman-trackpad-settings", JSON.stringify({ ...DEFAULT_SETTINGS, mirrorFocusedText: true, swipeTyping: false, ...patch })); setEpoch(value => value + 1); },
  } }); }, [context]);
  return <RemotePage key={epoch} connected connecting={false} textContext={context} cursorPos={null} cursorSupported={false} send={send} />;
}

createRoot(document.getElementById("root")!).render(<Fixture />);
