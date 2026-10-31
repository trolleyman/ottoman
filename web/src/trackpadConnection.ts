import type { TrackpadMessage } from "./api";

export interface ConnectionState {
  connected: boolean;
  connecting: boolean;
}

// One owner for the socket and its retry timer. Disposal detaches handlers
// before closing, so a late close event cannot resurrect an old session.
export function createTrackpadConnection(
  url: string,
  onState: (state: ConnectionState) => void,
  onMessage: (message: TrackpadMessage) => void,
) {
  let socket: WebSocket | null = null;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let disposed = false;
  let connected = false;

  const closeSocket = () => {
    if (!socket) return;
    socket.onclose = null;
    socket.onmessage = null;
    socket.close();
    socket = null;
  };

  const connect = () => {
    if (disposed || connected) return;
    clearTimeout(retry);
    closeSocket();
    onState({ connected: false, connecting: true });
    const current = new WebSocket(url);
    socket = current;
    current.onclose = () => {
      if (disposed || socket !== current) return;
      connected = false;
      onState({ connected: false, connecting: false });
      retry = setTimeout(connect, 3000);
    };
    current.onmessage = (event) => {
      if (disposed || socket !== current) return;
      let message: TrackpadMessage;
      try {
        message = JSON.parse(event.data as string) as TrackpadMessage;
        if (!message || typeof message !== "object") return;
      } catch {
        return;
      }
      // An agent message, rather than just the proxy handshake, proves it is live.
      connected = true;
      onState({ connected: true, connecting: false });
      onMessage(message);
    };
  };

  // Schedule the initial connection so creating the owner has no synchronous
  // React state updates. Cleanup also cancels this during StrictMode's probe.
  retry = setTimeout(connect, 0);
  return {
    reconnect: connect,
    send(message: TrackpadMessage) {
      if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify(message));
    },
    dispose() {
      disposed = true;
      clearTimeout(retry);
      closeSocket();
    },
  };
}
