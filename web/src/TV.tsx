import { useStore } from "./store";

export function TV() {
  const tv = useStore((s) => s.tv);
  const pairTV = useStore((s) => s.pairTV);
  const setTVPower = useStore((s) => s.setTVPower);
  const setTVVolume = useStore((s) => s.setTVVolume);
  const setTVMute = useStore((s) => s.setTVMute);

  // Hide the section unless a TV is configured on the agent.
  if (!tv || !tv.configured) return null;

  return (
    <section>
      <h2 className="text-lg font-semibold text-zinc-200 mb-4">TV</h2>
      <div className="rounded-xl border border-zinc-700/50 bg-zinc-800/50 p-5 flex flex-col gap-4 max-w-md">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="font-semibold text-zinc-100">LG webOS TV</h3>
            <p className="text-xs text-zinc-500 font-mono">{tv.host}</p>
          </div>
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

        {tv.error && !tv.paired && (
          <p className="text-xs text-red-400">{tv.error}</p>
        )}

        <div className="flex items-center gap-2">
          <button
            onClick={() => setTVPower(true)}
            className="flex-1 text-xs font-medium bg-zinc-700/40 hover:bg-zinc-600/50 text-zinc-200 px-3 py-1.5 rounded-lg transition-colors cursor-pointer"
          >
            Power on
          </button>
          <button
            onClick={() => setTVPower(false)}
            className="flex-1 text-xs font-medium bg-zinc-700/40 hover:bg-zinc-600/50 text-zinc-200 px-3 py-1.5 rounded-lg transition-colors cursor-pointer"
          >
            Power off
          </button>
        </div>

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
    </section>
  );
}
