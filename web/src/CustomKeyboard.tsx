import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowBigUp,
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowRightToLine,
  ArrowUp,
  ChevronUp,
  Command,
  Check,
  CornerDownLeft,
  Delete,
  Move,
  MoreHorizontal,
  SkipBack,
  Play,
  Pause,
  SkipForward,
  Volume1,
  Volume2,
  VolumeX,
  Option,
  Search,
  Settings,
  Smile,
  X,
} from "lucide-react";
import { Modifier } from "./api";
import { type KeyboardSettings } from "./trackpadSettings";
import { keyboardRows, resolveKeyboardLayout } from "./keyboardData";
import { currentWord as getCurrentWord, matchWordCase } from "./wordPrediction";
import { searchSymbols } from "./symbolData";
import { CaretText } from "./CaretText";
import { useSwipeTyping } from "./useSwipeTyping";
const EmojiPicker = lazy(() => import("./EmojiPicker").then((module) => ({ default: module.EmojiPicker })));
import { loadRecentEmoji, savePreference } from "./preferences";
import { ActionKey, KeyPreview, WindowsIcon, ModifierButton, Key, UtilityKey, NavigationKey } from "./KeyboardKeys";
import { haptic } from "./keyboardHaptics";
import { KeyboardSettingsPanel } from "./KeyboardSettingsPanel";

type ShiftState = "off" | "once" | "locked";
type KeyboardMode = "letters" | "symbols" | "emoji";

