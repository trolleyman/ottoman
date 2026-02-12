import type { Layout } from "./types";
import { sortedLayoutMonitors } from "./utils";
import { MiniLayoutPreview } from "./MiniLayoutPreview";

export function LayoutCard({
  layout,
  isCurrent,
  disabled,
  scale,
  onClick,
  onDelete,
}: {
  layout: Layout;
  isCurrent: boolean;
  disabled: boolean;
  scale: number;
  onClick: () => void;
  onDelete?: () => void;
}) {
  const enabled = sortedLayoutMonitors(layout.monitors ?? []);
  const idAliases = [layout.id, ...(layout.aliases ?? [])].join(" \u00b7 ");

  return (
    <div className="relative group mb-auto flex-grow">
      {onDelete && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className="absolute -top-2 -right-2 w-6 h-6 rounded-full bg-zinc-800 border border-zinc-600 text-zinc-400 hover:text-red-400 hover:border-red-400 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-all z-20 shadow-lg cursor-pointer"
          title="Delete layout"
        >
          ×
        </button>
      )}
      <button
        onClick={onClick}
        disabled={disabled}
        className={`
          relative overflow-hidden rounded-xl text-sm font-medium transition-all cursor-pointer p-4 flex flex-col w-full gap-1.5 min-w-[180px] text-left
          ${isCurrent
            ? "bg-gradient-to-br from-blue-500/20 via-blue-500/10 to-indigo-500/20 text-blue-400 border border-blue-500/40 ring-1 ring-blue-500/20"
            : "bg-gradient-to-br from-zinc-800/80 via-zinc-800/50 to-zinc-900/80 text-zinc-300 border border-zinc-700/50 hover:border-zinc-600 hover:from-zinc-800 hover:to-zinc-800/80"
          }
          disabled:opacity-50 disabled:cursor-wait
        `}
      >
        {/* Header row: name left, emoji right */}
        <span className="flex items-start justify-between gap-3 w-full">
          <span className="flex flex-col gap-0.5">
            <span className="flex items-center gap-2">
              {isCurrent && (
                <span className="inline-block w-2 h-2 rounded-full bg-blue-400 shrink-0" />
              )}
              <span className="font-semibold">{layout.name}</span>
            </span>
            <span className="text-[10px] text-zinc-500 font-normal">{idAliases}</span>
          </span>
          {layout.emoji && <span className="text-lg leading-none">{layout.emoji}</span>}
        </span>

        {/* Monitor list */}
        {enabled.length > 0 && (
          <div className="flex flex-col gap-1 mt-2 w-full">
            {enabled.map((m, i) => (
              <div
                key={m.port || m.edid || i}
                className={`grid grid-cols-[auto_1fr_auto] items-center gap-2 text-[11px] px-2 py-1.5 rounded border ${m.primary
                  ? "bg-blue-500/10 border-blue-500/20"
                  : "bg-zinc-900/40 border-transparent"
                  }`}
              >
                <span className="truncate text-zinc-300 font-medium" title={m.name || m.port}>
                  {m.name || m.port || "Unknown"}
                </span>
                <span className="font-mono text-zinc-500">{m.width}x{m.height}</span>
                <span className="font-mono text-zinc-600 text-[10px]">{m.edid}</span>
              </div>
            ))}
          </div>
        )}
      </button>

      {/* Hover preview — below card with notch */}
      {enabled.length > 0 && (
        <div className="hidden sm:block absolute left-1/2 -translate-x-1/2 top-full mt-2 z-10 opacity-0 group-hover:opacity-100 pointer-events-none group-hover:pointer-events-auto transition-opacity duration-150">
          {/* Notch */}
          <div className="mx-auto w-0 h-0 border-l-[6px] border-l-transparent border-r-[6px] border-r-transparent border-b-[6px] border-b-zinc-700" />
          <div className="rounded-xl border border-zinc-700 bg-gradient-to-b from-zinc-900 to-zinc-950 p-3 shadow-2xl">
            <MiniLayoutPreview monitors={layout.monitors} scale={scale} />
          </div>
        </div>
      )}
    </div>
  );
}
