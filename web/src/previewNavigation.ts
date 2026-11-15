type PreviewPoint = { offset: number; x: number; y: number; height: number };
const geometry = new WeakMap<HTMLElement, { node: Node; value: string; width: number; height: number; font: string; status: string; scrollTop: number; points: PreviewPoint[] }>();

export function previewPositions(root: HTMLElement) {
  const node = root.querySelector("[data-caret-text]")?.firstChild;
  if (!node) return [];
  const value = node.textContent ?? "";
  const bounds = root.getBoundingClientRect();
  const font = getComputedStyle(node.parentElement!).font;
  const cached = geometry.get(root);
  const status = document.fonts.status;
  if (cached && cached.node === node && cached.value === value && cached.width === bounds.width && cached.height === bounds.height && cached.font === font && cached.status === status && cached.scrollTop === root.scrollTop) {
    return cached.points.map(point => ({ ...point, x: point.x + bounds.left, y: point.y + bounds.top }));
  }
  const range = document.createRange();
  const points: PreviewPoint[] = [];
  let offset = 0;
  for (const { segment: char } of new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(value)) {
    range.setStart(node, offset);
    range.setEnd(node, offset + char.length);
    const rect = range.getBoundingClientRect();
    points.push({ offset, x: rect.left, y: rect.top, height: rect.height });
    offset += char.length;
  }
  geometry.set(root, { node, value, width: bounds.width, height: bounds.height, font, status, scrollTop: root.scrollTop, points: points.map(point => ({ ...point, x: point.x - bounds.left, y: point.y - bounds.top })) });
  return points;
}

export function previewHit(root: HTMLElement, x: number, y: number) {
  const node = root.querySelector("[data-caret-text]")?.firstChild;
  const doc = root.ownerDocument;
  const position = doc.caretPositionFromPoint?.(x, y);
  if (node && position && position.offsetNode === node) return Math.min(position.offset, (node.textContent?.length ?? 1) - 1);
  const webkit = doc as Document & { caretRangeFromPoint?: (x: number, y: number) => Range | null };
  const range = webkit.caretRangeFromPoint?.(x, y);
  if (node && range && range.startContainer === node) return Math.min(range.startOffset, (node.textContent?.length ?? 1) - 1);
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
  if (key === "Home") return { offset: line[0].offset };
  if (key === "End") {
    const last = line.at(-1)!;
    const text = root.querySelector("[data-caret-text]")?.textContent ?? "";
    const offset = text[last.offset] === "\n" || text[last.offset] === "\u200b" ? last.offset : points[points.indexOf(last) + 1]?.offset ?? last.offset;
    return { offset };
  }
  const direction = key === "ArrowUp" || key === "PageUp" ? -1 : 1;
  const rows = [...new Set(points.map(point => point.y))].sort((a, b) => a - b);
  const row = rows.findIndex(y => Math.abs(y - current.y) < 2);
  const step = key.startsWith("Page") ? Math.max(1, Math.floor((root.closest("[data-preview-scroll]")?.clientHeight ?? root.clientHeight) / Math.max(1, current.height))) : 1;
  const targetY = rows[Math.max(0, Math.min(rows.length - 1, row + step * direction))];
  const x = preferredX ?? current.x;
  const target = points.filter(point => Math.abs(point.y - targetY) < 2).reduce((best, point) => Math.abs(point.x - x) < Math.abs(best.x - x) ? point : best);
  return { offset: target.offset, x };
}
