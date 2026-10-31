import { COMMON_CONTRACTIONS } from "./contractions";
import words from "norvig-frequencies";
import { createWordPredictor } from "./wordPrediction";
import { createSwipeDecoder } from "./swipeTyping";

// Frequency-ranked English vocabulary, loaded only when swipe typing is enabled.
export const decodeSwipe = createSwipeDecoder([...COMMON_CONTRACTIONS, ...words]);

export const predictWords = createWordPredictor(words);
