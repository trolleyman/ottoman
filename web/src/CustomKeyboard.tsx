import { useEffect, useRef, useState } from "react";
import {
  ArrowBigUp,
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowRightToLine,
  ArrowUp,
  ChevronUp,
  Clock3,
  Command,
  CornerDownLeft,
  Delete,
  Move,
  Option,
  Search,
  Settings,
  Smile,
  X,
} from "lucide-react";
import { Modifier } from "./api";

export interface KeyboardSettings {
  numberRow: boolean;
  symbolHints: boolean;
  doubleSpacePeriod: boolean;
  haptics: boolean;
  longPressDelayMs: number;
  spaceHoldDelayMs: number;
  backspaceRepeatDelayMs: number;
  backspaceRepeatIntervalMs: number;
  shiftDoubleTapMs: number;
  previewFadeMs: number;
}

interface KeyDefinition {
  key: string;
  alt?: string;
  grow?: number;
}

const NUMBER_ROW: KeyDefinition[] = [
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

const LETTER_ROWS: KeyDefinition[][] = [
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

const SYMBOL_ROWS: KeyDefinition[][] = [
  ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"].map((key) => ({ key })),
  ["@", "#", "£", "$", "%", "&", "-", "+", "(", ")"].map((key) => ({ key })),
  ['"', "'", ":", ";", "!", "?", "/"].map((key) => ({ key })),
];

const SYMBOLS = [
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

const EMOJIS = [
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

const STARTER_RECENTS = ["😂", "❤️", "👍", "🙏", "🔥", "🎉"];

function loadRecentEmoji(): string[] {
  try {
    const saved = localStorage.getItem("ottoman-recent-emoji");
    return saved ? JSON.parse(saved) as string[] : STARTER_RECENTS;
  } catch {
    return STARTER_RECENTS;
  }
}

type ShiftState = "off" | "once" | "locked";
type KeyboardMode = "letters" | "symbols" | "emoji";

function WindowsIcon({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" className={className}>
      <path d="M3 5.4 11 4.3v7.2H3V5.4Zm9.2-1.3L21 2.9v8.6h-8.8V4.1ZM3 12.7h8v7.2l-8-1.1v-6.1Zm9.2 0H21v8.6l-8.8-1.2v-7.4Z" />
    </svg>
  );
}

function ModifierButton({
  modifier,
  label,
  text,
  selected,
  onToggle,
  children,
}: {
  modifier: Modifier;
  label: string;
  text: string;
  selected: Modifier[];
  onToggle: (modifier: Modifier) => void;
  children: React.ReactNode;
}) {
  const active = selected.includes(modifier);
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={active}
      onPointerDown={(event) => event.preventDefault()}
      onClick={() => onToggle(modifier)}
      className={`flex h-9 min-w-10 flex-1 items-center justify-center gap-2 rounded-md border px-1 transition-colors ${
        active
          ? "border-blue-400/60 bg-blue-500 text-white shadow-[0_0_0_2px_rgba(59,130,246,.16)]"
          : "custom-keyboard-utility border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
      }`}
    >
      {children}
      <span className="text-[10px] font-semibold tracking-wide">{text}</span>
    </button>
  );
}

function haptic(enabled: boolean, pattern: number | number[] = 8) {
  if (!enabled) return;
  if ("vibrate" in navigator && navigator.vibrate(pattern)) return;

  // Safari 18+ gives `<input type="checkbox" switch>` a native Taptic tap.
  // Trigger one from the same user gesture as the key press. Older iOS builds
  // ignore the non-standard switch attribute, which is a safe silent fallback.
  if (/iPhone|iPad|iPod/.test(navigator.userAgent)) {
    const id = "ottoman-haptic-switch";
    let pulse = document.getElementById(id) as HTMLInputElement | null;
    if (!pulse) {
      pulse = document.createElement("input");
      pulse.id = id;
      pulse.type = "checkbox";
      pulse.setAttribute("switch", "");
      pulse.setAttribute("aria-hidden", "true");
      pulse.tabIndex = -1;
      Object.assign(pulse.style, {
        position: "fixed",
        width: "1px",
        height: "1px",
        opacity: "0",
        pointerEvents: "none",
      });
      document.body.appendChild(pulse);
    }
    pulse.click();
  }
}

function Key({
  definition,
  label,
  settings,
  onType,
}: {
  definition: KeyDefinition;
  label?: string;
  settings: KeyboardSettings;
  onType: (value: string, wasLongPress: boolean) => void;
}) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const held = useRef(false);
  const [preview, setPreview] = useState(false);

  const clearTimer = () => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  };

  const start = (event: React.PointerEvent<HTMLButtonElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId);
    held.current = false;
    if (!definition.alt) return;
    timer.current = setTimeout(() => {
      held.current = true;
      setPreview(true);
      haptic(settings.haptics, 16);
      onType(definition.alt!, true);
    }, settings.longPressDelayMs);
  };

  const finish = () => {
    clearTimer();
    setPreview(false);
    if (held.current) {
      held.current = false;
      return;
    }
    haptic(settings.haptics);
    onType(definition.key, false);
  };

  return (
    <button
      type="button"
      className="custom-keyboard-key relative flex min-w-0 flex-1 touch-none select-none items-center justify-center overflow-visible rounded-md border border-white/10 bg-zinc-600/95 text-[clamp(15px,4.5vw,20px)] font-medium text-white shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.35)] active:translate-y-px active:bg-zinc-500"
      style={definition.grow ? { flexGrow: definition.grow } : undefined}
      aria-label={definition.alt ? `${definition.key}, hold for ${definition.alt}` : definition.key}
      onPointerDown={start}
      onPointerUp={finish}
      onPointerCancel={() => {
        clearTimer();
        setPreview(false);
        held.current = false;
      }}
      onContextMenu={(event) => event.preventDefault()}
    >
      <span>{label ?? definition.key}</span>
      {settings.symbolHints && definition.alt && (
        <span className="custom-keyboard-hint absolute right-1 top-0.5 text-[9px] font-semibold text-zinc-300/55">
          {definition.alt}
        </span>
      )}
      {preview && (
        <span className="absolute -top-12 left-1/2 z-10 flex h-12 w-10 -translate-x-1/2 items-center justify-center rounded-lg border border-white/20 bg-zinc-500 text-xl shadow-xl">
          {definition.alt}
        </span>
      )}
    </button>
  );
}

