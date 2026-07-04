// A pill-style on/off switch used for monitor and TV power. Power is a
// fire-and-forget command (DDC standby / TV WoL+SSAP) with no state to read
// back, so callers drive this optimistically from local state.
export function PowerToggle({
  on,
  onChange,
  label = "Power",
  disabled = false,
}: {
  on: boolean;
  onChange: (on: boolean) => void;
  label?: string;
  disabled?: boolean;
}) {
  return (
    <div className="flex items-center justify-between">
      <span className={`text-sm ${on ? "text-zinc-300" : "text-zinc-500"}`}>
        {label}
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={on}
        aria-label={label}
        disabled={disabled}
        onClick={() => onChange(!on)}
        className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-40 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/50 ${on ? "bg-emerald-500/80" : "bg-zinc-700"
          }`}
      >
        <span
          className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${on ? "translate-x-[22px]" : "translate-x-0.5"
            }`}
        />
      </button>
    </div>
  );
}
