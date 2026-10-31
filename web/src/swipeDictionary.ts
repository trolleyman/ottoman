import words from "norvig-frequencies";
import { createSwipeDecoder } from "./swipeTyping";

// Frequency-ranked English vocabulary, loaded only when swipe typing is enabled.
export const decodeSwipe = createSwipeDecoder(words);
