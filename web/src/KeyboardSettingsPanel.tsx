import { KEYBOARD_TIMINGS, type KeyboardSettings } from "./trackpadSettings";

function SettingSwitch({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-label={label}
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`relative h-5 w-9 shrink-0 rounded-full border transition-colors ${
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
    </button>
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

export function KeyboardSettingsPanel({ settings, onSettingsChange, onUseCustomKeyboard, onUseDeviceKeyboard, usingCustomKeyboard }: {
  settings: KeyboardSettings;
  onSettingsChange: (patch: Partial<KeyboardSettings>) => void;
  onUseCustomKeyboard: () => void;
  onUseDeviceKeyboard: () => void;
  usingCustomKeyboard: boolean;
}) {
  return (
          <div className="custom-keyboard-emoji-grid flex min-h-0 flex-1 flex-col overflow-y-auto rounded-lg bg-zinc-900/70 p-3">
            <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
              Keyboard settings
            </div>
            <div className="mb-3 grid grid-cols-2 rounded-lg bg-zinc-950 p-1 text-xs">
              <button
                type="button"
                onClick={onUseCustomKeyboard}
                className={`rounded-md px-2 py-2 ${usingCustomKeyboard ? "bg-zinc-700 text-white" : "text-zinc-400 hover:bg-zinc-800"}`}
                aria-pressed={usingCustomKeyboard}
              >
                Ottoman
              </button>
              <button
                type="button"
                onClick={onUseDeviceKeyboard}
                className={`rounded-md px-2 py-2 ${!usingCustomKeyboard ? "bg-zinc-700 text-white" : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"}`}
                aria-pressed={!usingCustomKeyboard}
              >
                Device
              </button>
            </div>
            <div className="grid grid-cols-1 gap-1 sm:grid-cols-2 sm:gap-x-4">
              {([
                ["Number row", "numberRow"],
                ["Long-press symbols", "symbolHints"],
                ["Double-space period", "doubleSpacePeriod"],
                ["Key haptics", "haptics"],
              ] as const).map(([label, key]) => (
                <div
                  key={key}
                  className="custom-keyboard-utility flex min-h-10 items-center justify-between gap-3 rounded-md border border-white/5 bg-zinc-800/70 px-3 text-xs text-zinc-300"
                >
                  {label}
                  <SettingSwitch
                    label={label}
                    checked={settings[key]}
                    onChange={(checked) => onSettingsChange({ [key]: checked })}
                  />
                </div>
              ))}
            </div>
            <div className="mb-1 mt-4 text-[11px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
              Timing
            </div>
            <div className="grid grid-cols-1 gap-1 sm:grid-cols-2 sm:gap-x-4">
              {KEYBOARD_TIMINGS.map(({ key, label, min, max, step }) => (
                <TimingField key={key} label={label} value={settings[key]} min={min} max={max} step={step}
                  onChange={(value) => onSettingsChange({ [key]: value })} />
              ))}
            </div>
            <p className="mt-3 text-[11px] leading-4 text-zinc-500">
              Device uses the phone's native keyboard. Tap the small settings button on the trackpad to return here.
            </p>
          </div>
  );
}