function KeyboardSession({
  connected,
  docked = false,
  textBeforeCursor = "",
  sensitiveInput = false,
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
  onSuperPress,
}: {
  connected: boolean;
  docked?: boolean;
  textBeforeCursor?: string;
  sensitiveInput?: boolean;
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
  onSuperPress: () => void;
}) {
  const [mode, setMode] = useState<KeyboardMode>("letters");
  const [shift, setShift] = useState<ShiftState>(() =>
    modifiers.includes(Modifier.SHIFT) ? "once" : "off",
  );
  const [emojiSearching, setEmojiSearching] = useState(false);
  const [symbolSearching, setSymbolSearching] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(settingsPageOpen);
  const [emojiQuery, setEmojiQuery] = useState("");
  const [symbolQuery, setSymbolQuery] = useState("");
  const [recentEmoji, setRecentEmoji] = useState<string[]>(loadRecentEmoji);
  const lastShiftTap = useRef(-Infinity);
  const backspaceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backspaceRepeat = useRef<ReturnType<typeof setTimeout> | null>(null);
  // iOS can start its selection magnifier despite pointerdown prevention and
  // user-select: none. Cancel the native touch gesture for keys, leaving input
  // handling to pointer events. Scrollable pickers/settings retain native pans.
  const keyboardTouchRef = useCallback((keyboard: HTMLDivElement | null) => {
    if (!keyboard) return;
    const preventNativeHold = (event: TouchEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) return;
      const key = target.closest("button");
      if (key && !key.closest(".keyboard-scroll")) event.preventDefault();
    };
    const preventKeyMenu = (event: MouseEvent) => {
      if (event.target instanceof Element && event.target.closest("button")) event.preventDefault();
    };
    keyboard.addEventListener("touchstart", preventNativeHold, { passive: false });
    keyboard.addEventListener("contextmenu", preventKeyMenu);
    return () => {
      keyboard.removeEventListener("touchstart", preventNativeHold);
      keyboard.removeEventListener("contextmenu", preventKeyMenu);
    };
  }, []);
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
  const currentWord = getCurrentWord(textBeforeCursor);
  const [predictor, setPredictor] = useState<((prefix: string) => string[]) | null>(null);
  useEffect(() => {
    let disposed = false;
    void import("./swipeDictionary").then((module) => {
      if (!disposed) setPredictor(() => module.predictWords);
    }).catch(() => {});
    return () => { disposed = true; };
  }, []);
  const [selectedWord, setSelectedWord] = useState({ word: "", context: textContext });
  const pendingSwipeSpace = useRef({ context: textContext, pending: false });
  const takeWordPrefix = () => {
    const prefix = pendingSwipeSpace.current.context === textContext && pendingSwipeSpace.current.pending ? " " : "";
    pendingSwipeSpace.current = { context: textContext, pending: false };
    return prefix;
  };
  const swipeShift = useRef<ShiftState>("off");
  const swipe = useSwipeTyping(
    !sensitiveInput && settings.swipeTyping && mode === "letters" && !settingsOpen && !modifiers.some((modifier) => modifier !== Modifier.SHIFT),
    settings.longPressDelayMs,
    textContext,
    (word, previous) => {
      if (!previous) swipeShift.current = shift;
      const format = (value: string) => swipeShift.current === "locked" ? value.toUpperCase() : swipeShift.current === "once" ? value[0].toUpperCase() + value.slice(1) : matchWordCase(value, "");
      if (previous) onReplaceText(format(previous), format(word));
      else {
        onText(takeWordPrefix() + format(word));
        if (shift === "once") {
          setShift("off");
          if (modifiers.includes(Modifier.SHIFT)) onToggleModifier(Modifier.SHIFT);
        }
      }
      setSelectedWord({ word: format(word), context: textContext });
      pendingSwipeSpace.current = { context: textContext, pending: true };
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
    const prefix = /^[a-z0-9]/i.test(next) ? takeWordPrefix() : "";
    if (/\s/.test(next)) pendingSwipeSpace.current.pending = false;
    onText(prefix + next, shifted);
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
    pendingSwipeSpace.current.pending = false;
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
    swipe.clear();
    pendingSwipeSpace.current.pending = false;
    haptic(settings.haptics);
    onNavigate(key);
  };

  const rows = mode === "symbols" && !symbolSearching ? symbolRows : letterRows;
  const matchingEmoji = (emojiData?.EMOJIS ?? []).filter(([, keywords]) =>
    !emojiQuery.trim() || keywords.includes(emojiQuery.trim().toLocaleLowerCase()),
  );
  const matchingSymbols = searchSymbols(symbolQuery);
  const searching = mode === "symbols" && symbolSearching || mode === "emoji" && emojiSearching;
  const searchQuery = mode === "emoji" ? emojiQuery : symbolQuery;
  const searchResults = mode === "emoji" ? matchingEmoji : matchingSymbols;

  const selectSymbol = (symbol: string) => {
    swipe.clear();
    haptic(settings.haptics);
    onText(symbol);
  };

  const selectEmoji = (emoji: string) => {
    swipe.clear();
    haptic(settings.haptics);
    onText(emoji);
    const next = [emoji, ...recentEmoji.filter((item) => item !== emoji)].slice(0, 12);
    setRecentEmoji(next);
    savePreference("ottoman-recent-emoji", next);
  };

  const wordChoices = sensitiveInput || modifiers.some((modifier) => modifier !== Modifier.SHIFT) ? []
    : swipe.candidates.length ? swipe.candidates : currentWord ? [currentWord, ...(predictor?.(currentWord) ?? []).slice(0, 2)] : [];

  return (
    <div ref={keyboardTouchRef} data-taller-keys={settings.tallerKeys || undefined} data-keyboard-theme={settings.keyboardTheme === "system" ? (systemDark ? "dark" : "light") : settings.keyboardTheme} className={`custom-keyboard-shell z-50 border-white/10 bg-zinc-950/98 pb-[max(8px,env(safe-area-inset-bottom))] backdrop-blur-xl ${docked ? "relative shrink-0 rounded-2xl border remote-keyboard" : "fixed inset-x-0 bottom-0 border-t shadow-[0_-16px_40px_rgba(0,0,0,.5)] md:absolute md:inset-x-auto md:bottom-auto md:left-0 md:top-full md:mt-2 md:w-[min(680px,calc(100vw-3rem))] md:rounded-xl md:border"}`}>
      <div className="mx-auto flex max-w-3xl items-center gap-1 px-2 pt-1.5 pb-0.5" aria-label="Keyboard controls">
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
            <ModifierButton modifier={Modifier.META} label="Command" text="Cmd" selected={modifiers} onToggle={onToggleModifier} onHold={() => { haptic(settings.haptics, 16); onSuperPress(); }} holdDelayMs={settings.longPressDelayMs}>
              <Command className="h-4 w-4" />
            </ModifierButton>
          </>
        ) : (
          <>
            <ModifierButton modifier={Modifier.META} label={remoteOS === "windows" ? "Windows" : "Super"} text={remoteOS === "windows" ? "Win" : "Super"} selected={modifiers} onToggle={onToggleModifier} onHold={() => { haptic(settings.haptics, 16); onSuperPress(); }} holdDelayMs={settings.longPressDelayMs}>
              {remoteOS === "windows" && <WindowsIcon className="h-[17px] w-[17px]" />}
            </ModifierButton>
            <ModifierButton modifier={Modifier.ALT} label="Alt" text="Alt" selected={modifiers} onToggle={onToggleModifier}>
              <Option className="h-4 w-4" />
            </ModifierButton>
          </>
        )}
        <div className="relative shrink-0">
          <ActionKey type="button" aria-label="More keyboard actions" aria-expanded={moreOpen} aria-controls="keyboard-extra-actions"
            onPress={() => setMoreOpen((open) => !open)}
            className="custom-keyboard-utility flex h-9 w-9 items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 text-zinc-400">
            <MoreHorizontal className="h-4 w-4" />
          </ActionKey>
          {moreOpen && <div id="keyboard-extra-actions" role="group" aria-label="Additional keyboard keys" className="absolute bottom-full right-0 z-[80] mb-2 flex w-64 flex-col gap-1 rounded-xl border border-zinc-700 bg-zinc-900 p-2 shadow-2xl">
            <div className="grid grid-cols-3 gap-1">
              {[{ key: "PrintScreen", label: "Print Screen" }, { key: "Insert", label: "Insert" }].map(({ key, label }) => (
                <ActionKey key={key} type="button" onPress={() => { navigate(key); setMoreOpen(false); }} className="keyboard-extra-key">{label}</ActionKey>
              ))}
              <ActionKey type="button" aria-label="Type backtick" onPress={() => { type("`", true); haptic(settings.haptics); setMoreOpen(false); }} className="keyboard-extra-key">Backtick</ActionKey>
              {[
                { key: "MediaTrackPrevious", label: "Previous track", Icon: SkipBack },
                { key: "MediaPlayPause", label: "Play/pause", Icon: Play },
                { key: "MediaTrackNext", label: "Next track", Icon: SkipForward },
                { key: "AudioVolumeDown", label: "Volume down", Icon: Volume1 },
                { key: "AudioVolumeMute", label: "Mute", Icon: VolumeX },
                { key: "AudioVolumeUp", label: "Volume up", Icon: Volume2 },
              ].map(({ key, label, Icon }) => (
                <ActionKey key={key} type="button" aria-label={label} title={label} onPress={() => navigate(key)} className="keyboard-extra-key"><Icon aria-hidden="true" className="h-4 w-4" />{key === "MediaPlayPause" && <Pause aria-hidden="true" className="h-4 w-4" />}</ActionKey>
              ))}
            </div>
            <div className="grid grid-cols-2 gap-1">
              {[{ key: "PageUp", label: "Page up" }, { key: "PageDown", label: "Page down" }].map(({ key, label }) => (
                <ActionKey key={key} type="button" onPress={() => { navigate(key); setMoreOpen(false); }} className="keyboard-extra-key">{label}</ActionKey>
              ))}
            </div>
            <div className="grid grid-cols-3 gap-1">
              {Array.from({ length: 12 }, (_, index) => `F${index + 1}`).map((key) => (
                <ActionKey key={key} type="button" onPress={() => { navigate(key); setMoreOpen(false); }} className="keyboard-extra-key">{key}</ActionKey>
              ))}
            </div>
          </div>}

        </div>
        <ActionKey
          type="button"
          aria-label={settingsOpen ? "Hide keyboard settings" : "Keyboard settings"}
          title={settingsOpen ? "Hide keyboard settings" : "Keyboard settings"}
          aria-pressed={settingsOpen}
          onPress={() => setSettingsOpen((current) => !current)}
          className={`custom-keyboard-utility flex h-9 w-9 shrink-0 items-center justify-center rounded-md border ${
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
          className="custom-keyboard-utility flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-white"
        >
          <X className="h-4 w-4" />
        </ActionKey>
      </div>

      <div className="mx-auto flex max-w-3xl gap-1 px-2 pt-0.5" aria-label="Navigation keys">
        <NavigationKey label="Escape" onPress={() => navigate("Escape")}>Esc</NavigationKey>
        <NavigationKey label="Home" onPress={() => navigate("Home")}>Home</NavigationKey>
        <NavigationKey label="End" onPress={() => navigate("End")}>End</NavigationKey>
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
        <NavigationKey label="Delete" onPress={() => navigate("Delete")}>
          <Delete className="h-4 w-4 rotate-180" />
        </NavigationKey>
      </div>

      <div className={`mx-auto mt-1.5 flex max-w-3xl flex-col gap-0 px-1.5 sm:px-2 ${settings.tallerKeys ? "h-[300px] sm:h-[322px]" : "h-[252px] sm:h-[274px]"}`}>
        {settingsOpen ? (
          <KeyboardSettingsPanel settings={settings} onSettingsChange={onSettingsChange}
            onUseCustomKeyboard={onUseCustomKeyboard} onUseDeviceKeyboard={onUseDeviceKeyboard}
            usingCustomKeyboard={usingCustomKeyboard} onBack={() => setSettingsOpen(false)} />
        ) : (
          <>
        <div className="mb-1 flex h-8 shrink-0 items-center gap-1" aria-label={mode === "letters" ? "Word suggestions" : `Search ${mode === "emoji" ? "Emoji" : "Symbols"}`}>
          {mode === "letters" ? (
            wordChoices.map((word) => (
              <ActionKey type="button" key={word}
                aria-pressed={selectedWord.context === textContext && selectedWord.word.toLowerCase() === word.toLowerCase() && currentWord.toLowerCase() === word.toLowerCase()}
                onPress={() => {
                setSelectedWord({ word: matchWordCase(word, currentWord), context: textContext });
                if (swipe.candidates.length) swipe.select(word);
                else {
                  if (word !== currentWord) onReplaceText(currentWord, matchWordCase(word, currentWord));
                  swipe.clear();
                  pendingSwipeSpace.current = { context: textContext, pending: true };
                }
                haptic(settings.haptics);
              }} className="keyboard-suggestion flex h-8 min-w-0 flex-1 items-center justify-center gap-1 rounded-md border border-white/5 bg-white/[0.02] px-2 text-sm text-zinc-300 hover:bg-white/5 aria-pressed:border-blue-400/60 aria-pressed:bg-blue-500/20 aria-pressed:text-blue-200">
                {selectedWord.context === textContext && selectedWord.word.toLowerCase() === word.toLowerCase() && currentWord.toLowerCase() === word.toLowerCase() && <Check className="h-3 w-3 shrink-0" aria-hidden="true" />}
                <span className="truncate">{matchWordCase(word, currentWord)}</span>
              </ActionKey>
            ))
          ) : <>
            <ActionKey type="button" aria-label={mode === "emoji" ? "Search Emoji" : "Search Symbols"}
              onPress={() => mode === "emoji" ? setEmojiSearching(true) : setSymbolSearching(true)}
              className="custom-keyboard-utility flex h-8 min-w-20 flex-1 items-center gap-2 rounded-md px-2 text-left text-xs text-zinc-400">
              <Search className="h-3.5 w-3.5 shrink-0" />
              <div className="relative min-w-0 flex-1 text-left">
                <CaretText value={searchQuery} cursor={searchQuery.length} hideCaret={!searching} variant="search" className="h-4 overflow-hidden whitespace-pre text-xs leading-4" />
                {!searchQuery && <span className="pointer-events-none absolute inset-0 ml-1 text-zinc-500">Search {mode === "emoji" ? "emoji" : "symbols"}</span>}
              </div>
            </ActionKey>
          </>}
        </div>
        {mode === "emoji" && !emojiSearching ? (
          <Suspense fallback={<div className="flex-1 text-center text-sm text-zinc-400">Loading emoji…</div>}><EmojiPicker recent={recentEmoji} onSelect={selectEmoji} /></Suspense>
        ) : (
          <>
            <div className="keyboard-key-rows relative flex min-h-0 flex-1 flex-col" data-swipe-ready={swipe.ready} {...swipe.handlers}>
            {(settings.numberRow || searching || mode === "symbols") && (
              searching ? <div className="keyboard-scroll flex min-h-0 flex-1 gap-1 overflow-x-auto py-0.5" role="region" aria-label={mode === "emoji" ? "Emoji search results" : "Symbol search results"}>
                {searchResults.map(([symbol, keywords]) => <ActionKey type="button" key={symbol} aria-label={`Type ${symbol}`} title={keywords}
                  onPress={() => mode === "emoji" ? selectEmoji(symbol) : selectSymbol(symbol)}
                  className="keyboard-suggestion flex w-9 shrink-0 items-center justify-center rounded-md border border-white/5 bg-white/[0.02] text-lg text-zinc-200 hover:bg-white/5">{symbol}</ActionKey>)}
                {searchResults.length === 0 && <span role="status" className="flex flex-1 items-center justify-center text-xs text-zinc-500">No matches</span>}
              </div> : <div className="flex min-h-0 flex-1">
                {numberRow.map((definition) => (
                  <Key key={definition.key} definition={mode === "symbols" ? { key: definition.alt! } : shift !== "off" ? { key: definition.alt!, alt: definition.key } : definition} settings={settings} onType={type} />
                ))}
              </div>
            )}
            {rows.map((row, index) => (
              <div
                key={index}
                className={`flex min-h-0 flex-1 ${index === 1 ? "px-[4.5%]" : ""}`}
              >
                {index === 2 && mode === "symbols" && !symbolSearching && <div className="keyboard-cell min-w-[52px] flex-[1.4] border border-transparent" />}
                {index === 2 && (mode === "letters" || symbolSearching) && (
                  <UtilityKey label="Shift" active={shift !== "off"} wide onPress={toggleShift}>
                    <ArrowBigUp className={`h-5 w-5 ${shift === "locked" ? "fill-current" : ""}`} />
                  </UtilityKey>
                )}
                {index === 2 && mode === "emoji" && emojiSearching && (
                  <div className="keyboard-cell custom-keyboard-utility flex min-w-[52px] flex-[1.4] items-center justify-center rounded-md border border-white/5 bg-zinc-800 text-zinc-500">
                    <Search className="h-4 w-4" />
                  </div>
                )}
                {row.map((definition, column) => definition.key ? (
                  <Key
                    key={definition.key}
                    definition={definition}
                    label={(mode === "letters" || symbolSearching) && shift !== "off" ? definition.key.toUpperCase() : undefined}
                    settings={settings}
                    onType={type}
                    swipeActive={swipe.active}
                    suppressTap={swipe.suppressTap}
                  />
                ) : <div key={`empty-${column}`} aria-hidden="true" className="custom-keyboard-key min-w-0 flex-1" />)}
                {index === 2 && (
                  <button
                    type="button"
                    aria-label="Backspace"
                    onContextMenu={(event) => event.preventDefault()}
                    onPointerDown={(event) => {
                      event.preventDefault();
                      event.currentTarget.setPointerCapture(event.pointerId);
                      beginBackspace();
                    }}
                    onPointerUp={endBackspace}
                    onPointerCancel={endBackspace}
                    onLostPointerCapture={endBackspace}
                    className={`keyboard-cell custom-keyboard-utility relative flex min-w-[52px] flex-[1.4] touch-none select-none items-center justify-center rounded-md border border-white/5 bg-zinc-800 text-zinc-100 shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:bg-zinc-700 ${backspaceHeld ? "ring-2 ring-blue-400/60 text-blue-200" : ""}`}
                  >
                    <Delete className="h-5 w-5" />
                    {backspaceHeld && <KeyPreview><Delete className="h-5 w-5" /></KeyPreview>}
                  </button>
                )}
              </div>
            ))}
              {swipe.trail.length > 1 && <svg className="pointer-events-none absolute inset-0 z-40 h-full w-full overflow-visible" aria-hidden="true" data-swipe-trail>
                {swipe.trail.slice(1).map((point, index) => <line key={index} x1={swipe.trail[index].x} y1={swipe.trail[index].y} x2={point.x} y2={point.y} stroke="#60a5fa" strokeWidth={2 + point.opacity * 3} strokeLinecap="round" opacity={point.opacity * 0.8} />)}
              </svg>}
            </div>
          </>
        )}

        <div className={`flex shrink-0 gap-0 ${settings.tallerKeys ? "h-14" : "h-12"}`}>
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
            className={`keyboard-cell custom-keyboard-key flex-[4] touch-none select-none rounded-md border text-sm shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.35)] active:translate-y-px ${
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
          {settings.commaKey && <UtilityKey label="Comma" onPress={() => { haptic(settings.haptics); type(","); }}>,</UtilityKey>}
          {settings.periodKey && <UtilityKey label="Period" onPress={() => { haptic(settings.haptics); type("."); }}>.</UtilityKey>}
          <ActionKey
            type="button"
            disabled={!connected}
            aria-label="Enter"
            preview={<CornerDownLeft className="h-5 w-5" />}
            onPress={() => {
              swipe.clear();
              pendingSwipeSpace.current.pending = false;
              if (emojiSearching && matchingEmoji[0]) selectEmoji(matchingEmoji[0][0]);
              else if (symbolSearching && matchingSymbols[0]) selectSymbol(matchingSymbols[0][0]);
              else onEnter(shift !== "off");
            }}
            className="keyboard-cell custom-keyboard-utility flex min-w-[48px] flex-[1.3] touch-none select-none items-center justify-center rounded-md border border-white/5 bg-zinc-800 text-zinc-100 shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:translate-y-px active:bg-zinc-700 disabled:opacity-40"
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
