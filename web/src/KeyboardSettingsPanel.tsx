import { useState } from "react";
import { ArrowLeft } from "lucide-react";
import { supportsHaptics } from "./keyboardHaptics";
import { DEFAULT_SETTINGS, TERMINAL_FONTS, KEYBOARD_TIMINGS, TRACKPAD_GESTURES, TRACKPAD_RANGES, type TrackpadSettings } from "./trackpadSettings";

type BooleanKey = { [K in keyof TrackpadSettings]: TrackpadSettings[K] extends boolean ? K : never }[keyof TrackpadSettings];

export function KeyboardSettingsPanel({ settings, onSettingsChange, onBack }: {
  settings: TrackpadSettings; onSettingsChange: (patch: Partial<TrackpadSettings>) => void; onBack: () => void;
}) {
  const [reset, setReset] = useState(false);
  const [tab, setTab] = useState("Keyboard");
  const toggle = (label: string, key: BooleanKey) => <label key={key} className="input-setting-row"><span>{label}</span><input type="checkbox" role="switch" aria-label={label} aria-checked={settings[key]} checked={settings[key]} disabled={key === "haptics" && !supportsHaptics()} onChange={e => onSettingsChange({ [key]: e.target.checked })} /></label>;
  const fields = (items: readonly (readonly [string, BooleanKey])[]) => <div className="input-settings-group">{items.map(([label, key]) => toggle(label, key))}</div>;
  return <section className="input-settings" aria-label="Input settings">
    <header><button type="button" onClick={onBack} aria-label="Back to keyboard"><ArrowLeft /></button><h2>Settings</h2></header>
    <div className="input-settings-tabs" role="tablist" aria-label="Input settings">{["Keyboard", "Trackpad"].map(name => <button key={name} id={`settings-tab-${name}`} aria-controls={`settings-${name}`} type="button" role="tab" aria-selected={tab === name} onClick={() => setTab(name)}>{name}</button>)}</div>
    <div role="tabpanel" id="settings-Keyboard" aria-labelledby="settings-tab-Keyboard" hidden={tab !== "Keyboard"}>
      <div className="input-settings-group">
        <label className="input-setting-row">Keyboard<select aria-label="Keyboard" value={settings.customKeyboard ? "custom" : "device"} onChange={e => onSettingsChange({ customKeyboard: e.target.value === "custom" })}><option value="custom">Ottoman</option><option value="device">Device</option></select></label>
        <label className="input-setting-row">Layout<select aria-label="Keyboard layout" value={settings.keyboardLayout} onChange={e => onSettingsChange({ keyboardLayout: e.target.value as TrackpadSettings["keyboardLayout"] })}><option value="en-GB">British (UK)</option><option value="en-US">English (US)</option><option value="auto">Device locale</option></select></label>
        <label className="input-setting-row">Appearance<select aria-label="Keyboard appearance" value={settings.keyboardTheme} onChange={e => onSettingsChange({ keyboardTheme: e.target.value as TrackpadSettings["keyboardTheme"] })}><option value="dark">Dark</option><option value="light">Light</option><option value="system">System</option></select></label>
      </div>
      <h3>Typing</h3>{fields([["Swipe typing", "swipeTyping"], ["Autocorrect", "autocorrect"], ["Double-space period", "doubleSpacePeriod"], ["Key haptics", "haptics"]])}
      <details open><summary>Keys</summary>{fields([["Show emoji key on letters", "emojiKey"], ["Number row", "numberRow"], ["Taller keys", "tallerKeys"], ["Long-press symbols", "symbolHints"], ["Tab beside space", "tabKey"], ["Comma beside space", "commaKey"], ["Period beside space", "periodKey"], ["Suggestions above Ctrl row", "suggestionsAboveControls"]])}</details>
      <details><summary>Text & terminal</summary>{fields([["Mirror focused text", "mirrorFocusedText"], ["Show terminal scrollback", "terminalScrollback"], ["Navigate by preview lines", "previewNavigation"]])}
        <label className="input-setting-row">Terminal font<select aria-label="Terminal font" value={settings.terminalFont} onChange={e => onSettingsChange({ terminalFont: e.target.value as TrackpadSettings["terminalFont"] })}>{Object.entries(TERMINAL_FONTS).map(([key, font]) => <option key={key} value={key}>{font.label}</option>)}</select></label><p>Focused text uses desktop accessibility. Passwords stay hidden.</p>
      </details>
      <details><summary>Prediction & gestures</summary>{fields([["Adaptive key sizes", "adaptiveKeys"], ["Debug key sizes", "debugKeySizes"]])}
        <label className="input-setting-row">Space cursor sensitivity<input type="range" aria-label="Space cursor sensitivity" min="0.5" max="5" step="0.25" value={settings.spaceCursorSensitivity} onChange={e => onSettingsChange({ spaceCursorSensitivity: Number(e.target.value) })} /><output>{settings.spaceCursorSensitivity}×</output></label>
        {KEYBOARD_TIMINGS.map(({ key, label, min, max, step }) => <label className="input-setting-row" key={key}>{label}<span><input type="number" aria-label={`${label} milliseconds`} min={min} max={max} step={step} value={settings[key]} onChange={e => { const value = e.target.valueAsNumber; if (Number.isFinite(value)) onSettingsChange({ [key]: Math.min(max, Math.max(min, value)) }); }} /> ms</span></label>)}
      </details>
    </div>
    <div role="tabpanel" id="settings-Trackpad" aria-labelledby="settings-tab-Trackpad" hidden={tab !== "Trackpad"}>
      {TRACKPAD_RANGES.map(({ key, label, min, max, step, decimals }) => <label key={key} className="input-setting-range"><span>{label}<output>{settings[key].toFixed(decimals)}</output></span><input type="range" aria-label={label} min={min} max={max} step={step} value={settings[key]} onChange={e => onSettingsChange({ [key]: Number(e.target.value) })} /></label>)}
      <h3>Gestures</h3>{fields([["Hold to right-click", "holdRightClick"]])}
      <div className="input-settings-group">{TRACKPAD_GESTURES.map(({ key, label, min, max, step, unit }) => <label key={key} className="input-setting-row">{label}<span><input type="number" aria-label={label} min={min} max={max} step={step} disabled={!settings.holdRightClick} value={settings[key]} onChange={e => { const value = e.target.valueAsNumber; if (Number.isFinite(value)) onSettingsChange({ [key]: Math.min(max, Math.max(min, value)) }); }} /> {unit}</span></label>)}</div>
      {fields([["Volume keys control selected output", "volumeKeysControlOutput"]])}
    </div>
    <button type="button" className="input-settings-reset" onClick={() => { onSettingsChange({ ...DEFAULT_SETTINGS }); setReset(true); }}>Reset all settings to defaults</button>
    {reset && <p role="status">Settings reset to defaults.</p>}
  </section>;
}
