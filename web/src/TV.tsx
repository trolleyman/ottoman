import { useState } from "react";
import { useStore } from "./store";
import { PowerToggle } from "./PowerToggle";
import { Row } from "./Monitors";

// TVCard renders the configured TV as a card that lives inside the Monitors
// grid, styled to match MonitorCard. Returns null when no TV is configured.
export function TVCard() {
  const tv = useStore((s) => s.tv);
  const pairTV = useStore((s) => s.pairTV);
  const setTVPower = useStore((s) => s.setTVPower);
  const setTVVolume = useStore((s) => s.setTVVolume);
  const setTVMute = useStore((s) => s.setTVMute);

  // Optimistic power state — like monitors, TV power has no read-back. A
  // configured+paired TV is reachable, so start "on".
  const [powerOn, setPowerOn] = useState(true);

  if (!tv || !tv.configured) return null;

  return (
    <div className="rounded-xl border border-zinc-700/50 bg-zinc-800/50 p-5 flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h3 className="font-semibold text-zinc-100 truncate">LG webOS TV</h3>
        {tv.pairing ? (
          <span className="text-xs font-medium bg-amber-500/20 text-amber-400 px-2 py-0.5 rounded-full">
            Pairing…
          </span>
        ) : tv.paired ? (
          <span className="text-xs font-medium bg-green-500/20 text-green-400 px-2 py-0.5 rounded-full">
            Paired
          </span>
        ) : (
          <button
            onClick={pairTV}
            className="text-xs font-medium bg-blue-500/20 text-blue-400 hover:bg-blue-500/30 px-2 py-0.5 rounded-full transition-colors cursor-pointer"
          >
            Pair
          </button>
        )}
      </div>

      <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm">
        <Row label="Type" value="webOS" />
        {tv.host && <Row label="Host" value={tv.host} />}
      </div>

      {tv.error && !tv.paired && (
        <p className="text-xs text-red-400">{tv.error}</p>
      )}

      <div className="flex flex-col gap-3 pt-3 border-t border-zinc-700/40">
        <PowerToggle
          on={powerOn}
          onChange={(on) => {
            setPowerOn(on);
            setTVPower(on);
          }}
        />

        {tv.paired && (
          <div className="flex items-center gap-3">
            <button
              onClick={() => setTVMute(!tv.muted)}
              className="text-xl leading-none cursor-pointer select-none"
              title={tv.muted ? "Unmute" : "Mute"}
            >
              {tv.muted ? "🔇" : "🔊"}
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
    </div>
  );
}
