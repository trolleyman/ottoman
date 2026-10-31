import { expect, test } from "bun:test";
import { searchSymbols, SYMBOLS } from "../src/symbolData";

test("symbol lookup includes Greek case/variants, aliases and complete multiword queries", () => {
  for (const symbol of ["θ", "Θ", "ϑ"]) expect(searchSymbols("theta").map(([result]) => result)).toContain(symbol);
  expect(searchSymbols("capital theta").map(([symbol]) => symbol)).toEqual(["Θ"]);
  expect(searchSymbols("θ").map(([symbol]) => symbol)).toEqual(["θ"]);
  expect(searchSymbols("double integral").map(([symbol]) => symbol)).toContain("∬");
  expect(searchSymbols("question mark").map(([symbol]) => symbol)).toContain("?");
  expect(searchSymbols("superscript two").map(([symbol]) => symbol)).toContain("²");
  expect(searchSymbols("e acute").map(([symbol]) => symbol)).toContain("é");
  expect(searchSymbols("u+03b8").map(([symbol]) => symbol)).toEqual(["θ"]);
  expect(searchSymbols("pipe").map(([symbol]) => symbol)).toContain("|");
  expect(searchSymbols("backtick").map(([symbol]) => symbol)).toContain("`");
  expect(searchSymbols("u+d800")).toEqual([]);
  expect(searchSymbols("not a known symbol phrase")).toEqual([]);
});

test("the expanded catalogue contains unique symbols and merges aliases", () => {
  expect(SYMBOLS.length).toBeGreaterThan(300);
  expect(new Set(SYMBOLS.map(([symbol]) => symbol)).size).toBe(SYMBOLS.length);
  expect(searchSymbols("ohm").map(([symbol]) => symbol)).toEqual(["Ω"]);
});
