import { useState, useEffect, useCallback, useRef } from "react";
import type { TrackpadMessage, TrackpadMessageTextContext } from "./api";
import { createTrackpadConnection, type ConnectionState } from "./trackpadConnection";

export function useTrackpadWebSocket(authed: boolean, refreshKey: number) {
  const [textContext, setTextContext] = useState<TrackpadMessageTextContext | null>(null);
  const [state, setState] = useState<ConnectionState>({ connected: false, connecting: false });
  const [cursorPos, setCursorPos] = useState<{ x: number; y: number } | null>(null);
  // Wayland's uinput backend injects input but cannot query cursor position.
  const [cursorSupported, setCursorSupported] = useState(true);
  const connectionRef = useRef<ReturnType<typeof createTrackpadConnection> | null>(null);

  useEffect(() => {
    if (!authed) return;
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const connection = createTrackpadConnection(
      `${protocol}//${window.location.host}/api/trackpad`,
      (next) => {
        setState(next);
        if (!next.connected) { setCursorPos(null); setTextContext(null); }
      },
      (message) => {
        if (message.type === "textcontext") {
          setTextContext(message);
        } else if (message.type === "mousepositionupdate") {
          setCursorSupported(true);
          setCursorPos({ x: message.x ?? 0, y: message.y ?? 0 });
        } else if (message.type === "connected") {
          setCursorSupported(false);
          setCursorPos(null);
        }
      },
    );
    connectionRef.current = connection;
    return () => {
      connection.dispose();
      setTextContext(null);
      connectionRef.current = null;
    };
  }, [authed]);

  useEffect(() => {
    connectionRef.current?.reconnect();
  }, [refreshKey]);

  const send = useCallback((message: TrackpadMessage) => {
    if (message.type === "textwatch") setTextContext(null);
    connectionRef.current?.send(message);
  }, []);

  return {
    textContext: authed ? textContext : null,
    connected: authed && state.connected,
    connecting: authed && state.connecting,
    cursorPos: authed ? cursorPos : null,
    cursorSupported,
    send,
  };
}
