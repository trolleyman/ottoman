import { useEffect, useRef, useState } from "react";
import { ArrowBigUp, CornerDownLeft, Delete, Settings, Smile, Smartphone, X } from "lucide-react";
import { Modifier } from "./api";

export interface KeyboardSettings {
  numberRow: boolean;
  symbolHints: boolean;
  doubleSpacePeriod: boolean;
  haptics: boolean;
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
    { key: "q", alt: "1" },
    { key: "w", alt: "2" },
    { key: "e", alt: "3" },
    { key: "r", alt: "4" },
    { key: "t", alt: "5" },
    { key: "y", alt: "6" },
    { key: "u", alt: "7" },
    { key: "i", alt: "8" },
    { key: "o", alt: "9" },
    { key: "p", alt: "0" },
  ],
  [
    { key: "a", alt: "@" },
    { key: "s", alt: "#" },
    { key: "d", alt: "$" },
    { key: "f", alt: "%" },
    { key: "g", alt: "&" },
    { key: "h", alt: "-" },
    { key: "j", alt: "+" },
    { key: "k", alt: "(" },
    { key: "l", alt: ")" },
  ],
  [
    { key: "z", alt: "*" },
    { key: "x", alt: '"' },
    { key: "c", alt: "'" },
    { key: "v", alt: ":" },
    { key: "b", alt: ";" },
    { key: "n", alt: "!" },
    { key: "m", alt: "?" },
  ],
];

const SYMBOL_ROWS: KeyDefinition[][] = [
  ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"].map((key) => ({ key })),
  ["@", "#", "$", "%", "&", "-", "+", "(", ")"].map((key) => ({ key })),
  ['"', "'", ":", ";", "!", "?", "/"].map((key) => ({ key })),
];

const EMOJIS = [
  "😀", "😂", "🥰", "😍", "😊", "😉", "😎", "🤔", "😅", "😭",
  "😤", "🥳", "👍", "👎", "👏", "🙏", "💪", "👀", "❤️", "🔥",
  "✨", "🎉", "✅", "❌", "💯", "🚀", "👋", "🤝", "💡", "📌",
];

type ShiftState = "off" | "once" | "locked";
type KeyboardMode = "letters" | "symbols" | "emoji";

const MODIFIER_KEYS = [
  { value: Modifier.CTRL, label: "Ctrl" },
  { value: Modifier.ALT, label: "Alt" },
  { value: Modifier.SHIFT, label: "Shift" },
  { value: Modifier.META, label: "Meta" },
] as const;

