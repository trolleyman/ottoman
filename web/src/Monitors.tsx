import type { Monitor } from "./api";
import { useStore } from "./store";

function Row({ label, value }: { label: string; value: string }) {
  return (
    <>
      <span className="text-zinc-500">{label}</span>
      <span className="text-zinc-300 font-mono text-xs leading-5 truncate">
        {value}
      </span>
    </>
  );
}

// visible reports whether a control should show, honouring the registry's
// per-monitor visibility overrides (absent = visible).
function visible(monitor: Monitor, control: string): boolean {
  const v = monitor.visibility?.[control];
  return v === undefined ? true : v;
}

function MonitorControls({ monitor }: { monitor: Monitor }) {
  const setMonitorBrightness = useStore((s) => s.setMonitorBrightness);
  const setMonitorPower = useStore((s) => s.setMonitorPower);

  const caps = monitor.capabilities;
  if (!caps) return null;

  const showBrightness = caps.brightness && visible(monitor, "brightness");
  const showPower = caps.power && visible(monitor, "power");
  if (!showBrightness && !showPower) return null;

  const brightness = monitor.brightness ?? -1;

  return (
    <div className="flex flex-col gap-3 pt-3 border-t border-zinc-700/40">
      {showBrightness && (
        <div className="flex items-center gap-3">
          <span className="text-lg leading-none select-none" title="Brightness">
            ☀️
          </span>
          <input
            type="range"
            min={0}
            max={100}
            value={brightness < 0 ? 50 : brightness}
            disabled={brightness < 0}
            onChange={(e) => setMonitorBrightness(monitor.edid, Number(e.target.value))}
            className="flex-1 accent-amber-500 cursor-pointer disabled:opacity-40"
          />
          <span className="text-sm text-zinc-400 font-mono w-10 text-right tabular-nums">
            {brightness < 0 ? "—" : `${brightness}%`}
          </span>
        </div>
      )}
      {showPower && (
        <div className="flex items-center gap-2">
          <button
            onClick={() => setMonitorPower(monitor.edid, true)}
            className="flex-1 text-xs font-medium bg-zinc-700/40 hover:bg-zinc-600/50 text-zinc-200 px-3 py-1.5 rounded-lg transition-colors cursor-pointer"
          >
            Power on
          </button>
          <button
            onClick={() => setMonitorPower(monitor.edid, false)}
            className="flex-1 text-xs font-medium bg-zinc-700/40 hover:bg-zinc-600/50 text-zinc-200 px-3 py-1.5 rounded-lg transition-colors cursor-pointer"
          >
            Power off
          </button>
        </div>
      )}
    </div>
  );
}

function MonitorCard({ monitor }: { monitor: Monitor }) {
  const a = monitor.active;
  return (
    <div className={`rounded-xl border p-5 flex flex-col gap-3 ${a
      ? "border-zinc-700/50 bg-zinc-800/50"
      : "border-zinc-800/50 bg-zinc-900/50 opacity-60"
      }`}>
      <div className="flex items-center justify-between">
        <h3 className={`font-semibold truncate ${a ? "text-zinc-100" : "text-zinc-400"}`}>
          {monitor.friendly_name || monitor.name || monitor.port || "Unknown"}
        </h3>
        <div className="flex gap-2">
          {!a && (
            <span className="text-xs font-medium bg-zinc-700/30 text-zinc-500 px-2 py-0.5 rounded-full">
              Inactive
            </span>
          )}
          {a?.primary && (
            <span className="text-xs font-medium bg-blue-500/20 text-blue-400 px-2 py-0.5 rounded-full">
              Primary
            </span>
          )}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm">
        {a && (
          <>
            <Row label="Resolution" value={`${a.width}x${a.height}`} />
            <Row
              label="Refresh"
              value={`${Number.isInteger(a.refresh_rate) ? a.refresh_rate : a.refresh_rate.toFixed(1)} Hz`}
            />
            <Row label="Position" value={`${a.position_x}, ${a.position_y}`} />
          </>
        )}
        {monitor.port && <Row label="Port" value={monitor.port} />}
        {monitor.edid && <Row label="EDID" value={monitor.edid} />}
        {monitor.manufacturer && (
          <Row label="Manufacturer" value={monitor.manufacturer} />
        )}
      </div>

      <MonitorControls monitor={monitor} />
    </div>
  );
}

export function Monitors() {
  const monitors = useStore((s) => s.monitors);
  const loading = useStore((s) => s.monitorsLoading);
  const error = useStore((s) => s.monitorsError);

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          Monitors
          {loading && monitors.length > 0 && (
            <div className="w-3.5 h-3.5 border-2 border-zinc-600 border-t-zinc-400 rounded-full animate-spin" />
          )}
        </h2>
        <span className="text-xs text-zinc-500">
          {monitors.filter((m) => m.active).length} active / {monitors.length} total
        </span>
      </div>
      {loading && monitors.length === 0 ? (
        <div className="text-zinc-500 text-sm">Loading monitors...</div>
      ) : error ? (
        <div className="text-red-400 text-sm">{error}</div>
      ) : monitors.length === 0 ? (
        <p className="text-zinc-500 text-sm">No monitors detected.</p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {monitors.map((m, i) => (
            <MonitorCard key={m.port || m.edid || i} monitor={m} />
          ))}
        </div>
      )}
    </section>
  );
}
