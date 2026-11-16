import { afterEach, beforeEach, describe, expect, jest, mock, test } from "bun:test";
import { createTrackpadConnection } from "../src/trackpadConnection";

class FakeWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static instances: FakeWebSocket[] = [];
  readyState = 1;
  onclose: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  send = mock((data: string) => { void data; });
  close = mock(() => { this.readyState = 3; });
  constructor(readonly url: string) { FakeWebSocket.instances.push(this); }
  message(data: string) { this.onmessage?.({ data }); }
}

const OriginalWebSocket = globalThis.WebSocket;
const originalDocument = Object.getOwnPropertyDescriptor(globalThis, "document");
const originalWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
let page: EventTarget & { visibilityState: string };
let browser: EventTarget;
function hide() {
  page.visibilityState = "hidden";
  page.dispatchEvent(new Event("visibilitychange"));
}
function show() {
  page.visibilityState = "visible";
  page.dispatchEvent(new Event("visibilitychange"));
}
beforeEach(() => {
  jest.useFakeTimers();
  page = Object.assign(new EventTarget(), { visibilityState: "visible" });
  browser = new EventTarget();
  Object.defineProperty(globalThis, "document", { configurable: true, value: page });
  Object.defineProperty(globalThis, "window", { configurable: true, value: browser });
  FakeWebSocket.instances = [];
  globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket;
});
afterEach(() => {
  jest.useRealTimers();
  if (originalDocument) Object.defineProperty(globalThis, "document", originalDocument);
  else Reflect.deleteProperty(globalThis, "document");
  if (originalWindow) Object.defineProperty(globalThis, "window", originalWindow);
  else Reflect.deleteProperty(globalThis, "window");
  globalThis.WebSocket = OriginalWebSocket;
});

function setup() {
  const state = mock(() => {});
  const message = mock(() => {});
  const connection = createTrackpadConnection("ws://localhost/api/trackpad", state, message);
  jest.advanceTimersByTime(0);
  return { connection, state, message, socket: FakeWebSocket.instances[0] };
}

describe("trackpad connection lifecycle", () => {
  test("requires an agent message, ignores malformed data, and sends only on an open socket", () => {
    const { connection, state, message, socket } = setup();
    expect(state).toHaveBeenLastCalledWith({ connected: false, connecting: true });
    socket.message("{");
    socket.message("null");
    expect(message).not.toHaveBeenCalled();
    socket.message('{"type":"connected"}');
    expect(state).toHaveBeenLastCalledWith({ connected: true, connecting: false });
    connection.send({ type: "text", text: "hello" });
    expect(socket.send).toHaveBeenCalledWith('{"type":"text","text":"hello"}');
    socket.readyState = 3;
    connection.send({ type: "text", text: "closed" });
    expect(socket.send).toHaveBeenCalledTimes(1);
    connection.dispose();
  });

  test("disposal cancels pending retries and ignores already queued close events", () => {
    const { connection, socket, state } = setup();
    const queuedClose = socket.onclose!;
    queuedClose();
    connection.dispose();
    const calls = state.mock.calls.length;
    queuedClose();
    jest.advanceTimersByTime(10000);
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(state).toHaveBeenCalledTimes(calls);
    expect(socket.onclose).toBeNull();
    expect(socket.onmessage).toBeNull();
  });

  test("manual reconnect replaces a pending retry without duplicating sockets", () => {
    const { connection, socket } = setup();
    socket.onclose!();
    connection.reconnect();
    const replacement = FakeWebSocket.instances[1];
    replacement.message('{"type":"connected"}');
    connection.reconnect();
    jest.advanceTimersByTime(10000);
    expect(FakeWebSocket.instances).toHaveLength(2);
    expect(socket.close).toHaveBeenCalledTimes(1);
    connection.dispose();
  });

  test("retries a dropped connection after the delay", () => {
    const { connection, socket, state } = setup();
    socket.message('{"type":"connected"}');
    socket.onclose!();
    expect(state).toHaveBeenLastCalledWith({ connected: false, connecting: false });
    jest.advanceTimersByTime(2999);
    expect(FakeWebSocket.instances).toHaveLength(1);
    jest.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(2);
    connection.dispose();
  });

  test("returning to the page immediately replaces a delayed retry", () => {
    const { connection, socket, state } = setup();
    hide();
    socket.onclose!();
    show();
    expect(FakeWebSocket.instances).toHaveLength(2);
    expect(state).toHaveBeenLastCalledWith({ connected: false, connecting: true });
    browser.dispatchEvent(new Event("online"));
    FakeWebSocket.instances[1].readyState = FakeWebSocket.CONNECTING;
    browser.dispatchEvent(new Event("online"));
    jest.advanceTimersByTime(3000);
    expect(FakeWebSocket.instances).toHaveLength(2);
    connection.dispose();
  });

  test("resume replaces an apparently live socket and ignores its queued events", () => {
    const { connection, socket, state } = setup();
    socket.message('{"type":"connected"}');
    const queuedClose = socket.onclose!;
    hide();
    show();
    expect(socket.close).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.instances).toHaveLength(2);
    queuedClose();
    expect(state).toHaveBeenLastCalledWith({ connected: false, connecting: true });
    connection.dispose();
  });

  test("restoring from the back-forward cache reconnects immediately", () => {
    const { connection, socket } = setup();
    socket.message('{"type":"connected"}');
    browser.dispatchEvent(new Event("pagehide"));
    browser.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: true }));
    expect(FakeWebSocket.instances).toHaveLength(2);
    connection.dispose();
    hide();
    show();
    browser.dispatchEvent(new Event("online"));
    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  test("disposal before startup prevents the first socket from opening", () => {
    const connection = createTrackpadConnection("ws://localhost/api/trackpad", () => {}, () => {});
    connection.dispose();
    jest.advanceTimersByTime(10000);
    expect(FakeWebSocket.instances).toHaveLength(0);
  });
});
