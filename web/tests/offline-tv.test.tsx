import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { RemotePage } from "../src/RemotePage";
import { useStore } from "../src/store";

test("remote shows TV controls while the PC is disconnected or connecting", () => {
  const previousDocument = Object.getOwnPropertyDescriptor(globalThis, "document");
  const initialState = useStore.getInitialState();
  const previousState = { ...initialState };
  Object.defineProperty(globalThis, "document", { configurable: true, value: { fullscreenElement: null } });
  try {
    Object.assign(initialState, { monitorsLoading: false, monitors: [{ edid: "tv", name: "Living room", control_backend: "tv", brightness: 45, capabilities: { power: true, volume: true, brightness: true }, tv_state: { paired: true, pairing: false, on: true, volume: 25, muted: false } }] });
    for (const connecting of [false, true]) {
      const html = renderToStaticMarkup(<RemotePage connected={false} connecting={connecting} textContext={null} cursorPos={null} cursorSupported={false} send={() => {}} />);
      expect(html).toContain('aria-label="TV controls"');
      expect(html).toContain("Living room");
      expect(html).toContain('aria-label="Volume"');
      expect(html).toContain('aria-label="Mute"');
      expect(html).toContain("Brightness");
      expect(html).not.toContain("Monitor settings");
      expect(html).not.toContain("remote-workspace");
    }
    Object.assign(initialState, { monitors: [] });
    const empty = renderToStaticMarkup(<RemotePage connected={false} connecting={false} textContext={null} cursorPos={null} cursorSupported={false} send={() => {}} />);
    expect(empty).toContain("Set up and pair a webOS TV");
  } finally {
    Object.assign(initialState, previousState);
    if (previousDocument) Object.defineProperty(globalThis, "document", previousDocument);
    else Reflect.deleteProperty(globalThis, "document");
  }
});
