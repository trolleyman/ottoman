import type { LayoutMonitor } from "./api";
import { formatScalePercent, logicalMonitorRect } from "./utils";

export function MiniLayoutPreview({ monitors, scale }: { monitors: LayoutMonitor[]; scale: number }) {
  if (monitors.length === 0) return null;

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const m of monitors) {
    const r = logicalMonitorRect(m);
    minX = Math.min(minX, r.x);
    minY = Math.min(minY, r.y);
    maxX = Math.max(maxX, r.x + r.w);
    maxY = Math.max(maxY, r.y + r.h);
  }

  const totalW = maxX - minX;
  const totalH = maxY - minY;
  if (totalW <= 0 || totalH <= 0) return null;

  const scaledW = totalW * scale;
  const scaledH = totalH * scale;

  return (
    <div
      className="relative mx-auto"
      style={{ width: scaledW, height: scaledH }}
    >
      {monitors.map((m, i) => {
        const r = logicalMonitorRect(m);
        const x = (r.x - minX) * scale;
        const y = (r.y - minY) * scale;
        const w = r.w * scale;
        const h = r.h * scale;

        // Labels live in constrained rows rather than free-floating in the
        // corners, so they shrink or drop out instead of overlapping each other.
        // Boxes get small easily here: a 200% monitor occupies half its physical
        // size, and the whole preview is scaled down to fit.
        const scalePct = formatScalePercent(m.scale);
        const showPosition = w >= 90;
        const showEdid = w >= 160;
        const showName = h >= 46;

        return (
          <div
            key={m.edid || m.port || i}
            className={`absolute rounded border ${m.primary ? `bg-blue-500/30 border-blue-500/40` : `border-zinc-600 bg-zinc-700/60`} overflow-hidden`}
            style={{ left: x, top: y, width: w, height: h }}
          >
            {showName && (
              <div className="absolute inset-0 flex items-center justify-center overflow-hidden px-1 py-3.5">
                <span className="line-clamp-2 text-center break-words text-zinc-200 font-medium leading-tight text-[10pt]">
                  {m.name}
                </span>
              </div>
            )}

            <div className="absolute inset-x-1 top-0.5 flex items-start justify-between gap-1 leading-none text-[7pt]">
              {showPosition ? (
                <span className="truncate font-mono text-zinc-400">
                  {m.position_x},{m.position_y}
                </span>
              ) : (
                <span />
              )}
              {m.primary && <span className="shrink-0 text-blue-400">primary</span>}
            </div>

            <div className="absolute inset-x-1 bottom-0.5 flex items-end justify-between gap-1 leading-none text-[7pt]">
              {/* Resolution is the most useful label, so it never shrinks. */}
              <span className="shrink-0 font-mono text-zinc-400">
                {m.width}x{m.height}{scalePct ? ` @${scalePct}` : ""}
              </span>
              {showEdid && (
                <span className="truncate font-mono text-zinc-500">{m.edid}</span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