function UtilityKey({
  children,
  label,
  active = false,
  wide = false,
  onPress,
}: {
  children: React.ReactNode;
  label: string;
  active?: boolean;
  wide?: boolean;
  onPress: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onPointerDown={(event) => event.preventDefault()}
      onClick={onPress}
      className={`flex touch-none select-none items-center justify-center rounded-md border text-sm font-semibold shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:translate-y-px ${
        wide ? "min-w-[52px] flex-[1.4]" : "min-w-0 flex-1"
      } ${
        active
          ? "border-blue-400/50 bg-blue-500 text-white"
          : "custom-keyboard-utility border-white/5 bg-zinc-800 text-zinc-100 active:bg-zinc-700"
      }`}
    >
      {children}
    </button>
  );
}

function NavigationKey({
  label,
  children,
  onPress,
}: {
  label: string;
  children: React.ReactNode;
  onPress: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onPointerDown={(event) => event.preventDefault()}
      onClick={onPress}
      className="custom-keyboard-utility flex h-8 min-w-0 flex-1 touch-none select-none items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 px-1 text-[10px] font-semibold text-zinc-400 shadow-sm hover:bg-zinc-800 hover:text-zinc-100 active:translate-y-px"
    >
      {children}
    </button>
  );
}

function SettingSwitch({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-label={label}
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`relative h-5 w-9 shrink-0 rounded-full border transition-colors ${
        checked
          ? "border-blue-400/60 bg-blue-500"
          : "border-zinc-600 bg-zinc-700"
      }`}
    >
      <span
        className={`absolute left-0 top-0.5 h-3.5 w-3.5 rounded-full bg-white shadow transition-transform ${
          checked ? "translate-x-[18px]" : "translate-x-0.5"
        }`}
      />
    </button>
  );
}

