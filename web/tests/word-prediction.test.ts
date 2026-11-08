import { describe, expect, it } from "bun:test";
import { createWordPredictor, currentWord, matchWordCase } from "../src/wordPrediction";

describe("word predictions", () => {
  it("keeps frequency order, excludes exact matches and invalid vocabulary", () => {
    const predict = createWordPredictor(["hello", "help", "hello", "he", "held", "helmet", "he's"]);
    expect(predict("HEL")).toEqual(["HELLO", "HELP", "HELD"]);
    expect(predict("hello")).toEqual([]);
    expect(predict("")).toEqual(["I", "The", "It"]);
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

 it("uses several preceding words and capitalizes completions", () => {
   const predict = createWordPredictor(["you", "your", "like", "later", "the"]);
   expect(predict("how are y")[0]).toBe("you");
   expect(predict("i would l")[0]).toBe("like");
   expect(predict("i would like ")[0]).toBe("to");
   expect(predict("Thank Y")[0]).toBe("You");
   expect(predict("THANK ")[0]).toBe("YOU");
   expect(predict("how are you. l")).toEqual(predict("l"));
 });

 it("uses repeated phrases in this field without retaining another field's text", () => {
   const predict = createWordPredictor(["the", "turquoise", "tiles"]);
   expect(predict("we paint turquoise tiles. we paint t")[0]).toBe("turquoise");
   expect(predict("we paint turquoise tiles. we paint turquoise ")[0]).toBe("tiles");
   expect(predict("we paint t")[0]).toBe("the");
 });

it("keeps next-word choices populated after spaces and sentence boundaries", () => {
  const predict = createWordPredictor(["the", "and", "to", "hello"]);
  expect(predict("thank ")[0]).toBe("you");
  expect(predict("unfamiliarword ")).toEqual(["the", "and", "to"]);
  expect(predict("sentence. ")).toEqual(["I", "The", "It"]);
});
