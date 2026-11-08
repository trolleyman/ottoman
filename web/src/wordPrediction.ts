import { contextualWords } from "./phrasePrediction";
import { COMMON_CONTRACTIONS } from "./contractions";

// Vocabulary arrives in frequency order; each prefix stores only its best choices.
export function createWordPredictor(words: readonly string[], correctWord?: (word: string) => string) {
  const prefixes = new Map<string, string[]>();
  const commonWords: string[] = [];
  for (const raw of words) {
    const word = raw.toLowerCase();
    if (!/^[a-z]+(?:'[a-z]+)?$/.test(word) || word.length < 2 || word.length > 20) continue;
    if (commonWords.length < 3 && !commonWords.includes(word)) commonWords.push(word);
    for (let length = 1; length < word.length; length++) {
      const prefix = word.slice(0, length);
      const candidates = prefixes.get(prefix) ?? [];
      if (candidates.length === 3 || candidates.includes(word)) continue;
      candidates.push(word);
      prefixes.set(prefix, candidates);
    }
  }
  return (textBeforeCursor: string) => {
    const prefix = currentWord(textBeforeCursor);
    const completed = textBeforeCursor.slice(0, textBeforeCursor.length - prefix.length);
    const context = contextualWords(completed);
    const normalized = prefix.toLowerCase().replaceAll("’", "'");
    const sentenceStart = !completed.trim() || /[.!?\n]\s*$/.test(completed);
    if (!normalized && sentenceStart) return ["I", "The", "It"];
    const vocabulary = new Map<string, number>();
    if (normalized) for (const match of completed.slice(-8192).matchAll(/[a-z]+(?:['’][a-z]+)?/gi)) {
      const word = match[0].toLowerCase().replaceAll("’", "'");
      if (word.length >= 2 && word.length <= 20 && word !== normalized && word.startsWith(normalized)) vocabulary.set(word, (vocabulary.get(word) ?? 0) + 1);
    }
    const plain = normalized.replaceAll("'", "");
    const contractions = plain.length >= 2 ? COMMON_CONTRACTIONS.filter((word) =>
      word.toLowerCase() !== normalized && word.toLowerCase().replaceAll("'", "").startsWith(plain),
    ).map((word) => word.toLowerCase()).sort((a, b) => a.length - b.length) : [];
    const direct = contractions.filter((word) => word.startsWith(normalized) || word.replaceAll("'", "") === plain);
    const candidates = [...new Set([...vocabulary.keys(), ...context.keys()].filter(word => word.startsWith(normalized) && word !== normalized).concat(direct.slice(0, normalized.includes("'") ? 3 : 1), normalized ? prefixes.get(normalized) ?? [] : commonWords, contractions))];
    // Phrase evidence wins; personal vocabulary breaks frequency-only ties.
    const score = (word: string) => (context.get(word) ?? 0) + Math.min(7, vocabulary.get(word) ?? 0);
    candidates.sort((a, b) => score(b) - score(a));
    if (!candidates.length && normalized.length >= 4 && correctWord) {
      const corrected = correctWord(prefix);
      if (corrected.toLowerCase() !== normalized) candidates.push(corrected);
    }
    return candidates.slice(0, 3).map(word => {
      if (prefix) return matchWordCase(word, prefix);
      if (word === "i" || word.startsWith("i'")) return "I" + word.slice(1);
      const sentence = completed.split(/[.!?\n]/).at(-1) ?? "";
      return /[A-Z]/.test(sentence) && !/[a-z]/.test(sentence) ? word.toUpperCase() : word;
    });
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

// Keep the suggestion strip useful during dictionary loading or offline failures.
export const predictCommonWords = createWordPredictor(["the", "and", "to", "you", "that", "is", "for", "with", "this", "have", "not", "are", "can", "will", "your", "be", "we", "it", "of", "in"]);
