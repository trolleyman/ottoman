import { COMMON_CONTRACTIONS } from "./contractions";

// Vocabulary arrives in frequency order; each prefix stores only its best choices.
export function createWordPredictor(words: readonly string[]) {
  const prefixes = new Map<string, string[]>();
  for (const raw of words) {
    const word = raw.toLowerCase();
    if (!/^[a-z]+(?:'[a-z]+)?$/.test(word) || word.length < 2 || word.length > 20) continue;
    for (let length = 1; length < word.length; length++) {
      const prefix = word.slice(0, length);
      const candidates = prefixes.get(prefix) ?? [];
      if (candidates.length === 3 || candidates.includes(word)) continue;
      candidates.push(word);
      prefixes.set(prefix, candidates);
    }
  }
  return (prefix: string) => {
    const normalized = prefix.toLowerCase().replaceAll("’", "'");
    if (!normalized) return [];
    const plain = normalized.replaceAll("'", "");
    const contractions = plain.length >= 2 ? COMMON_CONTRACTIONS.filter((word) =>
      word.toLowerCase() !== normalized && word.toLowerCase().replaceAll("'", "").startsWith(plain),
    ).map((word) => word.toLowerCase()).sort((a, b) => a.length - b.length) : [];
    const direct = contractions.filter((word) => word.startsWith(normalized) || word.replaceAll("'", "") === plain);
    return [...new Set([...direct.slice(0, normalized.includes("'") ? 3 : 1), ...(prefixes.get(normalized) ?? []), ...contractions])].slice(0, 3);
  };
}

export function currentWord(text: string): string {
  return text.match(/[a-z]+(?:['’][a-z]*)?$/i)?.[0] ?? "";
}

export function matchWordCase(word: string, prefix: string): string {
  if (prefix.length > 1 && prefix === prefix.toUpperCase()) return word.toUpperCase();
  if (/^i'/.test(word)) return `I${word.slice(1)}`;
  return /^[A-Z]/.test(prefix) ? word[0].toUpperCase() + word.slice(1) : word;
}
