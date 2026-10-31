export interface Point { x: number; y: number }
export type KeyPositions = Record<string, Point>;

export function resample(path: Point[], count = 24): Point[] {
  if (!path.length) return [];
  const distances = [0];
  for (let index = 1; index < path.length; index++)
    distances.push(distances[index - 1] + Math.hypot(path[index].x - path[index - 1].x, path[index].y - path[index - 1].y));
  const length = distances[distances.length - 1];
  if (!length) return Array.from({ length: count }, () => path[0]);
  let segment = 1;
  return Array.from({ length: count }, (_, index) => {
    const target = length * index / (count - 1);
    while (segment < path.length - 1 && distances[segment] < target) segment++;
    const fraction = (target - distances[segment - 1]) / (distances[segment] - distances[segment - 1] || 1);
    return {
      x: path[segment - 1].x + (path[segment].x - path[segment - 1].x) * fraction,
      y: path[segment - 1].y + (path[segment].y - path[segment - 1].y) * fraction,
    };
  });
}

export function createSwipeDecoder(words: readonly string[]) {
  const buckets = new Map<string, { word: string; rank: number }[]>();
  const seen = new Set<string>();
  words.forEach((raw, rank) => {
    const word = raw.toLowerCase();
    if (!/^[a-z]+(?:'[a-z]+)?$/.test(word) || word.length < 2 || word.length > 20 || seen.has(word)) return;
    seen.add(word);
    const key = word[0] + word.at(-1)!;
    const bucket = buckets.get(key) ?? [];
    bucket.push({ word, rank });
    buckets.set(key, bucket);
  });
  return (path: Point[], keys: KeyPositions): string[] => {
    if (path.length < 2) return [];
    const nearest = (point: Point) => Object.entries(keys).sort(([, a], [, b]) =>
      Math.hypot(a.x - point.x, a.y - point.y) - Math.hypot(b.x - point.x, b.y - point.y),
    )[0]?.[0];
    const first = nearest(path[0]);
    const last = nearest(path[path.length - 1]);
    if (!first || !last) return [];
    const samples = resample(path);
    const scale = Math.hypot((keys.q?.x ?? 0) - (keys.w?.x ?? 1), (keys.q?.y ?? 0) - (keys.w?.y ?? 0)) || 1;
    const candidates = buckets.get(first + last) ?? [];
    const ranked = candidates.map(({ word, rank }) => {
      const plain = word.replaceAll("'", "");
      const letters = Array.from(plain).filter((letter, index) => index === 0 || letter !== plain[index - 1]);
      const ideal = resample(letters.map((letter) => keys[letter]));
      // Arc-length sampling removes finger-speed differences. Dynamic alignment
      // tolerates a small pause or a rounded corner near a key.
      let previous = Array<number>(ideal.length + 1).fill(Infinity);
      previous[0] = 0;
      for (const point of samples) {
        const current = [Infinity];
        for (let index = 0; index < ideal.length; index++) {
          const distance = Math.hypot(point.x - ideal[index].x, point.y - ideal[index].y) / scale;
          current.push(distance * distance + Math.min(previous[index], previous[index + 1], current[index]));
        }
        previous = current;
      }
      const score = previous[ideal.length] / samples.length + Math.log1p(rank) * 0.025;
      return { word, score };
    });
    return ranked.sort((a, b) => a.score - b.score).slice(0, 3).map(({ word }) => word);
  };
}
