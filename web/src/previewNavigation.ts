export function previewPositions(root: HTMLElement) {
  const node = root.querySelector("[data-caret-text]")?.firstChild;
  if (!node) return [];
  const value = node.textContent ?? "";
  const range = document.createRange();
  const points: { offset: number; x: number; y: number; height: number }[] = [];
  let offset = 0;
  for (const char of value) {
    range.setStart(node, offset);
    range.setEnd(node, offset + char.length);
    const rect = range.getBoundingClientRect();
    points.push({ offset, x: rect.left, y: rect.top, height: rect.height });
    offset += char.length;
  }
  return points;
}

export function previewHit(root: HTMLElement, x: number, y: number) {
  const points = previewPositions(root);
  return points.reduce((best, point) => {
    const distance = (p: typeof point) => Math.abs(p.y + p.height / 2 - y) * 1000 + Math.abs(p.x - x);
    return !best || distance(point) < distance(best) ? point : best;
  }, undefined as typeof points[number] | undefined)?.offset;
}

export function previewMove(root: HTMLElement, offset: number, key: string, preferredX?: number) {
  const points = previewPositions(root);
  const index = points.findIndex(point => point.offset === offset);
  const current = points[index];
  if (!current) return;
  if (key === "ArrowLeft" || key === "ArrowRight") return { offset: points[Math.max(0, Math.min(points.length - 1, index + (key === "ArrowLeft" ? -1 : 1)))].offset };
  const line = points.filter(point => Math.abs(point.y - current.y) < 2);
  if (key === "Home" || key === "End") return { offset: (key === "Home" ? line[0] : line.at(-1))!.offset };
  const direction = key === "ArrowUp" || key === "PageUp" ? -1 : 1;
  const rows = [...new Set(points.map(point => point.y))].sort((a, b) => a - b);
  const row = rows.findIndex(y => Math.abs(y - current.y) < 2);
  const step = key.startsWith("Page") ? Math.max(1, Math.floor(root.clientHeight / Math.max(1, current.height))) : 1;
  const targetY = rows[Math.max(0, Math.min(rows.length - 1, row + step * direction))];
  const x = preferredX ?? current.x;
  const target = points.filter(point => Math.abs(point.y - targetY) < 2).reduce((best, point) => Math.abs(point.x - x) < Math.abs(best.x - x) ? point : best);
  return { offset: target.offset, x };
}
