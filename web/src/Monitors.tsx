import { useState } from "react";
import { Settings, Sun, Volume2, VolumeX } from "lucide-react";
import type { Monitor, MonitorSettingsRequest } from "./api";
import { useStore } from "./store";
import { PowerToggle } from "./PowerToggle";
import { useMonitorPower } from "./useMonitorPower";

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

// MonitorControls renders the in-body sliders (brightness, and TV volume). Power
// lives in the card header (see MonitorCard).
function MonitorControls({ monitor }: { monitor: Monitor }) {
  const setMonitorBrightness = useStore((s) => s.setMonitorBrightness);
  const tv = useStore((s) => s.tv);
  const setTVVolume = useStore((s) => s.setTVVolume);
  const setTVMute = useStore((s) => s.setTVMute);

  const caps = monitor.capabilities;
  if (!caps) return null;

  const showBrightness = caps.brightness && visible(monitor, "brightness");
  // A TV-backed monitor (caps.volume) drives the network TV's volume; that state
  // lives in the shared tv store (there's only ever one TV).
  const showVolume = caps.volume && visible(monitor, "volume") && !!tv?.paired;
  if (!showBrightness && !showVolume) return null;

  const brightness = monitor.brightness ?? -1;

  return (
    <div className="flex flex-col gap-3 pt-3 border-t border-zinc-700/40">
      {showBrightness && (
        <div className="flex items-center gap-3">
          <Sun className="h-[18px] w-[18px] shrink-0 text-amber-400/90" aria-label="Brightness" />
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
      {showVolume && tv && (
        <div className="flex items-center gap-3">
          <button
            onClick={() => setTVMute(!tv.muted)}
            className="text-zinc-400 hover:text-zinc-200 transition-colors cursor-pointer"
            title={tv.muted ? "Unmute" : "Mute"}
            aria-label={tv.muted ? "Unmute" : "Mute"}
          >
            {tv.muted ? <VolumeX className="h-[18px] w-[18px]" /> : <Volume2 className="h-[18px] w-[18px]" />}
          </button>
          <input
            type="range"
            min={0}
            max={100}
            value={tv.volume}
            onChange={(e) => setTVVolume(Number(e.target.value))}
            className={`flex-1 accent-blue-500 cursor-pointer ${tv.muted ? "opacity-40" : ""}`}
          />
          <span className="text-sm text-zinc-400 font-mono w-10 text-right tabular-nums">
            {tv.volume}
          </span>
        </div>
      )}
    </div>
  );
}

const BACKENDS: { value: string; label: string }[] = [
  { value: "", label: "Auto-detect" },
  { value: "ddc", label: "Monitor (DDC/CI)" },
  { value: "tv", label: "Network TV (webOS)" },
  { value: "none", label: "None" },
];

// MonitorSettingsEditor is the per-monitor config form: friendly name, control
// backend, TV transport (when the backend is a network TV), and which controls
// are shown. Saving persists to the registry via the agent.
function MonitorSettingsEditor({ monitor, onClose }: { monitor: Monitor; onClose: () => void }) {
  const save = useStore((s) => s.saveMonitorSettings);
  const [friendlyName, setFriendlyName] = useState(monitor.friendly_name ?? "");
  const [backend, setBackend] = useState(monitor.control_backend ?? "");
  const [tvHost, setTvHost] = useState(monitor.tv?.host ?? "");
  const [tvMac, setTvMac] = useState(monitor.tv?.mac ?? "");
  const [visibility, setVisibility] = useState<Record<string, boolean>>(monitor.visibility ?? {});
  const [saving, setSaving] = useState(false);

  const controls =
    backend === "tv"
      ? ["brightness", "power", "volume"]
      : backend === "ddc"
        ? ["brightness", "power"]
        : [];

  const submit = async () => {
    setSaving(true);
    const req: MonitorSettingsRequest = {
      edid: monitor.edid,
      friendly_name: friendlyName,
      backend,
      visibility,
    };
    if (backend === "tv") {
      req.tv = { type: "webos", host: tvHost.trim(), mac: tvMac.trim() };
    }
    const ok = await save(req);
    setSaving(false);
    if (ok) onClose();
  };

  const field = "bg-zinc-900/60 border border-zinc-700/60 rounded-lg px-2.5 py-1.5 text-sm text-zinc-200 w-full focus:outline-none focus:border-zinc-500";

  return (
    <div className="flex flex-col gap-3 pt-3 border-t border-zinc-700/40">
      <label className="flex flex-col gap-1">
        <span className="text-xs text-zinc-500">Friendly name</span>
        <input className={field} value={friendlyName} placeholder={monitor.name || "Unnamed"} onChange={(e) => setFriendlyName(e.target.value)} />
      </label>

      <label className="flex flex-col gap-1">
        <span className="text-xs text-zinc-500">Control backend</span>
        <select className={field} value={backend} onChange={(e) => setBackend(e.target.value)}>
          {BACKENDS.map((b) => (
            <option key={b.value} value={b.value}>{b.label}</option>
          ))}
        </select>
      </label>

      {backend === "tv" && (
        <>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-zinc-500">TV host (IP or hostname)</span>
            <input className={field} value={tvHost} placeholder="192.168.1.50" onChange={(e) => setTvHost(e.target.value)} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-zinc-500">TV MAC (for Wake-on-LAN)</span>
            <input className={`${field} font-mono`} value={tvMac} placeholder="8C:19:B5:72:FE:62" onChange={(e) => setTvMac(e.target.value)} />
          </label>
        </>
      )}

      {controls.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="text-xs text-zinc-500">Show controls</span>
          <div className="flex flex-wrap gap-3">
            {controls.map((c) => (
              <label key={c} className="flex items-center gap-1.5 text-sm text-zinc-300 capitalize cursor-pointer">
                <input
                  type="checkbox"
                  checked={visibility[c] ?? true}
                  onChange={(e) => setVisibility((v) => ({ ...v, [c]: e.target.checked }))}
                  className="accent-blue-500 cursor-pointer"
                />
                {c}
              </label>
            ))}
          </div>
        </div>
      )}

      <div className="flex items-center gap-2 pt-1">
        <button
          onClick={submit}
          disabled={saving}
          className="flex-1 text-xs font-medium bg-blue-500/20 hover:bg-blue-500/30 text-blue-300 px-3 py-1.5 rounded-lg transition-colors cursor-pointer disabled:opacity-50"
        >
          {saving ? "Saving…" : "Save"}
        </button>
        <button
          onClick={onClose}
          className="flex-1 text-xs font-medium bg-zinc-700/40 hover:bg-zinc-600/50 text-zinc-200 px-3 py-1.5 rounded-lg transition-colors cursor-pointer"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}

// TVPairPill shows the network TV's pairing status (or a Pair button) in a
// TV-backed monitor card's header. Pairing state lives in the shared tv store.
function TVPairPill() {
  const tv = useStore((s) => s.tv);
  const pairTV = useStore((s) => s.pairTV);
  if (tv?.pairing) {
    return (
      <span className="text-xs font-medium bg-amber-500/20 text-amber-400 px-2 py-0.5 rounded-full">
        Pairing…
      </span>
    );
  }
  if (tv?.paired) {
    return (
      <span className="text-xs font-medium bg-green-500/20 text-green-400 px-2 py-0.5 rounded-full">
        Paired
      </span>
    );
  }
  return (
    <button
      onClick={pairTV}
      className="text-xs font-medium bg-blue-500/20 text-blue-400 hover:bg-blue-500/30 px-2 py-0.5 rounded-full transition-colors cursor-pointer"
    >
      Pair
    </button>
  );
}

function MonitorCard({ monitor }: { monitor: Monitor }) {
  const a = monitor.active;
  const [editing, setEditing] = useState(false);

  // Power switch (with confirmation poll) lives in the header; the hook runs
  // unconditionally, seeded optimistically from `active`.
  const { on: powerOn, loading: powerLoading, toggle: togglePower } =
    useMonitorPower(monitor.edid, !!a);
  const showPower = !!monitor.capabilities?.power && visible(monitor, "power");

  return (
    <div className={`rounded-xl border p-5 flex flex-col gap-3 ${a
      ? "border-zinc-700/50 bg-zinc-800/50"
      : "border-zinc-800/50 bg-zinc-900/50 opacity-60"
      }`}>
      <div className="flex items-center justify-between">
        <h3 className={`font-semibold truncate ${a ? "text-zinc-100" : "text-zinc-400"}`}>
          {monitor.friendly_name || monitor.name || monitor.port || "Unknown"}
        </h3>
        <div className="flex items-center gap-2">
          {monitor.control_backend === "tv" && <TVPairPill />}
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
          {showPower && (
            <PowerToggle on={powerOn} loading={powerLoading} onChange={togglePower} />
          )}
          <button
            onClick={() => setEditing((v) => !v)}
            title="Monitor settings"
            aria-label="Monitor settings"
            className={`p-1.5 rounded-lg border transition-colors cursor-pointer ${editing
              ? "bg-zinc-700/70 border-zinc-600 text-zinc-100"
              : "bg-zinc-800/70 border-zinc-700/60 text-zinc-400 hover:text-zinc-100 hover:bg-zinc-700/60 hover:border-zinc-600"
              }`}
          >
            <Settings className="h-4 w-4" />
          </button>
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

      {editing ? (
        <MonitorSettingsEditor monitor={monitor} onClose={() => setEditing(false)} />
      ) : (
        <MonitorControls monitor={monitor} />
      )}
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
