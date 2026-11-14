import React, { useState, useRef, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { RemotePage } from "../../src/RemotePage";
import { useStore } from "../../src/store";
import { DEFAULT_SETTINGS } from "../../src/trackpadSettings";
import { codePointOffset } from "../../src/remoteText";
import type { TrackpadMessage, TrackpadMessageTextContext } from "../../src/api";
import "../../src/index.css";

const initial: TrackpadMessageTextContext = { type: "textcontext", fieldId: "fixture", revision: 1, value: "hello world", selectionStart: 11, selectionEnd: 11, windowStart: 0, available: true, editable: true, multiline: true, password: false, protected: false };
useStore.setState({
  monitors: [{ edid: "fixture-screen", port: "DP-1", name: "Screen", manufacturer: "Fixture", brightness: 55, capabilities: { brightness: true, power: true, volume: false } }],
  audioSinks: [{ id: 1, name: "fixture-audio", description: "Audio", default: true, volume: 0.4, muted: false }],
  setMonitorBrightness: (edid, brightness) => { useStore.setState(state => ({ monitors: state.monitors.map(item => item.edid === edid ? { ...item, brightness } : item) })); return Promise.resolve(); },
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
    brightness: () => useStore.getState().monitors[0].brightness,
    reset(value: string, cursor = value.length) { log.current.length = 0; setContext(current => ({ ...initial, revision: current.revision + 1, value, selectionStart: codePointOffset(value, cursor), selectionEnd: codePointOffset(value, cursor) })); },
    configure(patch: Partial<typeof DEFAULT_SETTINGS>) { localStorage.setItem("ottoman-trackpad-settings", JSON.stringify({ ...DEFAULT_SETTINGS, mirrorFocusedText: true, swipeTyping: false, ...patch })); setEpoch(value => value + 1); },
  } }); }, [context]);
  return <RemotePage key={epoch} connected connecting={false} textContext={context} cursorPos={null} cursorSupported={false} send={send} />;
}

createRoot(document.getElementById("root")!).render(<Fixture />);