export function ModifierBar({
  selected,
  onToggle,
  className = "",
}: {
  selected: Modifier[];
  onToggle: (modifier: Modifier) => void;
  className?: string;
}) {
  return (
    <div className={`flex gap-1.5 ${className}`} aria-label="Sticky modifier keys">
      {MODIFIER_KEYS.map(({ value, label }) => {
        const active = selected.includes(value);
        return (
          <button
            type="button"
            key={value}
            aria-pressed={active}
            onPointerDown={(event) => event.preventDefault()}
            onClick={() => onToggle(value)}
            className={`min-h-8 flex-1 rounded-md border px-2 text-xs font-semibold transition-colors ${
              active
                ? "border-blue-400/60 bg-blue-500 text-white shadow-[0_0_0_2px_rgba(59,130,246,.16)]"
                : "border-zinc-700 bg-zinc-800 text-zinc-400 hover:text-zinc-100"
            }`}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}

function haptic(enabled: boolean, pattern: number | number[] = 8) {
  if (enabled && "vibrate" in navigator) navigator.vibrate(pattern);
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
    }, 430);
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
      className="keyboard-key relative flex min-w-0 flex-1 touch-none select-none items-center justify-center overflow-visible rounded-md border border-white/10 bg-zinc-600/95 text-[clamp(15px,4.5vw,20px)] font-medium text-white shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.35)] active:translate-y-px active:bg-zinc-500"
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
        <span className="absolute right-1 top-0.5 text-[9px] font-semibold text-zinc-300/55">
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
          : "border-white/5 bg-zinc-800 text-zinc-100 active:bg-zinc-700"
      }`}
    >
      {children}
    </button>
  );
}

export function CustomKeyboard({
  open,
  connected,
  settings,
  onText,
  onBackspace,
  onEnter,
  onClose,
  onOpenSettings,
  onUseNative,
  modifiers,
  onToggleModifier,
}: {
  open: boolean;
  connected: boolean;
  settings: KeyboardSettings;
  onText: (text: string) => void;
  onBackspace: () => void;
  onEnter: () => void;
  onClose: () => void;
  onOpenSettings: () => void;
  onUseNative: () => void;
  modifiers: Modifier[];
  onToggleModifier: (modifier: Modifier) => void;
}) {
  const [mode, setMode] = useState<KeyboardMode>("letters");
  const [shift, setShift] = useState<ShiftState>("off");
  const lastShiftTap = useRef(0);
  const backspaceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backspaceRepeat = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => () => {
    if (backspaceTimer.current) clearTimeout(backspaceTimer.current);
    if (backspaceRepeat.current) clearInterval(backspaceRepeat.current);
  }, []);

  if (!open) return null;

  const type = (value: string) => {
    const next = mode === "letters" && shift !== "off" ? value.toUpperCase() : value;
    onText(next);
    if (shift === "once") setShift("off");
  };

  const toggleShift = () => {
    const now = performance.now();
    if (now - lastShiftTap.current < 350) setShift("locked");
    else setShift((current) => current === "off" ? "once" : "off");
    lastShiftTap.current = now;
    haptic(settings.haptics);
  };

  const beginBackspace = () => {
    haptic(settings.haptics);
    onBackspace();
    backspaceTimer.current = setTimeout(() => {
      backspaceRepeat.current = setInterval(onBackspace, 65);
    }, 430);
  };
  const endBackspace = () => {
    if (backspaceTimer.current) clearTimeout(backspaceTimer.current);
    if (backspaceRepeat.current) clearInterval(backspaceRepeat.current);
    backspaceTimer.current = null;
    backspaceRepeat.current = null;
  };

  const rows = mode === "symbols" ? SYMBOL_ROWS : LETTER_ROWS;

  return (
    <div className="fixed inset-x-0 bottom-0 z-50 border-t border-white/10 bg-zinc-950/98 pb-[max(8px,env(safe-area-inset-bottom))] shadow-[0_-16px_40px_rgba(0,0,0,.5)] backdrop-blur-xl md:absolute md:inset-x-auto md:bottom-auto md:left-0 md:top-full md:mt-2 md:w-[min(680px,calc(100vw-3rem))] md:rounded-xl md:border">
      <div className="mx-auto flex max-w-3xl items-center justify-between px-2 py-1.5">
        <div className="flex items-center">
          <button
            type="button"
            onClick={onOpenSettings}
            className="flex h-8 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
          >
            <Settings className="h-3.5 w-3.5" />
            Settings
          </button>
          <button
            type="button"
            onClick={onUseNative}
            className="flex h-8 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
          >
            <Smartphone className="h-3.5 w-3.5" />
            Device
          </button>
        </div>
        <span className="text-[10px] uppercase tracking-[0.16em] text-zinc-600">
          Ottoman input
        </span>
        <button
          type="button"
          aria-label="Close keyboard"
          onClick={onClose}
          className="flex h-8 w-8 items-center justify-center rounded-md text-zinc-400 hover:bg-zinc-800 hover:text-white"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <ModifierBar
        selected={modifiers}
        onToggle={onToggleModifier}
        className="mx-auto max-w-3xl px-2 pb-1.5"
      />

      <div className="mx-auto flex h-[252px] max-w-3xl flex-col gap-1.5 px-1.5 sm:h-[274px] sm:gap-2 sm:px-2">
        {mode === "emoji" ? (
          <div className="grid min-h-0 flex-1 grid-cols-6 gap-1.5 overflow-y-auto rounded-lg bg-zinc-900/70 p-2 sm:grid-cols-10">
            {EMOJIS.map((emoji) => (
              <button
                type="button"
                key={emoji}
                onPointerDown={(event) => event.preventDefault()}
                onClick={() => {
                  haptic(settings.haptics);
                  onText(emoji);
                }}
                className="min-h-11 rounded-lg text-2xl hover:bg-zinc-800 active:bg-zinc-700"
              >
                {emoji}
              </button>
            ))}
          </div>
        ) : (
          <>
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
                {index === 2 && mode === "letters" && (
                  <UtilityKey label="Shift" active={shift !== "off"} wide onPress={toggleShift}>
                    <ArrowBigUp className={`h-5 w-5 ${shift === "locked" ? "fill-current" : ""}`} />
                  </UtilityKey>
                )}
                {row.map((definition) => (
                  <Key
                    key={definition.key}
                    definition={definition}
                    label={mode === "letters" && shift !== "off" ? definition.key.toUpperCase() : undefined}
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
                    className="flex min-w-[52px] flex-[1.4] touch-none select-none items-center justify-center rounded-md border border-white/5 bg-zinc-800 text-zinc-100 shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:bg-zinc-700"
                  >
                    <Delete className="h-5 w-5" />
                  </button>
                )}
              </div>
            ))}
          </>
        )}

        <div className="flex min-h-0 flex-1 gap-1.5 sm:gap-2">
          <UtilityKey label={mode === "symbols" ? "Letters" : "Symbols"} wide onPress={() => setMode(mode === "symbols" ? "letters" : "symbols")}>
            {mode === "symbols" ? "ABC" : "?123"}
          </UtilityKey>
          <UtilityKey label={mode === "emoji" ? "Letters" : "Emoji"} onPress={() => setMode(mode === "emoji" ? "letters" : "emoji")}>
            {mode === "emoji" ? "ABC" : <Smile className="h-5 w-5" />}
          </UtilityKey>
          <button
            type="button"
            aria-label="Space"
            onPointerDown={(event) => event.preventDefault()}
            onClick={() => type(" ")}
            className="flex-[4] touch-none select-none rounded-md border border-white/10 bg-zinc-600/95 text-sm text-zinc-200 shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.35)] active:translate-y-px active:bg-zinc-500"
          >
            space
          </button>
          <UtilityKey label="Period" onPress={() => type(".")}>.</UtilityKey>
          <button
            type="button"
            disabled={!connected}
            aria-label="Enter"
            onPointerDown={(event) => event.preventDefault()}
            onClick={onEnter}
            className="flex min-w-[54px] flex-[1.4] touch-none select-none items-center justify-center rounded-md border border-blue-400/40 bg-blue-500 text-white shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:translate-y-px active:bg-blue-400 disabled:opacity-40"
          >
            <CornerDownLeft className="h-5 w-5" />
          </button>
        </div>
      </div>
    </div>
  );
}
