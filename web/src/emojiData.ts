import groups from "unicode-emoji-json/data-by-group.json";

export const EMOJI_GROUPS = groups;
export const EMOJIS = groups.flatMap((group) => group.emojis.map(({ emoji, name, slug }) => [
  emoji,
  `${name} ${slug.replaceAll("_", " ")} ${/heart|kiss/.test(name) ? "love" : ""} ${/grinning|smiling/.test(name) ? "happy smile" : ""}`,
] as const));

export function withSkinTone(emoji: string, tone: string, supported: boolean): string {
  if (!tone || !supported) return emoji;
  const characters = Array.from(emoji);
  if (characters[1] === "\uFE0F") characters.splice(1, 1);
  characters.splice(1, 0, tone);
  return characters.join("");
}
