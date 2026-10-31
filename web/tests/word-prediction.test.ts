import { describe, expect, it } from "bun:test";
import { createWordPredictor, currentWord, matchWordCase } from "../src/wordPrediction";

describe("word predictions", () => {
  it("keeps frequency order, excludes exact matches and invalid vocabulary", () => {
    const predict = createWordPredictor(["hello", "help", "hello", "he", "held", "helmet", "he's"]);
    expect(predict("HEL")).toEqual(["hello", "help", "held"]);
    expect(predict("hello")).toEqual([]);
    expect(predict("")).toEqual([]);
  });
  it("restores common contractions with or without a typed apostrophe", () => {
    const predict = createWordPredictor(["done", "isnt", "cannot", "island"]);
    expect(predict("don")).toContain("don't");
    expect(predict("isn")).toContain("isn't");
    expect(predict("don’")).toContain("don't");
    expect(predict("dont")).toContain("don't");
    expect(currentWord("hello, isn'")).toBe("isn'");
    expect(currentWord("hello, don't")).toBe("don't");
    expect(matchWordCase("isn't", "ISN")).toBe("ISN'T");
    expect(matchWordCase("i'm", "im")).toBe("I'm");
  });
  it("only completes the current local word and preserves its case", () => {
    expect(currentWord("hello, wor")).toBe("wor");
    expect(currentWord("hello ")).toBe("");
    expect(matchWordCase("world", "Wor")).toBe("World");
    expect(matchWordCase("world", "WOR")).toBe("WORLD");
  });
});
