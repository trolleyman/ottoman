import type { LayoutMonitor } from "./types";

export function MiniLayoutPreview({ monitors, scale }: { monitors: LayoutMonitor[]; scale: number }) {
  if (monitors.length === 0) return null;

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const m of monitors) {
    minX = Math.min(minX, m.position_x);
    minY = Math.min(minY, m.position_y);
    maxX = Math.max(maxX, m.position_x + m.width);
    maxY = Math.max(maxY, m.position_y + m.height);
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
        const x = (m.position_x - minX) * scale;
        const y = (m.position_y - minY) * scale;
        const w = m.width * scale;
        const h = m.height * scale;

        return (
          <div
            key={m.edid || m.port || i}
            className={`absolute rounded border ${m.primary ? `bg-blue-500/30 border-blue-500/40` : `border-zinc-600 bg-zinc-700/60`} overflow-hidden`}
            style={{ left: x, top: y, width: w, height: h }}
          >
            {m.primary && <>
              <span className="absolute top-0.5 right-1 leading-none text-blue-400 text-[7pt]">
                primary
              </span>
            </>}
            <span className="absolute top-0.5 left-1 text-zinc-400 font-mono leading-none text-[7pt]">
              {m.position_x},{m.position_y}
            </span>
            <span className="absolute bottom-0.5 left-1 text-zinc-400 font-mono leading-none text-[7pt]">
              {m.width}x{m.height}
            </span>
            <span className="absolute bottom-0.5 right-1 text-zinc-500 leading-none text-[7pt]">
              {m.edid}
            </span>
            <span className="absolute inset-0 flex items-center justify-center text-zinc-200 font-medium leading-none text-[10pt]">
              {m.name}
            </span>
          </div>
        );
      })}
    </div>
  );
}
