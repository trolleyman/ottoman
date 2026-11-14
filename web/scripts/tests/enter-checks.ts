import type { Page } from "playwright";
import type { TrackpadMessage, TrackpadMessageTextContext } from "../../src/api";

export async function checkEnter(page: Page) {
  const fixture = (expression: string) => page.evaluate(`window.fixture.${expression}`);
  const fields: Partial<TrackpadMessageTextContext>[] = [
    { fieldId: "login", available: false, editable: false, protected: true, password: true },
    { fieldId: "terminal", available: false, editable: false },
    { fieldId: "single-line", multiline: false },
    { fieldId: "multiline" },
    { fieldId: "rich-editor", editable: false },
  ];
  const setField = async (field: Partial<TrackpadMessageTextContext>) => {
    await fixture(`setField(${JSON.stringify(field)})`);
    await page.waitForTimeout(50);
  };
  const assertEnter = async (modifiers: string[] = []) => {
    const messages = (await fixture("log") as TrackpadMessage[]).filter(message => message.type !== "textwatch");
    const expected = ["keydown", "keyup"].map(type => ({ type, key: "Enter", modifiers }));
    if (JSON.stringify(messages) !== JSON.stringify(expected)) throw Error(`Enter must send only a native key pair: ${JSON.stringify(messages)}`);
  };
  const combinations = [
    { label: "Shift", modifier: "shift", flag: "shiftKey" },
    { label: "Control", modifier: "ctrl", flag: "ctrlKey" },
    { label: "Alt", modifier: "alt", flag: "altKey" },
  ];
  for (const field of fields) {
    await setField(field);
    await page.getByRole("button", { name: "Enter", exact: true }).click();
    await assertEnter();
    for (const { label, modifier } of combinations) {
      await setField(field);
      await page.getByRole("button", { name: label, exact: true }).click();
      await page.getByRole("button", { name: "Enter", exact: true }).click();
      await assertEnter([modifier]);
    }
  }
  await fixture("configure({ customKeyboard: false })");
  const input = page.getByLabel("Type on the remote computer with the device keyboard");
  for (const field of fields) {
    await setField(field);
    await input.press("Enter");
    await assertEnter();
    for (const { modifier, flag } of combinations) {
      await setField(field);
      // Modifiers supplied on Enter without separate modifier key events.
      await input.dispatchEvent("keydown", { key: "Enter", [flag]: true });
      await assertEnter([modifier]);
    }
  }
  for (const inputType of ["insertLineBreak", "insertParagraph"]) {
    await setField(fields[1]);
    await input.evaluate((element, inputType) => {
      const textarea = element as HTMLTextAreaElement;
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(textarea, textarea.value + "\n");
      textarea.dispatchEvent(new InputEvent("input", { bubbles: true, inputType, data: null }));
    }, inputType);
    await assertEnter();
  }
  await fixture("configure({ interactivePreview: true })");
  await setField({ fieldId: "fixture" });
  console.log("PASS: Enter sends native key events in login, terminal and text fields; Shift/Ctrl/Alt+Enter and device Return preserve key semantics");
}
