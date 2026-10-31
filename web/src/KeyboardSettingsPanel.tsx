import { ActionKey } from "./KeyboardKeys";
import { ArrowLeft } from "lucide-react";
import { supportsHaptics } from "./keyboardHaptics";
import { KEYBOARD_TIMINGS, type KeyboardSettings } from "./trackpadSettings";

function SettingSwitch({
  label,
  checked,
  onChange,
  disabled = false,
}: {
  label: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <ActionKey
      type="button"
      disabled={disabled}
      role="switch"
      aria-label={label}
      aria-checked={checked}
      onPress={() => onChange(!checked)}
      className={`relative h-5 w-9 shrink-0 disabled:opacity-40 rounded-full border transition-colors ${
        checked
          ? "border-blue-400/60 bg-blue-500"
          : "border-zinc-600 bg-zinc-700"
      }`}
    >
      <span
        className={`absolute left-0 top-0.5 h-3.5 w-3.5 rounded-full bg-white shadow transition-transform ${
          checked ? "translate-x-[18px]" : "translate-x-0.5"
        }`}
      />
    </ActionKey>
  );
}

function TimingField({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="custom-keyboard-utility flex min-h-10 items-center justify-between gap-2 rounded-md border border-white/5 bg-zinc-800/70 px-3 text-xs text-zinc-300">
      <span>{label}</span>
      <span className="flex items-center gap-1 text-zinc-500">
        <input
          type="number"
          inputMode="numeric"
          aria-label={`${label} milliseconds`}
          value={value}
          min={min}
          max={max}
          step={step}
          onChange={(event) => {
            const next = Number(event.target.value);
            if (Number.isFinite(next)) onChange(Math.min(max, Math.max(min, next)));
          }}
          className="h-7 w-16 appearance-none rounded border border-zinc-600 bg-zinc-950 px-1.5 text-right text-xs text-zinc-200 outline-none focus:border-blue-500 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
        />
        ms
      </span>
    </label>
  );
}

export function KeyboardSettingsPanel({ settings, onSettingsChange, onUseCustomKeyboard, onUseDeviceKeyboard, usingCustomKeyboard, onBack }: {
  onBack: () => void;
  settings: KeyboardSettings;
  onSettingsChange: (patch: Partial<KeyboardSettings>) => void;
  onUseCustomKeyboard: () => void;
  onUseDeviceKeyboard: () => void;
  usingCustomKeyboard: boolean;
}) {
  return (
          <div className="custom-keyboard-emoji-grid flex min-h-0 flex-1 flex-col overflow-y-auto rounded-lg bg-zinc-900/70 p-3">
            <ActionKey type="button" onPress={onBack} className="custom-keyboard-utility mb-3 flex min-h-9 shrink-0 items-center gap-2 rounded-md bg-zinc-800 px-3 text-sm font-medium text-zinc-200">
              <ArrowLeft className="h-4 w-4" />Back to keyboard
            </ActionKey>
            <div className="mb-3 grid grid-cols-2 gap-2 text-xs text-zinc-300">
              <label className="flex flex-col gap-1">Keyboard layout
                <select aria-label="Keyboard layout" value={settings.keyboardLayout}
                  onChange={(event) => onSettingsChange({ keyboardLayout: event.target.value as KeyboardSettings["keyboardLayout"] })}
                  className="custom-keyboard-utility rounded-md border border-zinc-600 bg-zinc-800 p-2">
                  <option value="en-GB">British (UK)</option><option value="en-US">English (US)</option><option value="auto">From device locale</option>
                </select>
              </label>
              <label className="flex flex-col gap-1">Keyboard appearance
                <select aria-label="Keyboard appearance" value={settings.keyboardTheme}
                  onChange={(event) => onSettingsChange({ keyboardTheme: event.target.value as KeyboardSettings["keyboardTheme"] })}
                  className="custom-keyboard-utility rounded-md border border-zinc-600 bg-zinc-800 p-2">
                  <option value="dark">Dark</option><option value="light">Light</option><option value="system">Match device</option>
                </select>
              </label>
            </div>
            <div className="mb-3 grid grid-cols-2 rounded-lg bg-zinc-950 p-1 text-xs">
              <ActionKey
                type="button"
                onPress={onUseCustomKeyboard}
                className={`rounded-md px-2 py-2 ${usingCustomKeyboard ? "bg-zinc-700 text-white" : "text-zinc-400 hover:bg-zinc-800"}`}
                aria-pressed={usingCustomKeyboard}
              >
                Ottoman
              </ActionKey>
              <ActionKey
                type="button"
                onPress={onUseDeviceKeyboard}
                className={`rounded-md px-2 py-2 ${!usingCustomKeyboard ? "bg-zinc-700 text-white" : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"}`}
                aria-pressed={!usingCustomKeyboard}
              >
                Device
              </ActionKey>
            </div>
            <div className="grid grid-cols-1 gap-1 sm:grid-cols-2 sm:gap-x-4">
              {([
                ["Mirror focused text", "mirrorFocusedText"],
                ["Number row", "numberRow"],
                ["Long-press symbols", "symbolHints"],
                ["Double-space period", "doubleSpacePeriod"],
                ["Key haptics", "haptics"],
                ["Period beside space", "periodKey"],
                ["Swipe typing", "swipeTyping"],
              ] as const).map(([label, key]) => (
                <div
                  key={key}
                  className="custom-keyboard-utility flex min-h-10 items-center justify-between gap-3 rounded-md border border-white/5 bg-zinc-800/70 px-3 text-xs text-zinc-300"
                >
                  {label}
                  <SettingSwitch
                    label={label}
                    checked={settings[key]}
                    disabled={key === "haptics" && !supportsHaptics()}
                    onChange={(checked) => onSettingsChange({ [key]: checked })}
                  />
                </div>
              ))}
            </div>
            {!supportsHaptics() && <p className="mt-2 text-[11px] leading-4 text-zinc-400">This browser does not support key vibration. Press previews and scroll feedback stay enabled. The device keyboard uses your phone’s keyboard feedback settings.</p>}
            <div className="mb-1 mt-4 text-[11px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
              Timing
            </div>
            <div className="grid grid-cols-1 gap-1 sm:grid-cols-2 sm:gap-x-4">
              {KEYBOARD_TIMINGS.map(({ key, label, min, max, step }) => (
                <TimingField key={key} label={label} value={settings[key]} min={min} max={max} step={step}
                  onChange={(value) => onSettingsChange({ [key]: value })} />
              ))}
            </div>
            {document.fullscreenEnabled && <ActionKey type="button"
              onPress={() => { void (document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => {}); }}
              className="custom-keyboard-utility mt-3 min-h-9 shrink-0 rounded-md border border-zinc-600 bg-zinc-800 px-3 text-xs text-zinc-200">Toggle fullscreen</ActionKey>}
            <p className="mt-3 text-[11px] leading-4 text-zinc-400">On iPhone, use Share → Add to Home Screen, then open Ottoman from that icon to type without Safari’s address bar.</p>
            <p className="mt-3 text-[11px] leading-4 text-zinc-500">
              Mirror focused text lets the desktop agent read the focused editable field through operating-system accessibility APIs. Passwords appear only as dots; their contents are never mirrored. Enable accessibility access on the desktop if requested.
              Device uses the phone's native keyboard. Tap the small settings button on the trackpad to return here.
            </p>
          </div>
  );
}
