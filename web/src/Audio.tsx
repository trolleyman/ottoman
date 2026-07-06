import { Volume1, Volume2, VolumeX } from "lucide-react";
import type { AudioSink } from "./api";
import { Section, SectionState } from "./Section";
import { useStore } from "./store";
import { useCoalescedSlider } from "./useCoalescedSlider";

function VolumeIcon({ muted, volume, className }: { muted: boolean; volume: number; className?: string }) {
  if (muted || volume === 0) return <VolumeX className={className} />;
  if (volume < 0.5) return <Volume1 className={className} />;
  return <Volume2 className={className} />;
}

function SinkCard({ sink }: { sink: AudioSink }) {
  const setSinkVolume = useStore((s) => s.setSinkVolume);
  const setSinkMute = useStore((s) => s.setSinkMute);
  const setSinkDefault = useStore((s) => s.setSinkDefault);

  // Drive the slider in integer-percent units (matching PipeWire's fraction ×100)
  // so drags stay smooth and don't fight the 3s poll — see useCoalescedSlider.
  const { value: pct, set, dragProps } = useCoalescedSlider(
    Math.round(sink.volume * 100),
    (v) => setSinkVolume(sink.name, v / 100),
  );

  // The slider runs 0–150%; 100% is the "normal max" and everything past it is
  // overdrive. 100% therefore sits at 100/150 ≈ 66.67% of the track width.
  const MAX = 150;
  const MARK = (100 / MAX) * 100; // % position of the 100% mark
  const overdrive = pct > 100;
  const fillPct = (Math.min(pct, 100) / MAX) * 100;
  const overdrivePct = (Math.max(pct - 100, 0) / MAX) * 100;

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
        <div className={`flex-1 flex flex-col gap-1 ${sink.muted ? "opacity-40" : ""}`}>
          <div className="relative h-4 flex items-center">
            {/* Track with a distinct overdrive zone (100–150%) */}
            <div className="absolute inset-x-0 h-1.5 rounded-full overflow-hidden bg-zinc-700">
              <div
                className="absolute inset-y-0 right-0 bg-amber-500/25"
                style={{ width: `${100 - MARK}%` }}
              />
            </div>
            {/* Current-value fill: blue up to 100%, amber into overdrive */}
            <div
              className="absolute h-1.5 rounded-full bg-blue-500"
              style={{ width: `${fillPct}%` }}
            />
            {overdrive && (
              <div
                className="absolute h-1.5 bg-amber-500"
                style={{ left: `${MARK}%`, width: `${overdrivePct}%` }}
              />
            )}
            {/* The 100% mark */}
            <div
              className="absolute top-0 bottom-0 w-0.5 -translate-x-1/2 rounded-full bg-zinc-300"
              style={{ left: `${MARK}%` }}
            />
            <input
              type="range"
              min={0}
              max={MAX}
              value={pct}
              {...dragProps}
              onChange={(e) => set(Number(e.target.value))}
              className="volume-slider absolute inset-0"
              style={{ ["--slider-thumb" as string]: overdrive ? "#f59e0b" : "#3b82f6" }}
            />
          </div>
          {/* Tick labels: make the 100% mark and the overdrive range explicit */}
          <div className="relative h-3 text-[10px] font-medium text-zinc-500 select-none">
            <span className="absolute left-0">0</span>
            <span className="absolute -translate-x-1/2 text-zinc-300" style={{ left: `${MARK}%` }}>
              100%
            </span>
            <span className="absolute right-0 text-amber-500/80">150%</span>
          </div>
        </div>
        <span
          className={`text-sm font-mono w-10 text-right tabular-nums ${overdrive ? "text-amber-500" : "text-zinc-400"}`}
        >
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
    <Section
      title="Audio"
      loading={loading && sinks.length > 0}
      meta={sinks.length > 0 ? `${sinks.length} output${sinks.length === 1 ? "" : "s"}` : undefined}
    >
      {loading && sinks.length === 0 ? (
        <SectionState>Loading audio…</SectionState>
      ) : error && sinks.length === 0 ? (
        <SectionState>Audio control unavailable.</SectionState>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {sinks.map((s) => (
            <SinkCard key={s.name} sink={s} />
          ))}
        </div>
      )}
    </Section>
  );
}
