import type { Layout, LayoutMonitor, Monitor } from "./api";

export async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return (await res.json()) as T;
}

export async function postJSON<TRequest, TResponse>(url: string, data: TRequest): Promise<TResponse> {
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return (await res.json()) as TResponse;
}

/** Format a display scale factor as a percentage label (e.g. 1.5 -> "150%").
 *  Returns null for an unset (0) or 100% scale, which needs no badge. */
export function formatScalePercent(scale: number | undefined): string | null {
  if (!scale || Math.abs(scale - 1) < 1e-6) return null;
  return `${Math.round(scale * 100)}%`;
}

/** A monitor's logical rectangle for previews. Positions are already logical
 *  (the backend normalises them); physical width/height are divided by the scale
 *  so a 200% monitor occupies half the space, and multi-monitor layouts line up.
 *  Layouts saved before scale existed default to scale 1 (physical == logical). */
export function logicalMonitorRect(m: { position_x: number; position_y: number; width: number; height: number; scale?: number }): { x: number; y: number; w: number; h: number } {
  const s = m.scale && m.scale > 0 ? m.scale : 1;
  return { x: m.position_x, y: m.position_y, w: m.width / s, h: m.height / s };
}

/** Compute a uniform scale that fits all layouts into the same coordinate space */
export function computeUniformScale(layouts: Layout[], maxW: number, maxH: number): number {
  let globalMaxW = 0;
  let globalMaxH = 0;
  for (const layout of layouts) {
    if ((layout.monitors ?? []).length === 0) continue;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const m of layout.monitors) {
      const r = logicalMonitorRect(m);
      minX = Math.min(minX, r.x);
      minY = Math.min(minY, r.y);
      maxX = Math.max(maxX, r.x + r.w);
      maxY = Math.max(maxY, r.y + r.h);
    }
    globalMaxW = Math.max(globalMaxW, maxX - minX);
    globalMaxH = Math.max(globalMaxH, maxY - minY);
  }
  if (globalMaxW <= 0 || globalMaxH <= 0) return 1;
  return Math.min(maxW / globalMaxW, maxH / globalMaxH);
}

/** Sort layouts: #, if one of the aliases is a number, then by ID */
export function sortedLayouts(layouts: Layout[]): Layout[] {
  return [...layouts].sort((a, b) => {
    const aNum = a.aliases?.find((alias) => !isNaN(Number(alias)));
    const bNum = b.aliases?.find((alias) => !isNaN(Number(alias)));
    if (aNum && bNum) {
      const aNumVal = Number(aNum);
      const bNumVal = Number(bNum);
      if (aNumVal !== bNumVal) return aNumVal - bNumVal;
    }
    if (a.id !== b.id) return a.id.localeCompare(b.id);
    return 0;
  });
}

/** Sort monitors: active first, then left-to-right, top-to-bottom */
export function sortedMonitors(monitors: Monitor[]): Monitor[] {
  return [...monitors].sort((a, b) => {
    // Active monitors before inactive
    if (!!a.active !== !!b.active) return a.active ? -1 : 1;
    const ax = a.active?.position_x ?? 0;
    const bx = b.active?.position_x ?? 0;
    if (ax !== bx) return ax - bx;
    return (a.active?.position_y ?? 0) - (b.active?.position_y ?? 0);
  });
}

/** Sort layout monitors: left-to-right, top-to-bottom */
export function sortedLayoutMonitors(monitors: LayoutMonitor[]): LayoutMonitor[] {
  return [...monitors].sort((a, b) => {
    if (a.position_x !== b.position_x) return a.position_x - b.position_x;
    return a.position_y - b.position_y;
  });
}
