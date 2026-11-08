// Small, transparent English phrase prior. The current field supplies additional
// evidence at prediction time; no field text is stored or sent to a service.
export const COMMON_PHRASES = [
  "hello world", "hello there", "hi there", "nice to meet you",
  "good morning", "good afternoon", "good evening", "good night", "good luck",
  "thank you", "thank you very much", "thanks for your help", "thanks for letting me know",
  "how are you", "how are things", "how do you", "how can i", "how can we",
  "i would like to", "would you like to", "i want to", "i need to", "i have to",
  "i am going to", "we are going to", "are you going to", "i think that", "i think we",
  "i don't know", "i don't think", "i can see", "i can help", "i will be", "i will send",
  "can you please", "could you please", "let me know", "let us know", "let me check",
  "see you soon", "see you later", "see you tomorrow", "talk to you later",
  "looking forward to", "looking forward to seeing you", "look forward to hearing from you",
  "as soon as possible", "at the end of the day", "in the middle of", "on the other hand",
  "a lot of", "one of the", "some of the", "all of the", "the rest of the",
  "happy birthday", "happy new year", "have a nice day", "have a good day", "have a great weekend",
  "please let me know", "please send me", "please could you", "what do you think",
  "what is the", "where is the", "when will you", "it would be", "that would be great",
  "the keyboard settings", "open the keyboard settings", "move the cursor", "move the window",
  "turn on the", "turn off the", "it works well", "it looks good", "it should be",
  "i love you", "i miss you", "how much is", "how many times", "to be honest",
] as const;

type Counts = Map<string, Map<string, number>>;
const tokens = (text: string) => text.toLowerCase().replaceAll("’", "'").match(/[a-z]+(?:'[a-z]+)?|[.!?\n]/g) ?? [];
function addText(counts: Counts, text: string, weight: number) {
  const history: string[] = [];
  for (const word of tokens(text)) {
    if (/^[.!?\n]$/.test(word)) { history.length = 0; continue; }
    for (let length = 1; length <= Math.min(3, history.length); length++) {
      const key = history.slice(-length).join(" ");
      const next = counts.get(key) ?? new Map<string, number>();
      next.set(word, (next.get(word) ?? 0) + weight);
      counts.set(key, next);
    }
    history.push(word);
  }
}
const prior: Counts = new Map();
for (const phrase of COMMON_PHRASES) addText(prior, phrase, 1);

export function contextualWords(completedText: string): Map<string, number> {
  // Bound work and avoid sharing learned vocabulary between fields or users.
  const recent = completedText.slice(-8192);
  const history = tokens(recent).slice(-3);
  const boundary = history.reduce((last, word, index) => /^[.!?\n]$/.test(word) ? index : last, -1);
  const words = history.slice(boundary + 1);
  const local: Counts = new Map();
  addText(local, recent, 3);
  const scores = new Map<string, number>();
  for (let length = 1; length <= words.length; length++) {
    const key = words.slice(-length).join(" ");
    for (const counts of [prior, local]) for (const [word, count] of counts.get(key) ?? []) {
      scores.set(word, (scores.get(word) ?? 0) + count * Math.pow(8, length));
    }
  }
  return scores;
}
