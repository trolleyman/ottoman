import { lazy, Suspense, useEffect, useRef, useState } from "react";
import {
  ArrowBigUp,
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowRightToLine,
  ArrowUp,
  ChevronUp,
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
import { type KeyboardSettings } from "./trackpadSettings";
import { keyboardRows, resolveKeyboardLayout, SYMBOLS } from "./keyboardData";
import { useSwipeTyping } from "./useSwipeTyping";
const EmojiPicker = lazy(() => import("./EmojiPicker").then((module) => ({ default: module.EmojiPicker })));
import { loadRecentEmoji, savePreference } from "./preferences";
import { ActionKey, WindowsIcon, ModifierButton, Key, UtilityKey, NavigationKey } from "./KeyboardKeys";
import { haptic } from "./keyboardHaptics";
import { KeyboardSettingsPanel } from "./KeyboardSettingsPanel";

type ShiftState = "off" | "once" | "locked";
type KeyboardMode = "letters" | "symbols" | "emoji";

function KeyboardSession({
  connected,
  remoteOS,
  textContext,
  settings,
  onText,
  onReplaceText,
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
  connected: boolean;
  remoteOS: string;
  textContext: number;
  settings: KeyboardSettings;
  onText: (text: string, shifted?: boolean) => void;
  onReplaceText: (previous: string, next: string) => void;
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
  const [shift, setShift] = useState<ShiftState>(() =>
    modifiers.includes(Modifier.SHIFT) ? "once" : "off",
  );
  const [emojiSearching, setEmojiSearching] = useState(false);
  const [symbolSearching, setSymbolSearching] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(settingsPageOpen);
  const [emojiQuery, setEmojiQuery] = useState("");
  const [symbolQuery, setSymbolQuery] = useState("");
  const [recentEmoji, setRecentEmoji] = useState<string[]>(loadRecentEmoji);
  const lastShiftTap = useRef(-Infinity);
  const backspaceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backspaceRepeat = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [backspaceHeld, setBackspaceHeld] = useState(false);
  const spaceHoldTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const spaceHeld = useRef(false);
  const spaceLast = useRef({ x: 0, y: 0 });
  const [spaceNavigating, setSpaceNavigating] = useState(false);
  const applePlatform = remoteOS === "darwin";
  const { numberRow, letterRows, symbolRows } = keyboardRows(resolveKeyboardLayout(settings.keyboardLayout, navigator.language));
  const [systemDark, setSystemDark] = useState(() => matchMedia("(prefers-color-scheme: dark)").matches);
  useEffect(() => {
    const media = matchMedia("(prefers-color-scheme: dark)");
    const change = () => setSystemDark(media.matches);
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);

  const [emojiData, setEmojiData] = useState<typeof import("./emojiData") | null>(null);
  useEffect(() => {
    if (mode === "emoji") void import("./emojiData").then(setEmojiData);
  }, [mode]);
  const swipeShift = useRef<ShiftState>("off");
  const swipe = useSwipeTyping(
    settings.swipeTyping && mode === "letters" && !settingsOpen && !modifiers.some((modifier) => modifier !== Modifier.SHIFT),
    settings.longPressDelayMs,
    textContext,
    (word, previous) => {
      if (!previous) swipeShift.current = shift;
      const format = (value: string) => swipeShift.current === "locked" ? value.toUpperCase() : swipeShift.current === "once" ? value[0].toUpperCase() + value.slice(1) : value;
      if (previous) onReplaceText(`${format(previous)} `, `${format(word)} `);
      else {
        onText(`${format(word)} `);
        if (shift === "once") {
          setShift("off");
          if (modifiers.includes(Modifier.SHIFT)) onToggleModifier(Modifier.SHIFT);
        }
      }
      haptic(settings.haptics);
    },
  );

  useEffect(() => () => {
    if (backspaceTimer.current) clearTimeout(backspaceTimer.current);
    if (backspaceRepeat.current) clearTimeout(backspaceRepeat.current);
    if (spaceHoldTimer.current) clearTimeout(spaceHoldTimer.current);
  }, []);

  const type = (value: string, wasLongPress = false) => {
    swipe.clear();
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
    const shifted = !wasLongPress && (mode === "letters" || symbolSearching) && shift !== "off";
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
    swipe.clear();
    endBackspace();
    haptic(settings.haptics);
    const erase = () => {
      if (mode === "emoji" && emojiSearching)
        setEmojiQuery((current) => Array.from(current).slice(0, -1).join(""));
      else if (mode === "symbols" && symbolSearching)
        setSymbolQuery((current) => Array.from(current).slice(0, -1).join(""));
      else onBackspace();
    };
    erase();
    setBackspaceHeld(true);
    let repeats = 0;
    const repeat = () => {
      erase();
      if (repeats % 3 === 0) haptic(settings.haptics, 4);
      repeats++;
      backspaceRepeat.current = setTimeout(repeat,
        Math.max(18, settings.backspaceRepeatIntervalMs * Math.pow(0.8, Math.floor(repeats / 4))),
      );
    };
    backspaceTimer.current = setTimeout(repeat, settings.backspaceRepeatDelayMs);
  };
  const endBackspace = () => {
    if (backspaceTimer.current) clearTimeout(backspaceTimer.current);
    if (backspaceRepeat.current) clearTimeout(backspaceRepeat.current);
    backspaceTimer.current = null;
    backspaceRepeat.current = null;
    setBackspaceHeld(false);
  };

  const beginSpace = (event: React.PointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    if (spaceHoldTimer.current) clearTimeout(spaceHoldTimer.current);
    spaceHeld.current = false;
    spaceLast.current = { x: event.clientX, y: event.clientY };
    spaceHoldTimer.current = setTimeout(() => {
      spaceHeld.current = true;
      setSpaceNavigating(true);
      haptic(settings.haptics, 16);
    }, settings.spaceHoldDelayMs);
  };

  const moveSpace = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (!spaceHeld.current) {
      spaceLast.current = { x: event.clientX, y: event.clientY };
      return;
    }
    // One vertical step changes an entire line; horizontal steps change one
    // character. Give horizontal motion the finer resolution.
    const horizontalStep = 12;
    const verticalStep = 30;
    const dx = event.clientX - spaceLast.current.x;
    const dy = event.clientY - spaceLast.current.y;
    const horizontalSteps = Math.trunc(dx / horizontalStep);
    const verticalSteps = Math.trunc(dy / verticalStep);
    const emit = (key: string, count: number) => {
      for (let i = 0; i < Math.min(Math.abs(count), 6); i++) onNavigate(key);
    };
    if (horizontalSteps !== 0 && Math.abs(dx) / horizontalStep >= Math.abs(dy) / verticalStep) {
      emit(horizontalSteps > 0 ? "ArrowRight" : "ArrowLeft", horizontalSteps);
      spaceLast.current.x += horizontalSteps * horizontalStep;
      spaceLast.current.y = event.clientY;
    } else if (verticalSteps !== 0) {
      emit(verticalSteps > 0 ? "ArrowDown" : "ArrowUp", verticalSteps);
      spaceLast.current.y += verticalSteps * verticalStep;
      spaceLast.current.x = event.clientX;
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

  const rows = mode === "symbols" && !symbolSearching ? symbolRows : letterRows;
  const matchingEmoji = (emojiData?.EMOJIS ?? []).filter(([, keywords]) =>
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
    savePreference("ottoman-recent-emoji", next);
  };

  return (
    <div data-keyboard-theme={settings.keyboardTheme === "system" ? (systemDark ? "dark" : "light") : settings.keyboardTheme} className="custom-keyboard-shell fixed inset-x-0 bottom-0 z-50 border-t border-white/10 bg-zinc-950/98 pb-[max(8px,env(safe-area-inset-bottom))] shadow-[0_-16px_40px_rgba(0,0,0,.5)] backdrop-blur-xl md:absolute md:inset-x-auto md:bottom-auto md:left-0 md:top-full md:mt-2 md:w-[min(680px,calc(100vw-3rem))] md:rounded-xl md:border">
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
            <ModifierButton modifier={Modifier.META} label={remoteOS === "windows" ? "Windows" : "Super"} text={remoteOS === "windows" ? "Win" : "Super"} selected={modifiers} onToggle={onToggleModifier}>
              {remoteOS === "windows" ? <WindowsIcon className="h-[17px] w-[17px]" /> : <span className="text-base" aria-hidden="true">◆</span>}
            </ModifierButton>
            <ModifierButton modifier={Modifier.ALT} label="Alt" text="Alt" selected={modifiers} onToggle={onToggleModifier}>
              <Option className="h-4 w-4" />
            </ModifierButton>
          </>
        )}
        <ActionKey
          type="button"
          aria-label={settingsOpen ? "Hide keyboard settings" : "Keyboard settings"}
          title={settingsOpen ? "Hide keyboard settings" : "Keyboard settings"}
          aria-pressed={settingsOpen}
          onPress={() => setSettingsOpen((current) => !current)}
          className={`custom-keyboard-utility flex h-9 min-w-10 flex-1 items-center justify-center rounded-md border ${
            settingsOpen
              ? "border-blue-400/60 bg-blue-500 text-white"
              : "border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
          }`}
        >
          {settingsOpen ? <ArrowLeft className="h-4 w-4" /> : <Settings className="h-4 w-4" />}
        </ActionKey>
        <ActionKey
          type="button"
          aria-label="Close keyboard"
          title="Close keyboard"
          onPress={() => {
            setSettingsOpen(false);
            onClose();
          }}
          className="custom-keyboard-utility flex h-9 min-w-10 flex-1 items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-white"
        >
          <X className="h-4 w-4" />
        </ActionKey>
      </div>

      <div className="mx-auto flex max-w-3xl gap-1 px-2 pt-1.5" aria-label="Navigation keys">
        <NavigationKey label="Escape" onPress={() => navigate("Escape")}>Esc</NavigationKey>
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
        <NavigationKey label="Home" onPress={() => navigate("Home")}>Home</NavigationKey>
        <NavigationKey label="End" onPress={() => navigate("End")}>End</NavigationKey>
        <NavigationKey label="Delete" onPress={() => navigate("Delete")}>
          <Delete className="h-4 w-4 rotate-180" />
        </NavigationKey>
      </div>

      <div className="mx-auto flex h-[252px] max-w-3xl flex-col gap-1.5 px-1.5 sm:h-[274px] sm:gap-2 sm:px-2">
        {settingsOpen ? (
          <KeyboardSettingsPanel settings={settings} onSettingsChange={onSettingsChange}
            onUseCustomKeyboard={onUseCustomKeyboard} onUseDeviceKeyboard={onUseDeviceKeyboard}
            usingCustomKeyboard={usingCustomKeyboard} onBack={() => setSettingsOpen(false)} />
        ) : (
          <>
        {mode === "emoji" && !emojiSearching ? (
          <Suspense fallback={<div className="flex-1 text-center text-sm text-zinc-400">Loading emoji…</div>}><EmojiPicker recent={recentEmoji} onSearch={() => setEmojiSearching(true)} onSelect={selectEmoji} /></Suspense>
        ) : (
          <>
            {mode === "emoji" && emojiSearching && (
              <div className="flex h-11 shrink-0 items-center gap-1.5 overflow-hidden rounded-md bg-zinc-900/70 px-2">
                <Search className="h-3.5 w-3.5 shrink-0 text-zinc-500" />
                <span className="min-w-12 flex-1 truncate text-sm text-zinc-200">
                  {emojiQuery || <span className="text-zinc-500">Search Emoji</span>}
                </span>
                {matchingEmoji.slice(0, 5).map(([emoji]) => (
                  <ActionKey
                    type="button"
                    key={emoji}
                    onPress={() => selectEmoji(emoji)}
                    className="h-8 w-8 shrink-0 rounded text-lg hover:bg-zinc-800"
                  >
                    {emoji}
                  </ActionKey>
                ))}
              </div>
            )}
            {mode === "symbols" && symbolSearching && (
              <div className="flex h-11 shrink-0 items-center gap-1.5 overflow-hidden rounded-md bg-zinc-900/70 px-2">
                <Search className="h-3.5 w-3.5 shrink-0 text-zinc-500" />
                <span className="min-w-12 flex-1 truncate text-sm text-zinc-200">
                  {symbolQuery || <span className="text-zinc-500">Search Symbols</span>}
                </span>
                {matchingSymbols.slice(0, 6).map(([symbol]) => (
                  <ActionKey
                    type="button"
                    key={symbol}
                    aria-label={`Type ${symbol}`}
                    onPress={() => selectSymbol(symbol)}
                    className="h-8 w-8 shrink-0 rounded text-lg text-zinc-100 hover:bg-zinc-800"
                  >
                    {symbol}
                  </ActionKey>
                ))}
              </div>
            )}
            <div className="flex h-7 shrink-0 items-center justify-center gap-1 text-[10px] text-zinc-500" aria-label="Word suggestions">
              {swipe.candidates.length > 0 && mode === "letters" ? swipe.candidates.map((word) => (
                <ActionKey type="button" key={word} onPress={() => swipe.select(word)} className="custom-keyboard-utility h-7 flex-1 rounded-md text-sm text-zinc-200 hover:bg-zinc-700">{word}</ActionKey>
              )) : mode === "symbols" ? "Symbols match the letter-key hints" : settings.swipeTyping ? (swipe.ready ? "Tap or swipe to type · hold for symbols" : "Loading swipe typing…") : "Hold a key for its symbol"}
            </div>
            <div className="relative flex min-h-0 flex-1 flex-col gap-1.5 sm:gap-2" {...swipe.handlers}>
            {settings.numberRow && mode !== "emoji" && (
              <div className="flex min-h-0 flex-1 gap-1 sm:gap-1.5">
                {numberRow.map((definition) => (
                  <Key key={definition.key} definition={mode === "letters" && shift !== "off" ? { key: definition.alt!, alt: definition.key } : definition} settings={settings} onType={type} />
                ))}
              </div>
            )}
            {rows.map((row, index) => (
              <div
                key={index}
                className={`flex min-h-0 flex-1 gap-1 sm:gap-1.5 ${index === 1 ? "px-[4.5%]" : ""}`}
              >
                {index === 2 && mode === "symbols" && !symbolSearching && (
                  <UtilityKey label="Search Symbols" wide onPress={() => setSymbolSearching(true)}><Search className="h-5 w-5" /></UtilityKey>
                )}
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
                    swipeActive={swipe.active}
                    suppressTap={swipe.suppressTap}
                  />
                ))}
                {index === 2 && (
                  <button
                    type="button"
                    aria-label="Backspace"
                    onPointerDown={(event) => {
                      event.preventDefault();
                      event.currentTarget.setPointerCapture(event.pointerId);
                      beginBackspace();
                    }}
                    onPointerUp={endBackspace}
                    onPointerCancel={endBackspace}
                    onLostPointerCapture={endBackspace}
                    className={`custom-keyboard-utility flex min-w-[52px] flex-[1.4] touch-none select-none items-center justify-center rounded-md border border-white/5 bg-zinc-800 text-zinc-100 shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:bg-zinc-700 ${backspaceHeld ? "ring-2 ring-blue-400/60 text-blue-200" : ""}`}
                  >
                    <Delete className="h-5 w-5" />
                  </button>
                )}
              </div>
            ))}
              {swipe.active && <svg className="pointer-events-none absolute inset-0 z-40 h-full w-full overflow-visible" aria-hidden="true"><polyline points={swipe.trail.map(({ x, y }) => `${x},${y}`).join(" ")} fill="none" stroke="#60a5fa" strokeWidth="5" strokeLinecap="round" strokeLinejoin="round" opacity="0.8" /></svg>}
            </div>
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
          <UtilityKey label="Comma" onPress={() => { haptic(settings.haptics); type(","); }}>,</UtilityKey>
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
          {settings.periodKey && <UtilityKey label="Period" onPress={() => { haptic(settings.haptics); type("."); }}>.</UtilityKey>}
          <ActionKey
            type="button"
            disabled={!connected}
            aria-label="Enter"
            onPress={() => {
              swipe.clear();
              if (emojiSearching && matchingEmoji[0]) selectEmoji(matchingEmoji[0][0]);
              else if (symbolSearching && matchingSymbols[0]) selectSymbol(matchingSymbols[0][0]);
              else onEnter(shift !== "off");
            }}
            className="flex min-w-[54px] flex-[1.4] touch-none select-none items-center justify-center rounded-md border border-blue-400/40 bg-blue-500 text-white shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:translate-y-px active:bg-blue-400 disabled:opacity-40"
          >
            <CornerDownLeft className="h-5 w-5" />
          </ActionKey>
        </div>
          </>
        )}
      </div>
    </div>
  );
}

// Each visible session owns its gesture timers; hiding it disposes them.
export function CustomKeyboard({ open, ...props }: React.ComponentProps<typeof KeyboardSession> & { open: boolean }) {
  return open ? <KeyboardSession {...props} /> : null;
}
