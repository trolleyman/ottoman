import { expect, test } from "bun:test";
import { client, useStore } from "../src/store";
import { CancelablePromise } from "../src/api/core/CancelablePromise";

test("PC volume writes keep device readings authoritative until the next poll", async () => {
  const previous = useStore.getState();
  const original = client.default.setAudioVolume.bind(client.default);
  let complete!: () => void;
  client.default.setAudioVolume = () => new CancelablePromise(resolve => { complete = () => resolve({ success: true }); });
  try {
    useStore.setState({ audioSinks: [{ id: 1, name: "pc", description: "PC", default: true, volume: 0.24, muted: false }] });
    const writing = useStore.getState().setSinkVolume("pc", 0.25);
    expect(useStore.getState().audioSinks[0].volume).toBe(0.24);
    complete();
    await writing;
    expect(useStore.getState().audioSinks[0].volume).toBe(0.24);
  } finally {
    client.default.setAudioVolume = original;
    useStore.setState(previous);
  }
});

test("TV volume writes cannot falsely acknowledge themselves before a cached poll", async () => {
  const previous = useStore.getState();
  const original = client.default.setMonitorVolume.bind(client.default);
  let complete!: () => void;
  client.default.setMonitorVolume = () => new CancelablePromise(resolve => { complete = () => resolve({ success: true }); });
  try {
    useStore.setState({ monitors: [{ edid: "tv", port: "HDMI-1", name: "TV", manufacturer: "Fixture", tv_state: { paired: true, pairing: false, muted: false, on: true, volume: 9 } }] });
    const writing = useStore.getState().setMonitorVolume("tv", 10);
    expect(useStore.getState().monitors[0].tv_state?.volume).toBe(9);
    complete();
    await writing;
    expect(useStore.getState().monitors[0].tv_state?.volume).toBe(9);
  } finally {
    client.default.setMonitorVolume = original;
    useStore.setState(previous);
  }
});
