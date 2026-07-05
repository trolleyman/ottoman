import { Volume1, Volume2, VolumeX } from "lucide-react";
import type { AudioSink } from "./api";
import { useStore } from "./store";

function VolumeIcon({ muted, volume, className }: { muted: boolean; volume: number; className?: string }) {
  if (muted || volume === 0) return <VolumeX className={className} />;
  if (volume < 0.5) return <Volume1 className={className} />;
  return <Volume2 className={className} />;
}

function SinkCard({ sink }: { sink: AudioSink }) {
  const setSinkVolume = useStore((s) => s.setSinkVolume);
  const setSinkMute = useStore((s) => s.setSinkMute);
  const setSinkDefault = useStore((s) => s.setSinkDefault);

  const pct = Math.round(sink.volume * 100);

  return (
    <div className="rounded-xl border border-zinc-700/50 bg-zinc-800/50 p-5 flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <h3 className="font-semibold text-zinc-100 truncate" title={sink.name}>
          {sink.description || sink.name}
        </h3>
        {sink.default ? (
          <span className="text-xs font-medium bg-blue-500/20 text-blue-400 px-2 py-0.5 rounded-full whitespace-nowrap">
            Default
          </span>
        ) : (
          <button
            onClick={() => void setSinkDefault(sink.name)}
            className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer whitespace-nowrap"
          >
            Make default
          </button>
        )}
      </div>

      <div className="flex items-center gap-3">
        <button
          onClick={() => void setSinkMute(sink.name, !sink.muted)}
          className="text-zinc-400 hover:text-zinc-200 transition-colors cursor-pointer"
          title={sink.muted ? "Unmute" : "Mute"}
          aria-label={sink.muted ? "Unmute" : "Mute"}
        >
          <VolumeIcon muted={sink.muted} volume={sink.volume} className="h-[18px] w-[18px]" />
        </button>
        <input
          type="range"
          min={0}
          max={150}
          value={pct}
          onChange={(e) => void setSinkVolume(sink.name, Number(e.target.value) / 100)}
          className={`flex-1 accent-blue-500 cursor-pointer ${sink.muted ? "opacity-40" : ""}`}
        />
        <span className="text-sm text-zinc-400 font-mono w-10 text-right tabular-nums">
          {pct}%
        </span>
      </div>
    </div>
  );
}

export function Audio() {
  const sinks = useStore((s) => s.audioSinks);
  const loading = useStore((s) => s.audioLoading);
  const error = useStore((s) => s.audioError);

  // Audio is optional (no PipeWire on some hosts); hide the section entirely
  // when there's nothing to show rather than surfacing a scary error.
  if (!loading && !error && sinks.length === 0) return null;

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          Audio
          {loading && sinks.length > 0 && (
            <div className="w-3.5 h-3.5 border-2 border-zinc-600 border-t-zinc-400 rounded-full animate-spin" />
          )}
        </h2>
        <span className="text-xs text-zinc-500">{sinks.length} output{sinks.length === 1 ? "" : "s"}</span>
      </div>
      {loading && sinks.length === 0 ? (
        <div className="text-zinc-500 text-sm">Loading audio...</div>
      ) : error && sinks.length === 0 ? (
        <div className="text-zinc-500 text-sm">Audio control unavailable.</div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {sinks.map((s) => (
            <SinkCard key={s.name} sink={s} />
          ))}
        </div>
      )}
    </section>
  );
}
