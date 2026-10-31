import { ActionKey } from "./KeyboardKeys";
import { useRef, useState } from "react";
import { Apple, Car, CircleDot, Clock3, Flag, Hand, Hash, Leaf, Lightbulb, Smile } from "lucide-react";
import { EMOJI_GROUPS, withSkinTone } from "./emojiData";

const GROUP_ICONS = [Smile, Hand, Leaf, Apple, Car, CircleDot, Lightbulb, Hash, Flag];
const TONE_COLORS = ["#eab308", "#ffdbac", "#edb98a", "#c68642", "#8d5524", "#54331d"];
const TONES = ["", "🏻", "🏼", "🏽", "🏾", "🏿"];

export function EmojiPicker({ recent, onSelect }: {
  recent: string[];
  onSelect: (emoji: string) => void;
}) {
  const [category, setCategory] = useState(-1);
  const [tone, setTone] = useState("");
  const [tonePickerOpen, setTonePickerOpen] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  const sections = useRef<(HTMLElement | null)[]>([]);
  const jumpTo = (index: number) => {
    const root = scroller.current;
    const section = sections.current[index + 1];
    if (!root || !section) return;
    root.scrollTo({ top: root.scrollTop + section.getBoundingClientRect().top - root.getBoundingClientRect().top, behavior: "smooth" });
  };
  return (
    <div className="custom-keyboard-emoji-grid flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg bg-white/[0.02] p-1.5">
      <div className="relative flex shrink-0 gap-1">
      <nav aria-label="Emoji categories" className="keyboard-scroll flex min-w-0 flex-1 gap-1 overflow-x-auto">
        {[{ name: "Recent emoji", Icon: Clock3 }, ...EMOJI_GROUPS.map((group, index) => ({ name: group.name, Icon: GROUP_ICONS[index] }))].map(({ name, Icon }, index) => (
          <ActionKey key={name} type="button" aria-label={name} aria-current={category === index - 1 ? "location" : undefined}
            onPress={() => jumpTo(index - 1)}
            className={`flex h-8 min-w-8 flex-1 items-center justify-center rounded ${category === index - 1 ? "bg-blue-500/20 text-blue-300" : "text-zinc-500 hover:bg-zinc-700/40 hover:text-zinc-300"}`}>
            <Icon className="h-4 w-4" />
          </ActionKey>
        ))}
      </nav>
      <ActionKey type="button" aria-label="Choose skin tone" aria-expanded={tonePickerOpen} onPress={() => setTonePickerOpen((open) => !open)} className="flex h-8 w-7 shrink-0 items-center justify-center rounded hover:bg-zinc-700/40">
        <Hand className="h-4 w-4" style={{ color: TONE_COLORS[TONES.indexOf(tone)] }} />
      </ActionKey>
      {tonePickerOpen && <div className="absolute right-0 top-9 z-50 flex gap-1 rounded-lg border border-zinc-600 bg-zinc-900 p-1.5 shadow-xl" aria-label="Emoji skin tone">
        {TONES.map((item, index) => (
          <ActionKey type="button" key={index} aria-label={index === 0 ? "Default skin tone" : `Skin tone ${index}`}
            aria-pressed={tone === item} onPress={() => { setTone(item); setTonePickerOpen(false); }}
            className={`flex h-7 w-7 items-center justify-center rounded ${tone === item ? "bg-blue-500/30 ring-1 ring-blue-400" : "hover:bg-zinc-700/40"}`}>
            <Hand className="h-4 w-4" style={{ color: TONE_COLORS[index] }} />
          </ActionKey>
        ))}
      </div>}
      </div>
      <div ref={scroller} role="region" aria-label="All emoji" className="keyboard-scroll min-h-0 flex-1 overflow-y-auto"
        onScroll={() => {
          const top = scroller.current!.getBoundingClientRect().top;
          let current = -1;
          sections.current.forEach((section, index) => { if (section && section.getBoundingClientRect().top <= top + 12) current = index - 1; });
          setCategory(current);
        }}>
        <section ref={(node) => { sections.current[0] = node; }} aria-label="Recent emoji">
          <h3 className="px-1 py-1.5 text-[10px] font-semibold text-zinc-400">Recent</h3>
          <div className="grid auto-rows-[36px] grid-cols-8 gap-1 sm:grid-cols-10">
            {recent.map((emoji) => <ActionKey type="button" key={emoji} onPress={() => onSelect(emoji)} className="rounded-md text-xl hover:bg-zinc-700/30">{emoji}</ActionKey>)}
          </div>
          {!recent.length && <p className="px-1 pb-2 text-xs text-zinc-500">Your recent emoji will appear here.</p>}
        </section>
        {EMOJI_GROUPS.map((group, index) => (
          <section key={group.slug} ref={(node) => { sections.current[index + 1] = node; }} aria-label={group.name}>
            <h3 className="px-1 py-1.5 text-[10px] font-semibold text-zinc-400">{group.name}</h3>
            <div className="grid auto-rows-[36px] grid-cols-8 gap-1 sm:grid-cols-10">
              {group.emojis.map((item) => {
                const emoji = withSkinTone(item.emoji, tone, item.skin_tone_support);
                return <ActionKey type="button" key={item.emoji} aria-label={item.name} title={item.name} onPress={() => onSelect(emoji)} className="rounded-md text-xl hover:bg-zinc-700/30 active:scale-90">{emoji}</ActionKey>;
              })}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}
