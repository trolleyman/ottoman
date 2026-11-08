import React, { useState, useRef, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { Trackpad } from "../../src/Trackpad";
import { DEFAULT_SETTINGS } from "../../src/trackpadSettings";
import { codePointOffset } from "../../src/remoteText";
import type { TrackpadMessage, TrackpadMessageTextContext } from "../../src/api";
import "../../src/index.css";

const initial: TrackpadMessageTextContext = { type: "textcontext", fieldId: "fixture", revision: 1, value: "hello world", selectionStart: 11, selectionEnd: 11, windowStart: 0, available: true, editable: true, multiline: true, password: false, protected: false };
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
    reset(value: string, cursor = value.length) { log.current.length = 0; setContext(current => ({ ...initial, revision: current.revision + 1, value, selectionStart: codePointOffset(value, cursor), selectionEnd: codePointOffset(value, cursor) })); },
    configure(patch: Partial<typeof DEFAULT_SETTINGS>) { localStorage.setItem("ottoman-trackpad-settings", JSON.stringify({ ...DEFAULT_SETTINGS, mirrorFocusedText: true, swipeTyping: false, ...patch })); setEpoch(value => value + 1); },
  } }); }, [context]);
  return <div className="pc-remote bg-zinc-950 p-2 text-white"><div className="remote-workspace"><div className="remote-pad"><Trackpad key={epoch} mode="remote" connected connecting={false} textContext={context} cursorPos={null} cursorSupported={false} send={send} />
  </div></div></div>;
}

createRoot(document.getElementById("root")!).render(<Fixture />);
