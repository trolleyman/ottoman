// A compact pill on/off switch for monitor/TV power, sized to sit in a card
// header next to the settings button. Power has no direct state read-back, so
// the caller drives `on` optimistically and passes `loading` while it polls the
// backend to confirm the monitor reached the target state (see useMonitorPower);
// while loading the knob shows a spinner and the switch is disabled.
export function PowerToggle({
  on,
  onChange,
  label = "Power",
  loading = false,
  disabled = false,
}: {
  on: boolean;
  onChange: (on: boolean) => void;
  label?: string;
  loading?: boolean;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-busy={loading}
      aria-label={label}
      title={label}
      disabled={disabled || loading}
      onClick={() => onChange(!on)}
      className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors cursor-pointer disabled:cursor-default focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/50 ${on ? "bg-emerald-500/80" : "bg-zinc-600"
        } ${disabled && !loading ? "opacity-40 disabled:cursor-not-allowed" : ""}`}
    >
      <span
        className={`inline-flex h-5 w-5 items-center justify-center transform rounded-full bg-white shadow transition-transform ${on ? "translate-x-[22px]" : "translate-x-0.5"
          }`}
      >
        {loading && (
          <span className="h-3 w-3 rounded-full border-2 border-zinc-300 border-t-zinc-500 animate-spin" />
        )}
      </span>
    </button>
  );
}