function TimingField({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="custom-keyboard-utility flex min-h-10 items-center justify-between gap-2 rounded-md border border-white/5 bg-zinc-800/70 px-3 text-xs text-zinc-300">
      <span>{label}</span>
      <span className="flex items-center gap-1 text-zinc-500">
        <input
          type="number"
          inputMode="numeric"
          aria-label={`${label} milliseconds`}
          value={value}
          min={min}
          max={max}
          step={step}
          onChange={(event) => {
            const next = Number(event.target.value);
            if (Number.isFinite(next)) onChange(Math.min(max, Math.max(min, next)));
          }}
          className="h-7 w-16 appearance-none rounded border border-zinc-600 bg-zinc-950 px-1.5 text-right text-xs text-zinc-200 outline-none focus:border-blue-500 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
        />
        ms
      </span>
    </label>
  );
}

export function CustomKeyboard({
  open,
  connected,
  settings,
  onText,
  onBackspace,
  onEnter,
  onTab,
  onNavigate,
  onClose,
  onSettingsChange,
  onUseCustomKeyboard,
  onUseDeviceKeyboard,
  usingCustomKeyboard,
  settingsPageOpen = false,
  modifiers,
  onToggleModifier,
}: {
  open: boolean;
  connected: boolean;
  settings: KeyboardSettings;
  onText: (text: string, shifted?: boolean) => void;
  onBackspace: () => void;
  onEnter: (shifted: boolean) => void;
  onTab: () => void;
  onNavigate: (key: string) => void;
  onClose: () => void;
  onSettingsChange: (patch: Partial<KeyboardSettings>) => void;
  onUseCustomKeyboard: () => void;
  onUseDeviceKeyboard: () => void;
  usingCustomKeyboard: boolean;
  settingsPageOpen?: boolean;
  modifiers: Modifier[];
  onToggleModifier: (modifier: Modifier) => void;
}) {
  const [mode, setMode] = useState<KeyboardMode>("letters");
  const [shift, setShift] = useState<ShiftState>("off");
  const [emojiSearching, setEmojiSearching] = useState(false);
  const [symbolSearching, setSymbolSearching] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(settingsPageOpen);
  const [emojiQuery, setEmojiQuery] = useState("");
  const [symbolQuery, setSymbolQuery] = useState("");
  const [recentEmoji, setRecentEmoji] = useState<string[]>(loadRecentEmoji);
  const lastShiftTap = useRef(0);
  const backspaceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backspaceRepeat = useRef<ReturnType<typeof setInterval> | null>(null);
  const spaceHoldTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const spaceHeld = useRef(false);
  const spaceLast = useRef({ x: 0, y: 0 });
  const [spaceNavigating, setSpaceNavigating] = useState(false);
  const applePlatform = typeof navigator !== "undefined" &&
    /Mac|iPhone|iPad|iPod/.test(navigator.userAgent);

  useEffect(() => () => {
    if (backspaceTimer.current) clearTimeout(backspaceTimer.current);
    if (backspaceRepeat.current) clearInterval(backspaceRepeat.current);
    if (spaceHoldTimer.current) clearTimeout(spaceHoldTimer.current);
  }, []);

  if (!open) return null;

  const type = (value: string) => {
    if (mode === "emoji" && emojiSearching) {
      setEmojiQuery((current) => `${current}${value.toLocaleLowerCase()}`);
      haptic(settings.haptics);
      return;
    }
    if (mode === "symbols" && symbolSearching) {
      setSymbolQuery((current) => `${current}${value.toLocaleLowerCase()}`);
      haptic(settings.haptics);
      return;
    }
    const shifted = (mode === "letters" || symbolSearching) && shift !== "off";
    const next = shifted ? value.toUpperCase() : value;
    onText(next, shifted);
    if (shift === "once") {
      setShift("off");
      if (modifiers.includes(Modifier.SHIFT)) onToggleModifier(Modifier.SHIFT);
    }
  };

  const toggleShift = () => {
    const now = performance.now();
    const next: ShiftState = now - lastShiftTap.current < settings.shiftDoubleTapMs
      ? "locked"
      : shift === "off" ? "once" : "off";
    setShift(next);
    if (modifiers.includes(Modifier.SHIFT) !== (next !== "off"))
      onToggleModifier(Modifier.SHIFT);
    lastShiftTap.current = now;
    haptic(settings.haptics);
  };

  const beginBackspace = () => {
    haptic(settings.haptics);
    const erase = () => {
      if (mode === "emoji" && emojiSearching)
        setEmojiQuery((current) => Array.from(current).slice(0, -1).join(""));
      else if (mode === "symbols" && symbolSearching)
        setSymbolQuery((current) => Array.from(current).slice(0, -1).join(""));
      else onBackspace();
    };
    erase();
    backspaceTimer.current = setTimeout(() => {
      backspaceRepeat.current = setInterval(erase, settings.backspaceRepeatIntervalMs);
    }, settings.backspaceRepeatDelayMs);
  };
  const endBackspace = () => {
    if (backspaceTimer.current) clearTimeout(backspaceTimer.current);
    if (backspaceRepeat.current) clearInterval(backspaceRepeat.current);
    backspaceTimer.current = null;
    backspaceRepeat.current = null;
  };

  const beginSpace = (event: React.PointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    spaceHeld.current = false;
    spaceLast.current = { x: event.clientX, y: event.clientY };
    spaceHoldTimer.current = setTimeout(() => {
      spaceHeld.current = true;
      setSpaceNavigating(true);
      haptic(settings.haptics, 16);
    }, settings.spaceHoldDelayMs);
  };

  const moveSpace = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (!spaceHeld.current) return;
    const step = 18;
    const dx = event.clientX - spaceLast.current.x;
    const dy = event.clientY - spaceLast.current.y;
    const horizontalSteps = Math.trunc(dx / step);
    const verticalSteps = Math.trunc(dy / step);
    const emit = (key: string, count: number) => {
      for (let i = 0; i < Math.min(Math.abs(count), 6); i++) onNavigate(key);
    };
    if (horizontalSteps !== 0) {
      emit(horizontalSteps > 0 ? "ArrowRight" : "ArrowLeft", horizontalSteps);
      spaceLast.current.x += horizontalSteps * step;
    }
    if (verticalSteps !== 0) {
      emit(verticalSteps > 0 ? "ArrowDown" : "ArrowUp", verticalSteps);
      spaceLast.current.y += verticalSteps * step;
    }
  };

  const endSpace = () => {
    if (spaceHoldTimer.current) clearTimeout(spaceHoldTimer.current);
    spaceHoldTimer.current = null;
    if (!spaceHeld.current) {
      haptic(settings.haptics);
      type(" ");
    }
    spaceHeld.current = false;
    setSpaceNavigating(false);
  };

  const cancelSpace = () => {
    if (spaceHoldTimer.current) clearTimeout(spaceHoldTimer.current);
    spaceHoldTimer.current = null;
    spaceHeld.current = false;
    setSpaceNavigating(false);
  };

  const navigate = (key: string) => {
    haptic(settings.haptics);
    onNavigate(key);
  };

  const rows = mode === "symbols" && !symbolSearching ? SYMBOL_ROWS : LETTER_ROWS;
  const matchingEmoji = EMOJIS.filter(([, keywords]) =>
    !emojiQuery.trim() || keywords.includes(emojiQuery.trim().toLocaleLowerCase()),
  );
  const matchingSymbols = SYMBOLS.filter(([, keywords]) =>
    !symbolQuery.trim() || keywords.includes(symbolQuery.trim().toLocaleLowerCase()),
  );

  const selectSymbol = (symbol: string) => {
    haptic(settings.haptics);
    onText(symbol);
  };

  const selectEmoji = (emoji: string) => {
    haptic(settings.haptics);
    onText(emoji);
    const next = [emoji, ...recentEmoji.filter((item) => item !== emoji)].slice(0, 12);
    setRecentEmoji(next);
    localStorage.setItem("ottoman-recent-emoji", JSON.stringify(next));
  };

  return (
    <div className="custom-keyboard-shell fixed inset-x-0 bottom-0 z-50 border-t border-white/10 bg-zinc-950/98 pb-[max(8px,env(safe-area-inset-bottom))] shadow-[0_-16px_40px_rgba(0,0,0,.5)] backdrop-blur-xl md:absolute md:inset-x-auto md:bottom-auto md:left-0 md:top-full md:mt-2 md:w-[min(680px,calc(100vw-3rem))] md:rounded-xl md:border">
      <div className="mx-auto flex max-w-3xl gap-1 px-2 pt-1.5" aria-label="Navigation keys">
        <NavigationKey label="Escape" onPress={() => navigate("Escape")}>Esc</NavigationKey>
        <NavigationKey label="Home" onPress={() => navigate("Home")}>Home</NavigationKey>
        <NavigationKey label="Arrow left" onPress={() => navigate("ArrowLeft")}>
          <ArrowLeft className="h-4 w-4" />
        </NavigationKey>
        <NavigationKey label="Arrow up" onPress={() => navigate("ArrowUp")}>
          <ArrowUp className="h-4 w-4" />
        </NavigationKey>
        <NavigationKey label="Arrow down" onPress={() => navigate("ArrowDown")}>
          <ArrowDown className="h-4 w-4" />
        </NavigationKey>
        <NavigationKey label="Arrow right" onPress={() => navigate("ArrowRight")}>
          <ArrowRight className="h-4 w-4" />
        </NavigationKey>
        <NavigationKey label="End" onPress={() => navigate("End")}>End</NavigationKey>
      </div>
      <div className="mx-auto flex max-w-3xl items-center gap-1.5 px-2 py-1.5" aria-label="Keyboard controls">
        <ModifierButton
          modifier={Modifier.CTRL}
          label="Control"
          text="Ctrl"
          selected={modifiers}
          onToggle={onToggleModifier}
        >
          <ChevronUp className="h-4 w-4" />
        </ModifierButton>
        {applePlatform ? (
          <>
            <ModifierButton modifier={Modifier.ALT} label="Option" text="Option" selected={modifiers} onToggle={onToggleModifier}>
              <Option className="h-4 w-4" />
            </ModifierButton>
            <ModifierButton modifier={Modifier.META} label="Command" text="Cmd" selected={modifiers} onToggle={onToggleModifier}>
              <Command className="h-4 w-4" />
            </ModifierButton>
          </>
        ) : (
          <>
            <ModifierButton modifier={Modifier.META} label="Windows" text="Win" selected={modifiers} onToggle={onToggleModifier}>
              <WindowsIcon className="h-[17px] w-[17px]" />
            </ModifierButton>
            <ModifierButton modifier={Modifier.ALT} label="Alt" text="Alt" selected={modifiers} onToggle={onToggleModifier}>
              <Option className="h-4 w-4" />
            </ModifierButton>
          </>
        )}
        <button
          type="button"
          aria-label="Keyboard settings"
          title="Keyboard settings"
          aria-pressed={settingsOpen}
          onClick={() => setSettingsOpen((current) => !current)}
          className={`custom-keyboard-utility flex h-9 min-w-10 flex-1 items-center justify-center rounded-md border ${
            settingsOpen
              ? "border-blue-400/60 bg-blue-500 text-white"
              : "border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
          }`}
        >
          <Settings className="h-4 w-4" />
        </button>
        <button
          type="button"
          aria-label="Close keyboard"
          title="Close keyboard"
          onClick={() => {
            setSettingsOpen(false);
            onClose();
          }}
          className="custom-keyboard-utility flex h-9 min-w-10 flex-1 items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-white"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="mx-auto flex h-[252px] max-w-3xl flex-col gap-1.5 px-1.5 sm:h-[274px] sm:gap-2 sm:px-2">
        {settingsOpen ? (
          <div className="custom-keyboard-emoji-grid flex min-h-0 flex-1 flex-col overflow-y-auto rounded-lg bg-zinc-900/70 p-3">
            <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
              Keyboard settings
            </div>
            <div className="mb-3 grid grid-cols-2 rounded-lg bg-zinc-950 p-1 text-xs">
              <button
                type="button"
                onClick={onUseCustomKeyboard}
                className={`rounded-md px-2 py-2 ${usingCustomKeyboard ? "bg-zinc-700 text-white" : "text-zinc-400 hover:bg-zinc-800"}`}
                aria-pressed={usingCustomKeyboard}
              >
                Ottoman
              </button>
              <button
                type="button"
                onClick={onUseDeviceKeyboard}
                className={`rounded-md px-2 py-2 ${!usingCustomKeyboard ? "bg-zinc-700 text-white" : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"}`}
                aria-pressed={!usingCustomKeyboard}
              >
                Device
              </button>
            </div>
            <div className="grid grid-cols-1 gap-1 sm:grid-cols-2 sm:gap-x-4">
              {([
                ["Number row", "numberRow"],
                ["Long-press symbols", "symbolHints"],
                ["Double-space period", "doubleSpacePeriod"],
                ["Key haptics", "haptics"],
              ] as const).map(([label, key]) => (
                <div
                  key={key}
                  className="custom-keyboard-utility flex min-h-10 items-center justify-between gap-3 rounded-md border border-white/5 bg-zinc-800/70 px-3 text-xs text-zinc-300"
                >
                  {label}
                  <SettingSwitch
                    label={label}
                    checked={settings[key]}
                    onChange={(checked) => onSettingsChange({ [key]: checked })}
                  />
                </div>
              ))}
            </div>
            <div className="mb-1 mt-4 text-[11px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
              Timing
            </div>
            <div className="grid grid-cols-1 gap-1 sm:grid-cols-2 sm:gap-x-4">
              <TimingField label="Symbol hold" value={settings.longPressDelayMs} min={150} max={1000} step={25} onChange={(value) => onSettingsChange({ longPressDelayMs: value })} />
              <TimingField label="Space cursor hold" value={settings.spaceHoldDelayMs} min={150} max={1000} step={25} onChange={(value) => onSettingsChange({ spaceHoldDelayMs: value })} />
              <TimingField label="Backspace repeat" value={settings.backspaceRepeatDelayMs} min={150} max={1000} step={25} onChange={(value) => onSettingsChange({ backspaceRepeatDelayMs: value })} />
              <TimingField label="Repeat interval" value={settings.backspaceRepeatIntervalMs} min={25} max={250} step={5} onChange={(value) => onSettingsChange({ backspaceRepeatIntervalMs: value })} />
              <TimingField label="Shift double-tap" value={settings.shiftDoubleTapMs} min={200} max={800} step={25} onChange={(value) => onSettingsChange({ shiftDoubleTapMs: value })} />
              <TimingField label="Preview fade" value={settings.previewFadeMs} min={1000} max={10000} step={250} onChange={(value) => onSettingsChange({ previewFadeMs: value })} />
            </div>
            <p className="mt-3 text-[11px] leading-4 text-zinc-500">
              Device uses the phone's native keyboard. Tap the small settings button on the trackpad to return here.
            </p>
          </div>
        ) : (
          <>
        {mode === "emoji" && !emojiSearching ? (
          <div className="custom-keyboard-emoji-grid flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg bg-zinc-900/70 p-1.5">
            <button
              type="button"
              onClick={() => setEmojiSearching(true)}
              className="custom-keyboard-utility mb-1.5 flex h-8 shrink-0 items-center gap-2 rounded-md border border-zinc-700 bg-zinc-800 px-2.5 text-left text-xs text-zinc-400"
            >
              <Search className="h-3.5 w-3.5" />
              Search Emoji
            </button>
            <div className="mb-1 flex shrink-0 items-center gap-1 overflow-x-auto">
              <Clock3 className="mx-1 h-3.5 w-3.5 shrink-0 text-zinc-500" aria-label="Recent emoji" />
              {recentEmoji.map((emoji) => (
                <button
                  type="button"
                  key={emoji}
                  onPointerDown={(event) => event.preventDefault()}
                  onClick={() => selectEmoji(emoji)}
                  className="h-9 w-9 shrink-0 rounded-md text-xl hover:bg-zinc-800 active:bg-zinc-700"
                >
                  {emoji}
                </button>
              ))}
            </div>
            <div className="grid min-h-0 flex-1 auto-rows-[36px] grid-cols-8 content-start gap-0.5 overflow-y-auto sm:grid-cols-10">
              {EMOJIS.map(([emoji]) => (
                <button
                  type="button"
                  key={emoji}
                  onPointerDown={(event) => event.preventDefault()}
                  onClick={() => selectEmoji(emoji)}
                  className="rounded-md text-xl hover:bg-zinc-800 active:bg-zinc-700"
                >
                  {emoji}
                </button>
              ))}
            </div>
          </div>
        ) : (
          <>
            {mode === "emoji" && emojiSearching && (
              <div className="flex h-11 shrink-0 items-center gap-1.5 overflow-hidden rounded-md bg-zinc-900/70 px-2">
                <Search className="h-3.5 w-3.5 shrink-0 text-zinc-500" />
                <span className="min-w-12 flex-1 truncate text-sm text-zinc-200">
                  {emojiQuery || <span className="text-zinc-500">Search Emoji</span>}
                </span>
                {matchingEmoji.slice(0, 5).map(([emoji]) => (
                  <button
                    type="button"
                    key={emoji}
                    onClick={() => selectEmoji(emoji)}
                    className="h-8 w-8 shrink-0 rounded text-lg hover:bg-zinc-800"
                  >
                    {emoji}
                  </button>
                ))}
              </div>
            )}
            {mode === "symbols" && !symbolSearching && (
              <button
                type="button"
                onClick={() => setSymbolSearching(true)}
                className="custom-keyboard-utility flex h-8 shrink-0 items-center gap-2 rounded-md border border-zinc-700 bg-zinc-800 px-2.5 text-left text-xs text-zinc-400"
              >
                <Search className="h-3.5 w-3.5" />
                Search Symbols
              </button>
            )}
            {mode === "symbols" && symbolSearching && (
              <div className="flex h-11 shrink-0 items-center gap-1.5 overflow-hidden rounded-md bg-zinc-900/70 px-2">
                <Search className="h-3.5 w-3.5 shrink-0 text-zinc-500" />
                <span className="min-w-12 flex-1 truncate text-sm text-zinc-200">
                  {symbolQuery || <span className="text-zinc-500">Search Symbols</span>}
                </span>
                {matchingSymbols.slice(0, 6).map(([symbol]) => (
                  <button
                    type="button"
                    key={symbol}
                    aria-label={`Type ${symbol}`}
                    onClick={() => selectSymbol(symbol)}
                    className="h-8 w-8 shrink-0 rounded text-lg text-zinc-100 hover:bg-zinc-800"
                  >
                    {symbol}
                  </button>
                ))}
              </div>
            )}
            {settings.numberRow && mode === "letters" && (
              <div className="flex min-h-0 flex-1 gap-1 sm:gap-1.5">
                {NUMBER_ROW.map((definition) => (
                  <Key key={definition.key} definition={definition} settings={settings} onType={type} />
                ))}
              </div>
            )}
            {rows.map((row, index) => (
              <div
                key={index}
                className={`flex min-h-0 flex-1 gap-1 sm:gap-1.5 ${index === 1 ? "px-[4.5%]" : ""}`}
              >
                {index === 2 && (mode === "letters" || symbolSearching) && (
                  <UtilityKey label="Shift" active={shift !== "off"} wide onPress={toggleShift}>
                    <ArrowBigUp className={`h-5 w-5 ${shift === "locked" ? "fill-current" : ""}`} />
                  </UtilityKey>
                )}
                {index === 2 && mode === "emoji" && emojiSearching && (
                  <div className="custom-keyboard-utility flex min-w-[52px] flex-[1.4] items-center justify-center rounded-md border border-white/5 bg-zinc-800 text-zinc-500">
                    <Search className="h-4 w-4" />
                  </div>
                )}
                {row.map((definition) => (
                  <Key
                    key={definition.key}
                    definition={definition}
                    label={(mode === "letters" || symbolSearching) && shift !== "off" ? definition.key.toUpperCase() : undefined}
                    settings={settings}
                    onType={type}
                  />
                ))}
                {index === 2 && (
                  <button
                    type="button"
                    aria-label="Backspace"
                    onPointerDown={(event) => {
                      event.preventDefault();
                      beginBackspace();
                    }}
                    onPointerUp={endBackspace}
                    onPointerCancel={endBackspace}
                    onPointerLeave={endBackspace}
                    className="custom-keyboard-utility flex min-w-[52px] flex-[1.4] touch-none select-none items-center justify-center rounded-md border border-white/5 bg-zinc-800 text-zinc-100 shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:bg-zinc-700"
                  >
                    <Delete className="h-5 w-5" />
                  </button>
                )}
              </div>
            ))}
          </>
        )}

        <div className="flex h-12 shrink-0 gap-1.5 sm:gap-2">
          <UtilityKey
            label={emojiSearching ? "Back to emoji" : symbolSearching ? "Back to symbols" : mode === "symbols" ? "Letters" : "Symbols"}
            wide
            onPress={() => {
              if (emojiSearching) setEmojiSearching(false);
              else if (symbolSearching) setSymbolSearching(false);
              else setMode(mode === "symbols" ? "letters" : "symbols");
            }}
          >
            {emojiSearching || symbolSearching ? <ArrowLeft className="h-5 w-5" /> : mode === "symbols" ? "ABC" : "?123"}
          </UtilityKey>
          <UtilityKey
            label={mode === "emoji" ? "Letters" : "Emoji"}
            onPress={() => {
              if (mode === "emoji") {
                setMode("letters");
                setEmojiSearching(false);
              } else {
                setMode("emoji");
                setSymbolSearching(false);
              }
            }}
          >
            {mode === "emoji" ? "ABC" : <Smile className="h-5 w-5" />}
          </UtilityKey>
          <UtilityKey label="Tab" onPress={onTab}>
            <ArrowRightToLine className="h-5 w-5" />
          </UtilityKey>
          <button
            type="button"
            aria-label="Space. Hold and drag to move the cursor"
            onPointerDown={beginSpace}
            onPointerMove={moveSpace}
            onPointerUp={endSpace}
            onPointerCancel={cancelSpace}
            onContextMenu={(event) => event.preventDefault()}
            className={`custom-keyboard-key flex-[4] touch-none select-none rounded-md border text-sm shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.35)] active:translate-y-px ${
              spaceNavigating
                ? "border-blue-400/60 bg-zinc-700/95 text-blue-100 ring-2 ring-inset ring-blue-400/15"
                : "border-white/10 bg-zinc-600/95 text-zinc-200 active:bg-zinc-500"
            }`}
          >
            {spaceNavigating ? (
              <span className="flex items-center justify-center gap-2 font-medium tracking-wide">
                <Move className="h-4 w-4 text-blue-300" />
                cursor
              </span>
            ) : "space"}
          </button>
          <UtilityKey label="Period" onPress={() => type(".")}>.</UtilityKey>
          <button
            type="button"
            disabled={!connected}
            aria-label="Enter"
            onPointerDown={(event) => event.preventDefault()}
            onClick={() => {
              if (emojiSearching && matchingEmoji[0]) selectEmoji(matchingEmoji[0][0]);
              else if (symbolSearching && matchingSymbols[0]) selectSymbol(matchingSymbols[0][0]);
              else onEnter(shift !== "off");
            }}
            className="flex min-w-[54px] flex-[1.4] touch-none select-none items-center justify-center rounded-md border border-blue-400/40 bg-blue-500 text-white shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:translate-y-px active:bg-blue-400 disabled:opacity-40"
          >
            <CornerDownLeft className="h-5 w-5" />
          </button>
        </div>
          </>
        )}
      </div>
    </div>
  );
}
