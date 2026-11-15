import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { RemotePage } from "../src/RemotePage";
import { useStore } from "../src/store";

test("disconnected remote preserves the trackpad and inert keyboard instead of TV controls", () => {
  const previousDocument = Object.getOwnPropertyDescriptor(globalThis, "document");
  const previousMedia = Object.getOwnPropertyDescriptor(globalThis, "matchMedia");
  const initial = useStore.getInitialState(); const previous = { ...initial };
  Object.defineProperty(globalThis, "document", { configurable: true, value: { fullscreenElement: null } });
  Object.defineProperty(globalThis, "matchMedia", { configurable: true, value: () => ({ matches: false }) });
  try {
    Object.assign(initial, { monitors: [] });
    for (const connecting of [false, true]) {
      const html = renderToStaticMarkup(<RemotePage connected={false} connecting={connecting} textContext={null} cursorPos={null} cursorSupported={false} send={() => {}} />);
      expect(html).toContain("remote-workspace");
      expect(html).toContain("Trackpad. Tap to click");
      expect(html).toContain('data-disconnected="true"');
      expect(html).toContain('inert=""');
      expect(html).not.toContain('aria-label="TV controls"');
      expect(html).not.toContain("Open without address bar");
    }
  } finally {
    Object.assign(initial, previous);
    if (previousDocument) Object.defineProperty(globalThis, "document", previousDocument); else Reflect.deleteProperty(globalThis, "document");
    if (previousMedia) Object.defineProperty(globalThis, "matchMedia", previousMedia); else Reflect.deleteProperty(globalThis, "matchMedia");
  }
});
