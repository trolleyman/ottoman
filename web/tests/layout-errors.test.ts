import { describe, expect, test } from "bun:test";
import { client, useStore } from "../src/store";
import { ApiError } from "../src/api";

describe("failed layout switches", () => {
  test("shows the HTTP 500 body and keeps the active layout", async () => {
    const original = client.default.switchLayout;
    client.default.switchLayout = () => {
      throw new ApiError({ method: "POST", url: "/api/layouts/switch" }, {
        url: "/api/layouts/switch", ok: false, status: 500,
        statusText: "Internal Server Error",
        body: { code: 500, error: 'layout monitor "HDMI-2" is not connected' },
      }, "Internal Server Error");
    };
    try {
      useStore.setState({ currentLayout: "normal", switching: false, layoutNotice: null });
      await useStore.getState().switchLayout("tv");
      expect(useStore.getState().currentLayout).toBe("normal");
      expect(useStore.getState().switching).toBe(false);
      expect(useStore.getState().layoutNotice).toEqual({
        kind: "warn", text: 'HTTP 500: layout monitor "HDMI-2" is not connected',
      });
    } finally {
      client.default.switchLayout = original;
    }
  });
});
