import { describe, expect, it } from "bun:test";
import { createWordPredictor, currentWord, matchWordCase } from "../src/wordPrediction";

describe("word predictions", () => {
  it("keeps frequency order, excludes exact matches and invalid vocabulary", () => {
    const predict = createWordPredictor(["hello", "help", "hello", "he", "held", "helmet", "he's"]);
    expect(predict("HE")).toEqual(["hello", "help", "held"]);
    expect(predict("hello")).toEqual([]);
    expect(predict("")).toEqual([]);
  });
  it("only completes the current local word and preserves its case", () => {
    expect(currentWord("hello, wor")).toBe("wor");
    expect(currentWord("hello ")).toBe("");
    expect(matchWordCase("world", "Wor")).toBe("World");
    expect(matchWordCase("world", "WOR")).toBe("WORLD");
  });
});
