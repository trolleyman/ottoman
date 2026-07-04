// A pill-style on/off switch used for monitor and TV power. Power has no direct
// state read-back, so the caller drives `on` optimistically and passes
// `loading` while it polls the backend to confirm the monitor actually reached
// the target state (see MonitorControls). While loading the knob shows a
// spinner and the switch is disabled.
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
    <div className="flex items-center justify-between">
      <span className={`text-sm ${on ? "text-zinc-300" : "text-zinc-500"}`}>
        {label}
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={on}
        aria-busy={loading}
        aria-label={label}
        disabled={disabled || loading}
        onClick={() => onChange(!on)}
        className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors cursor-pointer disabled:cursor-default focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/50 ${on ? "bg-emerald-500/80" : "bg-zinc-700"
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
    </div>
  );
}
