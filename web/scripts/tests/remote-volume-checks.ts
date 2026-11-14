import type { Page } from "playwright";

export async function checkRemoteVolume(page: Page) {
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const fixture = (code: string) => page.evaluate(`window.fixture.${code}`);
  const pad = page.getByLabel(/Trackpad\. Tap to click/);
  const key = async (key: string) => {
    await pad.dispatchEvent("keydown", { key });
    await pad.dispatchEvent("keyup", { key });
  };
  const volume = (label: string) => page.getByRole("slider", { name: `${label} volume`, exact: true });
  const setVolume = async (label: string, value: number) => {
    await volume(label).fill(String(value));
    await page.waitForTimeout(150);
  };
  const setting = page.getByRole("checkbox", { name: "Volume keys control selected output", exact: true });
  await fixture('reset("volume")');
  await key("AudioVolumeUp");
  assert(await volume("PC").inputValue() === "41", "Default keys must adjust the selected slider");
  assert(!(await fixture("log") as { key?: string }[]).some(item => item.key === "AudioVolumeUp"), "Default volume keys must not reach OS");
  await page.waitForTimeout(60);
  assert(await volume("PC").inputValue() === "41", "Stale PC refresh must not undo a key step before the write completes");
  await page.waitForTimeout(100);
  await key("AudioVolumeDown");
  await page.getByRole("button", { name: "Trackpad settings", exact: true }).click();
  await setting.check();
  await page.screenshot({ path: "/tmp/remote-volume-settings@2x.png" });
  await page.getByRole("button", { name: "Close settings", exact: true }).click();
  assert(await page.evaluate(() => (JSON.parse(localStorage.getItem("ottoman-trackpad-settings")!) as { volumeKeysControlOutput: boolean }).volumeKeysControlOutput) === true, "Preference must be saved");
  await fixture('reset("volume")');
  await page.getByRole("button", { name: "Function keys", exact: true }).click();
  await page.getByRole("button", { name: "Volume up", exact: true }).click();
  assert(await volume("PC").inputValue() === "41", "Fn volume up must step 1%");
  await page.getByRole("button", { name: "Volume down", exact: true }).click();
  assert(await volume("PC").inputValue() === "40", "Fn volume down must step 1%");
  await page.getByRole("button", { name: "Mute", exact: true }).click();
  assert((await fixture("log") as { key?: string }[]).filter(item => item.key === "AudioVolumeMute").length === 2, "Mute must retain OS behavior");
  await page.getByRole("button", { name: "Function keys", exact: true }).click();
  await page.waitForTimeout(250);
  await fixture("addTV()");
  assert(await volume("TV").inputValue() === "12", "TV must become selected output");
  await fixture("volumeWrites.splice(0)");
  await pad.evaluate(element => {
    for (let i = 0; i < 8; i++) {
      element.dispatchEvent(new KeyboardEvent("keydown", { key: "AudioVolumeUp", bubbles: true }));
      element.dispatchEvent(new KeyboardEvent("keyup", { key: "AudioVolumeUp", bubbles: true }));
    }
  });
  assert(await volume("TV").inputValue() === "20", "Rapid presses must accumulate against optimistic value");
  await page.waitForTimeout(60);
  assert(await volume("TV").inputValue() === "20", "Stale TV refresh must not reset accumulated key steps");
  await page.waitForTimeout(190);
  const writes = await fixture("volumeWrites") as { target: string; value: number }[];
  assert(writes.length === 2 && writes[0].value === 13 && writes[1].value === 20 && writes.every(item => item.target === "fixture-tv"), `Slow TV writes must coalesce on the selected target: ${JSON.stringify(writes)}`);
  const brightness = await fixture("brightness()");
  await page.getByRole("button", { name: "Switch to brightness", exact: true }).click();
  await key("AudioVolumeDown");
  assert(await page.getByRole("slider", { name: "Screen brightness", exact: true }).inputValue() === String(brightness), "Volume keys must leave brightness unchanged");
  await page.getByRole("button", { name: "Switch to volume", exact: true }).click();
  assert(await volume("TV").inputValue() === "19", "Keys must adjust audio while brightness is shown");
  await setVolume("TV", 100);
  const before = await volume("TV").inputValue();
  await key("AudioVolumeUp");
  assert(await volume("TV").inputValue() === "100", `Volume must clamp at 100%: before=${before}, after=${await volume("TV").inputValue()}, writes=${JSON.stringify(await fixture("volumeWrites"))}`);
  await setVolume("TV", 0);
  await key("AudioVolumeDown");
  assert(await volume("TV").inputValue() === "0", "Volume must clamp at zero");
  await page.getByRole("button", { name: "Audio output: TV. Switch output", exact: true }).click();
  await key("AudioVolumeUp");
  assert(await volume("PC").inputValue() === "41", "Output switching must route next step to PC");
  assert(!(await fixture("log") as { key?: string }[]).some(item => item.key === "AudioVolumeUp" || item.key === "AudioVolumeDown"), "Redirected keydown and keyup must not reach OS");
  await setVolume("PC", 40);
  await fixture("removeTV()");
  await page.getByRole("button", { name: "Trackpad settings", exact: true }).click();
  await setting.uncheck();
  await page.getByRole("button", { name: "Close settings", exact: true }).click();
  await fixture('reset("volume")');
  await key("AudioVolumeDown");
  assert((await fixture("log") as { key?: string }[]).filter(item => item.key === "AudioVolumeDown").length === 2, "Disabling setting must restore OS keys");
}
