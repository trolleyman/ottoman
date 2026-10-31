import { ActionKey } from "./KeyboardKeys";
import { useState } from "react";
import { EMOJI_GROUPS, withSkinTone } from "./emojiData";

const GROUP_ICONS = ["😀", "👋", "🌿", "🍎", "🚗", "⚽", "💡", "🔣", "🏳️"];
const TONES = ["", "🏻", "🏼", "🏽", "🏾", "🏿"];

export function EmojiPicker({ recent, onSelect }: {
  recent: string[];
  onSelect: (emoji: string) => void;
}) {
  const [category, setCategory] = useState(0);
  const [tone, setTone] = useState("");
  const group = EMOJI_GROUPS[category];
  return (
    <div className="custom-keyboard-emoji-grid flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg bg-zinc-900/70 p-1.5">
      <div role="tablist" aria-label="Emoji categories" className="flex shrink-0 gap-0.5 overflow-x-auto">
        <ActionKey type="button" role="tab" aria-selected={category === -1} aria-label="Recent emoji" onPress={() => setCategory(-1)}
          className={`h-8 min-w-8 flex-1 rounded text-lg ${category === -1 ? "bg-blue-500/25" : "hover:bg-zinc-700/40"}`}>◷</ActionKey>
        {EMOJI_GROUPS.map((item, index) => (
          <ActionKey key={item.slug} type="button" role="tab" aria-label={item.name} aria-selected={category === index}
            onPress={() => setCategory(index)}
            className={`h-8 min-w-8 flex-1 rounded text-lg ${category === index ? "bg-blue-500/25" : "hover:bg-zinc-700/40"}`}>
            {GROUP_ICONS[index]}
          </ActionKey>
        ))}
      </div>
      <div className="flex shrink-0 items-center justify-between px-1 py-1">
        <span className="text-[10px] font-semibold text-zinc-400">{group?.name ?? "Recent emoji"}</span>
        <div className="flex gap-1" aria-label="Emoji skin tone">
          {TONES.map((item, index) => (
            <ActionKey type="button" key={index} aria-label={index === 0 ? "Default skin tone" : `Skin tone ${index}`}
              aria-pressed={tone === item} onPress={() => setTone(item)}
              className={`h-5 w-5 rounded-full text-xs ${tone === item ? "bg-blue-500/30 ring-1 ring-blue-400" : "hover:bg-zinc-700/40"}`}>
              {item ? `👋${item}` : "👋"}
            </ActionKey>
          ))}
        </div>
      </div>
      <div key={category} role="tabpanel" aria-label={group?.name ?? "Recent emoji"}
        className="grid min-h-0 flex-1 auto-rows-[36px] grid-cols-8 content-start gap-0.5 overflow-y-auto overscroll-contain sm:grid-cols-10">
        {group ? group.emojis.map((item) => {
          const emoji = withSkinTone(item.emoji, tone, item.skin_tone_support);
          return <ActionKey type="button" key={item.emoji} aria-label={item.name} title={item.name}
            onPress={() => onSelect(emoji)}
            className="rounded-md text-xl hover:bg-zinc-700/30 active:scale-90">{emoji}</ActionKey>;
        }) : recent.map((emoji) => (
          <ActionKey type="button" key={emoji} onPress={() => onSelect(emoji)}
            className="rounded-md text-xl hover:bg-zinc-700/30 active:scale-90">{emoji}</ActionKey>
        ))}
        {!group && recent.length === 0 && <p className="col-span-8 px-2 text-xs text-zinc-500">Your recent emoji will appear here.</p>}
      </div>
    </div>
  );
}
