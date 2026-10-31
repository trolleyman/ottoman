import { useState, useEffect, useCallback, useRef } from "react";
import { type TrackpadMessage, type TrackpadMessageTextContext } from "./api";

export function useTrackpadWebSocket(authed: boolean, refreshKey: number) {
  const [textContext, setTextContext] = useState<TrackpadMessageTextContext | null>(null);
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [cursorPos, setCursorPos] = useState<{ x: number; y: number } | null>(null);
  // Whether the agent can read/set the absolute cursor position. The uinput
  // backend (Wayland) can inject input but can't query the cursor, so it sends
  // a "connected" message instead of position updates; hide cursor UI then.
  const [cursorSupported, setCursorSupported] = useState(true);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const connectRef = useRef<() => void>(() => {});

  const connect = useCallback(() => {
    if (!authed) {
      setTextContext(null);
      setConnected(false);
      setConnecting(false);
      return;
    }

    if (reconnectRef.current) {
      clearTimeout(reconnectRef.current);
      reconnectRef.current = null;
    }
    if (wsRef.current) {
      wsRef.current.onclose = null;
      wsRef.current.close();
    }

    setTextContext(null);
    setConnecting(true);

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/trackpad`);
    wsRef.current = ws;

    ws.onclose = () => {
      setTextContext(null);
      setConnected(false);
      setConnecting(false);
      setCursorPos(null);
      // Attempt reconnect after 3 seconds
      reconnectRef.current = setTimeout(() => connectRef.current(), 3000);
    };
    ws.onmessage = (e) => {
      if (wsRef.current !== ws) return;
      // First data message from client proves connection is live
      setConnected(true);
      setConnecting(false);
      try {
        const msg = JSON.parse(e.data as string) as TrackpadMessage;
        if (msg.type === "textcontext") {
          setTextContext(msg);
        } else if (msg.type === "mousepositionupdate") {
          setCursorSupported(true);
          setCursorPos({ x: msg.x ?? 0, y: msg.y ?? 0 });
        } else if (msg.type === "connected") {
          // Agent can't read the cursor position (e.g. uinput on Wayland).
          setCursorSupported(false);
          setCursorPos(null);
        }
      } catch { /* ignore parse errors */ }
    };
  }, [authed]);

  useEffect(() => {
    connectRef.current = connect;
  }, [connect]);

  useEffect(() => {
    reconnectRef.current = setTimeout(() => connectRef.current(), 0);
    return () => {
      if (wsRef.current) { wsRef.current.onclose = null; wsRef.current.onmessage = null; }
      wsRef.current?.close();
      if (reconnectRef.current) clearTimeout(reconnectRef.current);
    };
  }, [connect]);

  useEffect(() => {
    if (!connected && authed) {
      reconnectRef.current = setTimeout(() => connectRef.current(), 0);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshKey]);

  const send = useCallback((msg: TrackpadMessage) => {
    if (msg.type === "textwatch") setTextContext(null);
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  return { textContext, connected, connecting, cursorPos, cursorSupported, send };
}
