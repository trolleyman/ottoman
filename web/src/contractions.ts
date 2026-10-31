// The frequency corpus removes apostrophes. Restore common literal spellings.
export const COMMON_CONTRACTIONS = [
  "don't", "isn't", "can't", "won't", "it's", "that's", "I'm", "you're", "we're", "they're",
  "doesn't", "didn't", "wasn't", "weren't", "aren't", "couldn't", "wouldn't", "shouldn't",
  "haven't", "hasn't", "hadn't", "mustn't", "needn't", "shan't", "let's", "there's", "here's",
  "what's", "who's", "where's", "when's", "why's", "how's", "he's", "she's", "one's",
  "I've", "you've", "we've", "they've", "I'll", "you'll", "he'll", "she'll", "it'll", "we'll", "they'll",
  "I'd", "you'd", "he'd", "she'd", "we'd", "they'd", "there'll", "there'd", "who'll", "who'd",
  "could've", "should've", "would've", "might've", "must've", "o'clock", "y'all",
] as const;
