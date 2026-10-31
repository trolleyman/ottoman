// Vocabulary arrives in frequency order; each prefix stores only its best choices.
export function createWordPredictor(words: readonly string[]) {
  const prefixes = new Map<string, string[]>();
  for (const raw of words) {
    const word = raw.toLowerCase();
    if (!/^[a-z]{2,20}$/.test(word)) continue;
    for (let length = 1; length < word.length; length++) {
      const prefix = word.slice(0, length);
      const candidates = prefixes.get(prefix) ?? [];
      if (candidates.length === 3 || candidates.includes(word)) continue;
      candidates.push(word);
      prefixes.set(prefix, candidates);
    }
  }
  return (prefix: string) => prefixes.get(prefix.toLowerCase()) ?? [];
}

export function currentWord(text: string): string {
  return text.match(/[a-z]+$/i)?.[0] ?? "";
}

export function matchWordCase(word: string, prefix: string): string {
  if (prefix.length > 1 && prefix === prefix.toUpperCase()) return word.toUpperCase();
  return /^[A-Z]/.test(prefix) ? word[0].toUpperCase() + word.slice(1) : word;
}
