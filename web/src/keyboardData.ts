export interface KeyDefinition {
  key: string;
  alt?: string;
  grow?: number;
}

export const NUMBER_ROW: KeyDefinition[] = [
  { key: "1", alt: "!" },
  { key: "2", alt: "@" },
  { key: "3", alt: "#" },
  { key: "4", alt: "$" },
  { key: "5", alt: "%" },
  { key: "6", alt: "^" },
  { key: "7", alt: "&" },
  { key: "8", alt: "*" },
  { key: "9", alt: "(" },
  { key: "0", alt: ")" },
];

export const LETTER_ROWS: KeyDefinition[][] = [
  [
    { key: "q", alt: "\\" },
    { key: "w", alt: "|" },
    { key: "e", alt: "€" },
    { key: "r", alt: "®" },
    { key: "t", alt: "™" },
    { key: "y", alt: "¥" },
    { key: "u", alt: "_" },
    { key: "i", alt: "[" },
    { key: "o", alt: "]" },
    { key: "p", alt: "=" },
  ],
  [
    { key: "a", alt: "~" },
    { key: "s", alt: "<" },
    { key: "d", alt: ">" },
    { key: "f", alt: "{" },
    { key: "g", alt: "}" },
    { key: "h", alt: "-" },
    { key: "j", alt: "+" },
    { key: "k", alt: ":" },
    { key: "l", alt: ";" },
  ],
  [
    { key: "z", alt: "`" },
    { key: "x", alt: "×" },
    { key: "c", alt: "¢" },
    { key: "v", alt: "/" },
    { key: "b", alt: '"' },
    { key: "n", alt: "'" },
    { key: "m", alt: "?" },
  ],
];

export const SYMBOL_ROWS: KeyDefinition[][] = [
  ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"].map((key) => ({ key })),
  ["@", "#", "£", "$", "%", "&", "-", "+", "(", ")"].map((key) => ({ key })),
  ['"', "'", ":", ";", "!", "?", "/"].map((key) => ({ key })),
];

export const SYMBOLS = [
  ["£", "pound sterling currency british gbp"], ["€", "euro currency"],
  ["$", "dollar currency usd"], ["¥", "yen yuan currency"], ["¢", "cent currency"],
  ["©", "copyright"], ["®", "registered trademark"], ["™", "trademark"],
  ["°", "degree temperature angle"], ["±", "plus minus"], ["×", "multiply times"],
  ["÷", "divide division"], ["≈", "approximately equal"], ["≠", "not equal"],
  ["≤", "less than or equal"], ["≥", "greater than or equal"], ["∞", "infinity"],
  ["√", "square root"], ["π", "pi"], ["µ", "micro micron"], ["Ω", "omega ohm"],
  ["→", "right arrow"], ["←", "left arrow"], ["↑", "up arrow"], ["↓", "down arrow"],
  ["•", "bullet dot"], ["…", "ellipsis dots"], ["§", "section"], ["¶", "paragraph"],
  ["†", "dagger"], ["‡", "double dagger"], ["✓", "check tick"], ["—", "em dash"],
  ["–", "en dash"], ["№", "number numero"], ["@", "at sign"], ["#", "hash pound number"],
] as const;

export const EMOJIS = [
  ["😀", "grinning happy smile"], ["😂", "laugh tears funny"],
  ["🥰", "love hearts smiling"], ["😍", "heart eyes love"],
  ["😊", "smile blush happy"], ["😉", "wink playful"],
  ["😎", "cool sunglasses"], ["🤔", "thinking wonder"],
  ["😅", "sweat smile nervous"], ["😭", "cry sob sad"],
  ["😤", "angry steam"], ["🥳", "party celebrate"],
  ["👍", "thumbs up yes good"], ["👎", "thumbs down no bad"],
  ["👏", "clap applause"], ["🙏", "pray thanks please"],
  ["💪", "strong muscle"], ["👀", "eyes look"],
  ["❤️", "red heart love"], ["🔥", "fire hot"],
  ["✨", "sparkles magic"], ["🎉", "party popper celebrate"],
  ["✅", "check done yes"], ["❌", "cross no wrong"],
  ["💯", "hundred perfect"], ["🚀", "rocket launch"],
  ["👋", "wave hello goodbye"], ["🤝", "handshake deal"],
  ["💡", "idea light bulb"], ["📌", "pin location"],
] as const;

export const STARTER_RECENTS = ["😂", "❤️", "👍", "🙏", "🔥", "🎉"];

